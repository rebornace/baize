package mobileagent

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// errBlockedNetwork is deliberately generic: tool output reaches the model and
// the user, and must not disclose whether a guessed internal address exists.
var errBlockedNetwork = errors.New("mobileagent: destination network is not allowed")

// redactNetworkError collapses any wrapped blocked-address error to the generic
// sentinel so tool output does not echo the rejected host or port.
func redactNetworkError(err error) error {
	if err != nil && errors.Is(err, errBlockedNetwork) {
		return errBlockedNetwork
	}
	return err
}

// netAllowlist narrows private access to explicit CIDRs/IPs. Host entries are
// an optional extra restriction for DNS names resolving into those ranges.
type netAllowlist struct {
	hosts    map[string]bool
	prefixes []netip.Prefix
}

type netAllowlistJSON struct {
	Hosts []string `json:"hosts"`
	CIDRs []string `json:"cidrs"`
}

func parseNetAllowlist(raw string) (*netAllowlist, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var cfg netAllowlistJSON
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	out := &netAllowlist{hosts: map[string]bool{}}
	for _, host := range cfg.Hosts {
		h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
		if h == "" {
			continue
		}
		if net.ParseIP(h) != nil {
			return nil, errors.New("allowed hosts must be DNS names, not IP literals")
		}
		out.hosts[h] = true
	}
	for _, cidr := range cfg.CIDRs {
		c := strings.TrimSpace(cidr)
		if c == "" {
			continue
		}
		if strings.Contains(c, "/") {
			p, err := netip.ParsePrefix(c)
			if err != nil {
				return nil, err
			}
			out.prefixes = append(out.prefixes, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(c)
		if err != nil {
			return nil, err
		}
		out.prefixes = append(out.prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	if len(out.hosts) == 0 && len(out.prefixes) == 0 {
		return nil, nil
	}
	return out, nil
}

func (n *netAllowlist) allowsIP(ip net.IP) bool {
	if n == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	for _, p := range n.prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// checkHost enforces an optional hostname restriction. It only matters when
// the name resolves to a private address; public destinations are unaffected.
func (n *netAllowlist) checkHost(ctx context.Context, host string) error {
	if n == nil || len(n.hosts) == 0 {
		return nil
	}
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if h == "" || net.ParseIP(h) != nil {
		return nil // IP literals are governed by CIDRs only.
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, h)
	if err != nil || len(ips) == 0 {
		return nil // let the dialer surface the network error
	}
	for _, ip := range ips {
		if !isPublicIP(ip.IP) {
			if !n.hosts[h] {
				return errBlockedNetwork
			}
			return nil
		}
	}
	return nil
}

// guardedHTTPClient returns a client that either allows any destination
// (allowAll) or only public targets plus private targets matching the
// allowlist. The IP check runs in net.Dialer.Control on the actual dial
// address, closing the DNS-rebinding window; redirects re-enter the transport.
func guardedHTTPClient(allowAll bool, allow *netAllowlist) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	if !allowAll {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errBlockedNetwork
			}
			ip := net.ParseIP(strings.Trim(host, "[]"))
			if ip == nil {
				return errBlockedNetwork
			}
			if isPublicIP(ip) || allow.allowsIP(ip) {
				return nil
			}
			return errBlockedNetwork
		}
	}
	transport := &http.Transport{
		// No environment proxy: a proxy would hide the real destination from
		// the address check and could reach internal networks on our behalf.
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	checkRedirect := limitRedirects
	if !allowAll && allow != nil {
		checkRedirect = func(req *http.Request, via []*http.Request) error {
			if err := allow.checkHost(req.Context(), req.URL.Hostname()); err != nil {
				return err
			}
			return limitRedirects(req, via)
		}
	}
	return &http.Client{
		Timeout:       httpGetTimeout,
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
}

// isPublicIP reports whether ip is safe to reach by default. It intentionally
// rejects more than net.IP.IsPrivate: loopback, link-local, CGNAT, multicast,
// unspecified and reserved ranges are all on-device or infrastructure targets.
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 0:
			return false
		case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127: // 100.64.0.0/10 CGNAT
			return false
		case v4[0] == 192 && v4[1] == 0 && v4[2] == 0: // 192.0.0.0/24 IETF
			return false
		case v4[0] == 198 && (v4[1] == 18 || v4[1] == 19): // benchmarking
			return false
		case v4[0] >= 240: // 240.0.0.0/4 reserved
			return false
		}
		return true
	}
	// Deprecated IPv6 site-local fec0::/10 is not covered by IsPrivate.
	if len(ip) == net.IPv6len && ip[0] == 0xfe && ip[1]&0xc0 == 0xc0 {
		return false
	}
	return true
}
