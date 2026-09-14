package memory

import "time"

const (
	SourceExplicit        = "explicit"
	SourceAuto            = "auto"
	MaxEntryChars         = 500
	DefaultTopK           = 8
	DefaultMaxInjectChars = 2000
)

// Entry is one account-scoped memory fact.
type Entry struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"owner_id"`
	Key       string    `json:"key,omitempty"`
	Text      string    `json:"text"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func truncateText(s string) string {
	r := []rune(s)
	if len(r) <= MaxEntryChars {
		return s
	}
	return string(r[:MaxEntryChars])
}
