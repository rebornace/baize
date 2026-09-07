package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPAdminClientStatusAndActions(t *testing.T) {
	var gotSig, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get(HeaderProtocol) != ProtocolVersion {
			t.Errorf("missing/incorrect protocol header on %s: %q", r.URL.Path, r.Header.Get(HeaderProtocol))
		}
		if r.Header.Get(HeaderTimestamp) == "" {
			t.Errorf("missing timestamp on %s", r.URL.Path)
		}
		if r.Header.Get(HeaderSignature) == "" {
			t.Errorf("missing signature on %s", r.URL.Path)
		}
		gotSig = r.Header.Get(HeaderSignature)
		switch r.URL.Path {
		case "/admin/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"has_credentials": true, "polling": true, "account_id": "bot@im.bot"})
		case "/admin/login/start":
			_ = json.NewEncoder(w).Encode(map[string]string{"ticket": "tk", "qr_url": "qr"})
		case "/admin/login/status":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		default:
			w.WriteHeader(http.StatusOK)
		}
		_ = body
	}))
	t.Cleanup(srv.Close)

	ac := newHTTPAdminClient(srv.URL, "secret")
	hasCreds, polling, err := ac.Status(context.Background())
	if err != nil || !hasCreds || !polling {
		t.Fatalf("status: %v %v %v", hasCreds, polling, err)
	}
	if !strings.HasPrefix(gotPath, "GET /admin/status") || gotSig == "" {
		t.Fatalf("status call path=%q sig=%q", gotPath, gotSig)
	}
	tk, qr, err := ac.LoginStart(context.Background())
	if err != nil || tk != "tk" || qr != "qr" {
		t.Fatalf("login start: %q %q %v", tk, qr, err)
	}
	st, err := ac.LoginPoll(context.Background(), "tk")
	if err != nil || st != "success" {
		t.Fatalf("login poll: %q %v", st, err)
	}
	if err := ac.Logout(context.Background()); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if err := ac.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := ac.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestHTTPAdminClientUnreachable(t *testing.T) {
	ac := newHTTPAdminClient("http://127.0.0.1:1", "s") // closed port
	if _, _, err := ac.Status(context.Background()); err == nil {
		t.Fatal("expected error for unreachable adapter")
	}
}

// TestHTTPAdminClientLoginPollUsesLongTimeout guards the long-poll contract:
// iLink's get_qrcode_status holds the adapter's request open until scan/confirm
// (~30s idle). Fast admin calls must still fail quickly, but login/status must
// tolerate a slow adapter instead of being cut at the short deadline (the
// "HTTP 502 context deadline exceeded" regression).
func TestHTTPAdminClientLoginPollUsesLongTimeout(t *testing.T) {
	const delay = 120 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		switch r.URL.Path {
		case "/admin/login/status":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "pending"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"has_credentials": false, "polling": false})
		}
	}))
	t.Cleanup(srv.Close)

	// short deadline < delay (fast calls time out); long deadline > delay (poll survives).
	ac := newHTTPAdminClientWithTimeouts(srv.URL, "s", 30*time.Millisecond, 5*time.Second)

	if _, _, err := ac.Status(context.Background()); err == nil {
		t.Fatal("fast Status call should time out at the short deadline")
	}
	st, err := ac.LoginPoll(context.Background(), "tk")
	if err != nil {
		t.Fatalf("LoginPoll should tolerate the slow adapter under the long deadline: %v", err)
	}
	if st != "pending" {
		t.Fatalf("LoginPoll status=%q want pending", st)
	}
}
