package store

import (
	"path/filepath"
	"testing"
	"time"
)

func channelPendingCase(t *testing.T, st Store) {
	t.Helper()
	now := time.Now().UTC()
	first := ChannelPending{
		ConversationID: "c1", BatchID: "b1", Source: "weixin",
		Account: "acc", PeerID: "p1", Text: "first", Truncated: true,
		Attachments: []PendingAttachment{{Name: "a.txt", MIME: "text/plain", Data: []byte("hello")}},
		UpdatedAt:   now.Add(-time.Second),
	}
	second := ChannelPending{
		ConversationID: "c2", BatchID: "b2", Source: "weixin",
		Account: "acc", PeerID: "p2", Text: "second", UpdatedAt: now,
	}
	other := ChannelPending{
		ConversationID: "c3", BatchID: "b3", Source: "webhook",
		Account: "acc", PeerID: "p3", Text: "other", UpdatedAt: now,
	}
	for _, p := range []ChannelPending{first, second, other} {
		if err := st.UpsertChannelPending(p); err != nil {
			t.Fatalf("upsert %s: %v", p.BatchID, err)
		}
	}
	first.Text = "first+more"
	if err := st.UpsertChannelPending(first); err != nil {
		t.Fatalf("upsert update: %v", err)
	}

	got, err := st.ListChannelPending("weixin")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].BatchID != "b1" || got[1].BatchID != "b2" {
		t.Fatalf("weixin rows=%+v", got)
	}
	if got[0].Text != "first+more" || !got[0].Truncated || got[0].Account != "acc" || got[0].PeerID != "p1" {
		t.Fatalf("b1=%+v", got[0])
	}
	if len(got[0].Attachments) != 1 || got[0].Attachments[0].Name != "a.txt" ||
		got[0].Attachments[0].MIME != "text/plain" || string(got[0].Attachments[0].Data) != "hello" {
		t.Fatalf("attachments=%+v", got[0].Attachments)
	}
	all, err := st.ListChannelPending("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("all rows=%d", len(all))
	}

	if err := st.DeleteChannelPending("c1", "b1"); err != nil {
		t.Fatal(err)
	}
	got, err = st.ListChannelPending("weixin")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].BatchID != "b2" {
		t.Fatalf("after delete=%+v", got)
	}
	if err := st.DeleteChannelPending("c1", "b1"); err != nil {
		t.Fatalf("delete must be idempotent: %v", err)
	}
}

func TestChannelPendingMemory(t *testing.T) {
	channelPendingCase(t, NewMemory())
}

func TestChannelPendingSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "channel_pending.db")
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	channelPendingCase(t, st)
}
