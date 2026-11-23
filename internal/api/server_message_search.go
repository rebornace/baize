package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
)

const (
	defaultMessageSearchLimit = 20
	maxMessageSearchLimit     = 50
	// Fetch extra rows so ACL filtering can still fill the requested page.
	messageSearchFetchLimit = 200
	messageSnippetRunes     = 160
	maxMessageSearchDays    = 3650
)

type messageSearchHitJSON struct {
	ConversationID    string `json:"conversation_id"`
	ConversationTitle string `json:"conversation_title,omitempty"`
	MessageID         string `json:"message_id"`
	Role              string `json:"role"`
	Snippet           string `json:"snippet"`
	CreatedAt         string `json:"created_at,omitempty"`
}

// handleSearchConversationMessages searches text across conversations. The
// login gate is applied per conversation before returning hits.
func (s *Server) handleSearchConversationMessages(w http.ResponseWriter, r *http.Request) {
	hits := []messageSearchHitJSON{}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || s.Messages == nil {
		writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
		return
	}
	limit := defaultMessageSearchLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxMessageSearchLimit {
		limit = maxMessageSearchLimit
	}
	filter := conversation.SearchFilter{Query: q, Limit: messageSearchFetchLimit}
	filter.ConversationID = strings.TrimSpace(r.URL.Query().Get("conversation_id"))
	switch role := strings.TrimSpace(r.URL.Query().Get("role")); role {
	case "", "all":
	case conversation.RoleUser, conversation.RoleAssistant, conversation.RoleSystemNote:
		filter.Roles = []string{role}
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid role")
		return
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || days < 1 || days > maxMessageSearchDays {
			writeError(w, http.StatusBadRequest, "invalid_request", "days must be 1..3650")
			return
		}
		filter.Since = time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	}
	msgs, err := s.Messages.SearchAllFiltered(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	titles := s.conversationTitleMap()
	gate := s.gateTokens().Enabled()
	principal := s.principalFrom(r.Context())
	allowed := map[string]bool{}
	if gate && principal.Role != controlplane.RoleAdmin {
		if ms := s.metaStore(); ms != nil {
			if metas, err := ms.ListMeta(conversation.MetaFilter{}); err == nil {
				for _, meta := range metas {
					if conversation.CanAccess(principal, meta) {
						allowed[meta.ID] = true
					}
				}
			}
		}
	}
	for _, msg := range msgs {
		if gate && principal.Role != controlplane.RoleAdmin && !allowed[msg.ConversationID] {
			continue
		}
		hit := messageSearchHitJSON{
			ConversationID:    msg.ConversationID,
			ConversationTitle: titles[msg.ConversationID],
			MessageID:         msg.ID,
			Role:              msg.Role,
			Snippet:           messageSnippet(msg.Content),
		}
		if !msg.CreatedAt.IsZero() {
			hit.CreatedAt = msg.CreatedAt.UTC().Format(time.RFC3339)
		}
		hits = append(hits, hit)
		if len(hits) >= limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
}

func messageSnippet(content string) string {
	text := strings.Join(strings.Fields(conversation.StripMediaRefs(content)), " ")
	runes := []rune(text)
	if len(runes) <= messageSnippetRunes {
		return text
	}
	return string(runes[:messageSnippetRunes]) + "…"
}
