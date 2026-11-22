package mobileagent

import (
	"errors"
	"net"
	"net/http"
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

// guardedHTTPClient returns a client that either allows any destination
// (allowPrivate) or rejects loopback, private, link-local, CGNAT, multicast
// and reserved addresses after DNS resolution.
//
// The check runs in net.Dialer.Control, i.e. on the address the socket is
// actually about to connect to. That closes the DNS-rebinding window between
// "resolve and validate" and "connect".
func guardedHTTPClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	if !allowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errBlockedNetwork
			}
			ip := net.ParseIP(strings.Trim(host, "[]"))
			if ip == nil || !isPublicIP(ip) {
				return errBlockedNetwork
			}
			return nil
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
	return &http.Client{
		Timeout:       httpGetTimeout,
		Transport:     transport,
		CheckRedirect: limitRedirects,
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
