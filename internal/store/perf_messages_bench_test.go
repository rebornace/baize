package store_test

import (
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

// BenchmarkPerfListMessages times conversation message List for a seeded
// conversation of N=500 messages on the production sqlite path (shared DB via
// store.Open + conversation.OpenSQL). Matches handleListMessages:
// Messages.List(convID) with no limit/opts — there is no ListMessages API.
func BenchmarkPerfListMessages(b *testing.B) {
	path := filepath.Join(b.TempDir(), "messages-perf.db")
	st, err := store.Open("sqlite", path)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if c, ok := st.(io.Closer); ok {
			_ = c.Close()
		}
	})
	backend, ok := st.(store.SQLBackend)
	if !ok {
		b.Fatal("sqlite store is not SQLBackend")
	}
	msgs, err := conversation.OpenSQL(backend.DB(), backend.Dialect())
	if err != nil {
		b.Fatal(err)
	}

	const convID = "perf-conv"
	const nMsgs = 500
	for i := 0; i < nMsgs; i++ {
		role := conversation.RoleUser
		if i%2 == 1 {
			role = conversation.RoleAssistant
		}
		if _, err := msgs.Append(convID, conversation.Message{
			Role:    role,
			Content: fmt.Sprintf("msg-%d", i),
		}); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got := msgs.List(convID)
		if len(got) != nMsgs {
			b.Fatalf("want %d messages, got %d", nMsgs, len(got))
		}
	}
}
