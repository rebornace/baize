package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

// handleExportConversation streams one conversation as Markdown. Thinking
// blocks and internal media URLs are intentionally omitted; attachments are not
// embedded.
func (s *Server) handleExportConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "conversation id required")
		return
	}
	if !s.requireConversationAccess(w, r, id) {
		return
	}
	if s.Messages == nil {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	title := id
	for _, sum := range s.Messages.ListSummaries() {
		if sum.ID == id && strings.TrimSpace(sum.Title) != "" {
			title = sum.Title
			break
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- Conversation: `%s`\n", id)
	fmt.Fprintf(&b, "- Exported: %s\n\n---\n\n", time.Now().UTC().Format(time.RFC3339))
	for _, msg := range s.Messages.List(id) {
		role := exportRoleLabel(msg.Role)
		when := msg.CreatedAt.UTC().Format("2006-01-02 15:04 UTC")
		content := strings.TrimSpace(conversation.StripMediaRefs(msg.Content))
		fmt.Fprintf(&b, "## %s · %s\n\n%s\n\n", role, when, content)
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, exportConversationFilename(id)))
	_, _ = io.WriteString(w, b.String())
}

func exportRoleLabel(role string) string {
	switch role {
	case conversation.RoleUser:
		return "User"
	case conversation.RoleAssistant:
		return "Assistant"
	case conversation.RoleSystemNote:
		return "System note"
	default:
		return role
	}
}

// exportConversationFilename keeps the download name portable across OSes.
func exportConversationFilename(id string) string {
	var b strings.Builder
	b.WriteString("baize-")
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	b.WriteString(".md")
	return b.String()
}
