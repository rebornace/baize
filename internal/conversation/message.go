package conversation

import "time"

const (
	RoleUser       = "user"
	RoleAssistant  = "assistant"
	RoleSystemNote = "system_note"
)

type Message struct {
	ID             string
	ConversationID string
	Role           string
	Content        string
	RunID          string
	CreatedAt      time.Time
}
