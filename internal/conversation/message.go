package conversation

import "time"

const (
	RoleUser       = "user"
	RoleAssistant  = "assistant"
	RoleSystemNote = "system_note"
)

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	RunID          string    `json:"run_id"`
	CreatedAt      time.Time `json:"created_at"`
}
