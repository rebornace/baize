package schedule

import (
	"testing"
	"time"
)

func TestRenewClaimLoopExtendsLease(t *testing.T) {
	old := claimRenewInterval
	claimRenewInterval = 10 * time.Millisecond
	defer func() { claimRenewInterval = old }()

	s := NewMemoryStore()
	now := time.Now().UTC()
	it, err := s.Insert(Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "renew",
		Kind: "once", Timezone: "UTC", FireAt: now.Add(-time.Second), NextRunAt: now.Add(-time.Second),
		Status: StatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := s.ClaimDue(now)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	initial := claimed.ClaimExpiresAt

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		renewClaimLoop(s, it.ID, stop)
	}()
	time.Sleep(50 * time.Millisecond)
	close(stop)
	<-done

	s.mu.Lock()
	got := s.byID[it.ID]
	s.mu.Unlock()
	if !got.ClaimExpiresAt.After(initial) {
		t.Fatalf("claim not renewed: initial=%s now=%s", initial, got.ClaimExpiresAt)
	}

	if ok, err := s.RenewClaim(it.ID, now, time.Minute); err != nil || !ok {
		t.Fatalf("renew while claimed: ok=%v err=%v", ok, err)
	}
	if err := s.ReleaseRunningRun(it.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.RenewClaim(it.ID, now, time.Minute); err != nil || ok {
		t.Fatalf("renew after release: ok=%v err=%v want false", ok, err)
	}
}
