package store

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

// guardRuns races n attempts against one conversation and returns the winner.
func guardRuns(t *testing.T, stores []Store, conv string, n int) (*Run, int, int) {
	t.Helper()
	var (
		okCnt, busyCnt atomic.Int32
		winner         atomic.Value // *Run
		start          = make(chan struct{})
		wg             sync.WaitGroup
		unexpected     = make(chan error, n)
	)
	for i := 0; i < n; i++ {
		st := stores[i%len(stores)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, err := st.CreateRunIfConversationIdle(CreateRunInput{
				AgentID: "a", Input: "race", ConversationID: conv,
			})
			switch {
			case err == nil:
				okCnt.Add(1)
				winner.Store(r)
			case errors.Is(err, ErrConversationBusy):
				busyCnt.Add(1)
			default:
				unexpected <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(unexpected)
	for err := range unexpected {
		t.Fatalf("unexpected error: %v", err)
	}
	r, _ := winner.Load().(*Run)
	return r, int(okCnt.Load()), int(busyCnt.Load())
}

func guardCase(t *testing.T, stores []Store) {
	t.Helper()
	conv := "guard-" + uuid.NewString()
	winner, ok, busy := guardRuns(t, stores, conv, 8)
	if ok != 1 || busy != 7 || winner == nil {
		t.Fatalf("ok=%d busy=%d winner=%v", ok, busy, winner)
	}
	if _, err := stores[0].CreateRunIfConversationIdle(CreateRunInput{
		AgentID: "a", Input: "again", ConversationID: conv,
	}); !errors.Is(err, ErrConversationBusy) {
		t.Fatalf("running conversation err=%v", err)
	}
	if err := stores[0].UpdateRun(winner.ID, StatusSucceeded, "done", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := stores[0].CreateRunIfConversationIdle(CreateRunInput{
		AgentID: "a", Input: "after", ConversationID: conv,
	}); err != nil {
		t.Fatalf("idle conversation err=%v", err)
	}
}

func TestCreateRunIfIdleMemory(t *testing.T) {
	guardCase(t, []Store{NewMemory()})
}

func TestCreateRunIfIdleSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "guard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	guardCase(t, []Store{st})
}

func TestPostgresCreateRunIfIdleAcrossReplicas(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st1, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st1.Close() })
	st2, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })

	conv := "guard-" + uuid.NewString()
	winner, ok, busy := guardRuns(t, []Store{st1, st2}, conv, 10)
	if ok != 1 || busy != 9 || winner == nil {
		t.Fatalf("ok=%d busy=%d winner=%v", ok, busy, winner)
	}
	// The other replica sees the winner immediately and stays guarded.
	if got, err := st2.GetRun(winner.ID); err != nil || got.ConversationID != conv {
		t.Fatalf("cross-replica run=%+v err=%v", got, err)
	}
	if _, err := st2.CreateRunIfConversationIdle(CreateRunInput{
		AgentID: "a", Input: "again", ConversationID: conv,
	}); !errors.Is(err, ErrConversationBusy) {
		t.Fatalf("running conversation err=%v", err)
	}
	if err := st1.UpdateRun(winner.ID, StatusSucceeded, "done", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st2.CreateRunIfConversationIdle(CreateRunInput{
		AgentID: "a", Input: "after", ConversationID: conv,
	}); err != nil {
		t.Fatalf("idle conversation err=%v", err)
	}
}
