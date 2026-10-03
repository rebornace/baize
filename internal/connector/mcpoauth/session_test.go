package mcpoauth

import (
	"testing"
	"time"
)

func TestSessionStoreTakeConsumesOnce(t *testing.T) {
	store := NewSessionStore()
	exp := time.Now().Add(5 * time.Minute)
	want := Pending{
		ConnectorID: "conn-1",
		Verifier:    "verifier-abc",
		RedirectURI: "https://example.com/callback",
		Expires:     exp,
	}
	store.Put("state-1", want)

	got, err := store.Take("state-1")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if got != want {
		t.Fatalf("Take: got %+v, want %+v", got, want)
	}

	_, err = store.Take("state-1")
	if err != ErrSessionNotFound {
		t.Fatalf("second Take: %v, want ErrSessionNotFound", err)
	}
}

func TestSessionStoreTakeUnknownState(t *testing.T) {
	store := NewSessionStore()
	_, err := store.Take("missing")
	if err != ErrSessionNotFound {
		t.Fatalf("Take: %v, want ErrSessionNotFound", err)
	}
}

func TestSessionStoreTakeExpired(t *testing.T) {
	store := NewSessionStore()
	store.Put("state-exp", Pending{
		ConnectorID: "c",
		Verifier:    "v",
		RedirectURI: "https://example.com/cb",
		Expires:     time.Now().Add(-time.Minute),
	})
	_, err := store.Take("state-exp")
	if err != ErrSessionExpired {
		t.Fatalf("Take: %v, want ErrSessionExpired", err)
	}
	_, err = store.Take("state-exp")
	if err != ErrSessionNotFound {
		t.Fatalf("after expired Take: %v, want ErrSessionNotFound", err)
	}
}

func TestSessionStorePutDefaultTTL(t *testing.T) {
	fixed := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	store := &SessionStore{
		ttl: defaultSessionTTL,
		now: func() time.Time { return fixed },
	}
	store.Put("s", Pending{
		ConnectorID: "c",
		Verifier:    "v",
		RedirectURI: "https://example.com/cb",
	})

	got, err := store.Take("s")
	if err != nil {
		t.Fatal(err)
	}
	wantExp := fixed.Add(defaultSessionTTL)
	if !got.Expires.Equal(wantExp) {
		t.Fatalf("Expires %v, want %v", got.Expires, wantExp)
	}
}
