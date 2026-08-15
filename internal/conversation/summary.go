package conversation

import "time"

type Summary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
}

func TruncateTitle(s string) string {
	r := []rune(s)
	if len(r) <= 40 {
		return s
	}
	return string(r[:40]) + "…"
}

func Summarize(id string, msgs []Message) Summary {
	sum := Summary{ID: id, Title: "新对话"}
	for _, m := range msgs {
		if m.CreatedAt.After(sum.UpdatedAt) {
			sum.UpdatedAt = m.CreatedAt
		}
	}
	for _, m := range msgs {
		if m.Role == RoleUser {
			sum.Title = TruncateTitle(m.Content)
			break
		}
	}
	return sum
}
