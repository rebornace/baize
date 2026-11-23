package bootstrap

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestShutdownOnSignalDrainsServerOnCancel exercises the cancel->Shutdown
// path that SIGINT/SIGTERM triggers in production, using a plain cancelable
// context so it runs deterministically on every platform (including Windows,
// where delivering SIGTERM to a process group is awkward). After cancel the
// server must stop accepting new connections promptly (Serve returns
// ErrServerClosed) and the listener must refuse new dials.
func TestShutdownOnSignalDrainsServerOnCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	srv := &http.Server{Handler: http.NewServeMux()}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	// Wait until the server accepts connections.
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(3 * time.Second)
	up := false
	for time.Now().Before(deadline) {
		if c, derr := net.DialTimeout("tcp", addr, 200*time.Millisecond); derr == nil {
			_ = c.Close()
			up = true
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if !up {
		_ = srv.Close()
		t.Fatal("test server never accepted connections")
	}

	ctx, cancel := context.WithCancel(context.Background())
	shutdownOnSignal(ctx, srv)

	// Sanity: before cancel the server is still serving.
	if c, derr := net.DialTimeout("tcp", addr, 200*time.Millisecond); derr != nil {
		t.Fatalf("server not accepting before cancel: %v", derr)
	} else {
		_ = c.Close()
	}

	cancel() // simulate the SIGINT/SIGTERM firing

	// Serve must return ErrServerClosed promptly (not after the 10s grace
	// window — there are no in-flight requests to drain).
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Serve returned %v, want ErrServerClosed", err)
		}
	case <-time.After(5 * time.Second):
		_ = srv.Close()
		t.Fatal("server did not shut down within timeout after context cancel")
	}

	// New connections must now be refused.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, derr := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if derr != nil {
			return // listener closed: Shutdown took effect
		}
		_ = c.Close()
		// A dial can briefly succeed on a closing socket; also probe via HTTP.
		if _, gerr := client.Get("http://" + addr + "/"); gerr != nil {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("server still accepting connections after cancel-triggered shutdown")
}

// TestNormalizeShutdownErr pins the intentional-shutdown mapping.
func TestNormalizeShutdownErr(t *testing.T) {
	if err := normalizeShutdownErr(http.ErrServerClosed); err != nil {
		t.Fatalf("normalizeShutdownErr(ErrServerClosed) = %v, want nil", err)
	}
	other := errors.New("listen tcp :8080: bind")
	if err := normalizeShutdownErr(other); err != other {
		t.Fatalf("normalizeShutdownErr(other) = %v, want the original error", err)
	}
	if err := normalizeShutdownErr(nil); err != nil {
		t.Fatalf("normalizeShutdownErr(nil) = %v, want nil", err)
	}
}
