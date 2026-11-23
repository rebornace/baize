package webhook

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// explicitFixedAddr reports whether adapter_args pin a non-ephemeral -addr=
// (port != "0"). Absence of -addr means dynamic default for autostart.
func explicitFixedAddr(args []string) bool {
	for _, a := range args {
		a = strings.TrimSpace(a)
		if !strings.HasPrefix(a, "-addr=") {
			continue
		}
		addr := strings.TrimSpace(strings.TrimPrefix(a, "-addr="))
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			// Non-host:port forms are treated as fixed (operator intent).
			return true
		}
		if port != "0" {
			return true
		}
	}
	return false
}

// injectDynamicPortArgs strips existing -addr= / -port-file= then appends
// ephemeral bind and the discovery file path.
func injectDynamicPortArgs(args []string, portFile string) []string {
	out := make([]string, 0, len(args)+2)
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if strings.HasPrefix(a, "-addr=") || strings.HasPrefix(a, "-port-file=") {
			continue
		}
		out = append(out, a)
	}
	out = append(out, "-addr=127.0.0.1:0", "-port-file="+portFile)
	return out
}

// waitPortFile polls until path contains a valid host:port or ctx/timeout fails.
func waitPortFile(ctx context.Context, path string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if addr, err := readListenPortFile(path); err == nil {
			return addr, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timeout waiting for port-file %s", path)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func readListenPortFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	addr := strings.TrimSpace(string(raw))
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return "", fmt.Errorf("invalid listen addr %q: %w", addr, err)
	}
	return addr, nil
}

// httpBaseFromListenAddr builds http://host:port from a listen host:port.
func httpBaseFromListenAddr(addr string) string {
	return "http://" + strings.TrimSpace(addr)
}
