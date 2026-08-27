package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConversationMaxMessagesDefault(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Conversation.MaxMessages != 40 {
		t.Fatalf("MaxMessages=%d want 40", cfg.Conversation.MaxMessages)
	}
}

func TestLoadConversationMaxMessagesExplicit(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\nconversation:\n  max_messages: 12\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Conversation.MaxMessages != 12 {
		t.Fatalf("MaxMessages=%d want 12", cfg.Conversation.MaxMessages)
	}
}

func TestLoadConversationPersistIdentitiesNilByDefault(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: sqlite\n  sqlite_path: ./data/baize.db\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Conversation.PersistIdentities != nil {
		t.Fatalf("PersistIdentities should be nil when unset, got %v", *cfg.Conversation.PersistIdentities)
	}
}

func TestLoadConversationPersistIdentitiesExplicit(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want bool
	}{
		{"true", "store:\n  driver: sqlite\nconversation:\n  persist_identities: true\n", true},
		{"false", "store:\n  driver: sqlite\nconversation:\n  persist_identities: false\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeConfig(t, c.yaml)
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Conversation.PersistIdentities == nil {
				t.Fatalf("PersistIdentities is nil")
			}
			if *cfg.Conversation.PersistIdentities != c.want {
				t.Fatalf("PersistIdentities=%v want %v", *cfg.Conversation.PersistIdentities, c.want)
			}
		})
	}
}
