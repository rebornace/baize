package api

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

type exportFormat string

const (
	exportFormatMarkdown exportFormat = "markdown"
	exportFormatJSON     exportFormat = "json"
	exportFormatHTML     exportFormat = "html"
)

type exportAttachment struct {
	Filename string `json:"filename"`
}

type exportMessage struct {
	ID          string             `json:"id"`
	Role        string             `json:"role"`
	RoleLabel   string             `json:"role_label"`
	CreatedAt   string             `json:"created_at"`
	Content     string             `json:"content"`
	Thinking    string             `json:"thinking,omitempty"`
	Attachments []exportAttachment `json:"attachments,omitempty"`
}

type exportDocument struct {
	ConversationID  string          `json:"conversation_id"`
	Title           string          `json:"title"`
	ExportedAt      string          `json:"exported_at"`
	IncludeThinking bool            `json:"include_thinking"`
	Messages        []exportMessage `json:"messages"`
}

// handleExportConversation exports one conversation as Markdown, JSON or HTML.
// Thinking is opt-in; attachments are listed by filename and never embedded,
// and internal media URLs are omitted.
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
	format, includeThinking, err := parseExportOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	title := id
	for _, sum := range s.Messages.ListSummaries() {
		if sum.ID == id && strings.TrimSpace(sum.Title) != "" {
			title = strings.Join(strings.Fields(sum.Title), " ")
			break
		}
	}

	doc := exportDocument{
		ConversationID:  id,
		Title:           title,
		ExportedAt:      time.Now().UTC().Format(time.RFC3339),
		IncludeThinking: includeThinking,
		Messages:        []exportMessage{},
	}
	for _, msg := range s.Messages.List(id) {
		item := exportMessage{
			ID:        msg.ID,
			Role:      msg.Role,
			RoleLabel: exportRoleLabel(msg.Role),
			CreatedAt: msg.CreatedAt.UTC().Format(time.RFC3339),
			Content:   strings.TrimSpace(conversation.StripMediaRefs(msg.Content)),
		}
		if includeThinking && strings.TrimSpace(msg.Thinking) != "" {
			item.Thinking = strings.TrimSpace(msg.Thinking)
		}
		for _, ref := range conversation.MediaRefs(msg.Content) {
			item.Attachments = append(item.Attachments, exportAttachment{Filename: ref.Filename})
		}
		doc.Messages = append(doc.Messages, item)
	}

	var (
		body []byte
		ct   string
	)
	switch format {
	case exportFormatJSON:
		body, err = json.MarshalIndent(doc, "", "  ")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		body = append(body, '\n')
		ct = "application/json; charset=utf-8"
	case exportFormatHTML:
		body = []byte(renderExportHTML(doc))
		ct = "text/html; charset=utf-8"
	default:
		body = []byte(renderExportMarkdown(doc))
		ct = "text/markdown; charset=utf-8"
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, exportConversationFilename(id, format)))
	_, _ = w.Write(body)
}

func parseExportOptions(r *http.Request) (exportFormat, bool, error) {
	rawFormat := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	var format exportFormat
	switch rawFormat {
	case "", "markdown", "md":
		format = exportFormatMarkdown
	case "json":
		format = exportFormatJSON
	case "html":
		format = exportFormatHTML
	default:
		return "", false, fmt.Errorf("invalid format %q (use markdown, json or html)", rawFormat)
	}
	include := false
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("include_thinking"))) {
	case "", "0", "false", "no":
	case "1", "true", "yes":
		include = true
	default:
		return "", false, fmt.Errorf("invalid include_thinking (use 1 or 0)")
	}
	return format, include, nil
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

func renderExportMarkdown(doc exportDocument) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", doc.Title)
	fmt.Fprintf(&b, "- Conversation: `%s`\n", doc.ConversationID)
	fmt.Fprintf(&b, "- Exported: %s\n", doc.ExportedAt)
	if doc.IncludeThinking {
		b.WriteString("- Includes thinking: yes\n")
	}
	b.WriteString("\n---\n\n")
	for _, msg := range doc.Messages {
		fmt.Fprintf(&b, "## %s · %s\n\n", msg.RoleLabel, exportDisplayTime(msg.CreatedAt))
		if msg.Content != "" {
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		}
		if len(msg.Attachments) > 0 {
			b.WriteString("**Attachments:** ")
			for i, ref := range msg.Attachments {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString("`" + markdownInline(ref.Filename) + "`")
			}
			b.WriteString("\n\n")
		}
		if msg.Thinking != "" {
			b.WriteString("> **Thinking**\n>\n")
			for _, line := range strings.Split(msg.Thinking, "\n") {
				b.WriteString("> " + line + "\n")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderExportHTML(doc exportDocument) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"zh\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>" + html.EscapeString(doc.Title) + "</title>\n")
	b.WriteString(`<style>
:root { color-scheme: light dark; }
body { margin: 0; background: #f6f7f9; color: #1f2328; font: 15px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
main { max-width: 900px; margin: 0 auto; padding: 32px 20px 64px; }
h1 { font-size: 26px; margin: 0 0 8px; }
.meta { color: #656d76; font-size: 13px; margin: 0 0 28px; }
.msg { background: #fff; border: 1px solid #d8dee4; border-radius: 10px; padding: 16px 18px; margin: 0 0 14px; }
.msg.user { border-left: 4px solid #0969da; }
.msg.assistant { border-left: 4px solid #1a7f37; }
.msg.system_note { border-left: 4px solid #9a6700; }
header { display: flex; justify-content: space-between; gap: 12px; color: #656d76; font-size: 12px; margin-bottom: 8px; }
.content { white-space: pre-wrap; overflow-wrap: anywhere; }
.attachments { margin-top: 12px; font-size: 13px; color: #656d76; }
.attachments ul { margin: 4px 0 0; padding-left: 20px; }
details { margin-top: 12px; color: #656d76; }
details pre { white-space: pre-wrap; overflow-wrap: anywhere; font: inherit; }
@media (prefers-color-scheme: dark) {
  body { background: #0d1117; color: #e6edf3; }
  .msg { background: #161b22; border-color: #30363d; }
  .meta, header, .attachments, details { color: #8b949e; }
}
</style>
</head>
<body>
<main>
`)
	b.WriteString("<h1>" + html.EscapeString(doc.Title) + "</h1>\n")
	b.WriteString("<p class=\"meta\">Conversation: <code>" + html.EscapeString(doc.ConversationID) +
		"</code> · Exported: " + html.EscapeString(doc.ExportedAt))
	if doc.IncludeThinking {
		b.WriteString(" · Includes thinking: yes")
	}
	b.WriteString("</p>\n")
	for _, msg := range doc.Messages {
		role := html.EscapeString(msg.Role)
		b.WriteString("<article class=\"msg " + role + "\">\n<header><span>" +
			html.EscapeString(msg.RoleLabel) + "</span><time>" +
			html.EscapeString(exportDisplayTime(msg.CreatedAt)) + "</time></header>\n")
		if msg.Content != "" {
			b.WriteString("<div class=\"content\">" + html.EscapeString(msg.Content) + "</div>\n")
		}
		if len(msg.Attachments) > 0 {
			b.WriteString("<div class=\"attachments\"><strong>Attachments:</strong><ul>")
			for _, ref := range msg.Attachments {
				b.WriteString("<li>" + html.EscapeString(ref.Filename) + "</li>")
			}
			b.WriteString("</ul></div>\n")
		}
		if msg.Thinking != "" {
			b.WriteString("<details><summary>Thinking</summary><pre>" +
				html.EscapeString(msg.Thinking) + "</pre></details>\n")
		}
		b.WriteString("</article>\n")
	}
	b.WriteString("</main>\n</body>\n</html>\n")
	return b.String()
}

func exportDisplayTime(raw string) string {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func markdownInline(raw string) string {
	return strings.NewReplacer("`", "\\`", "\r", " ", "\n", " ").Replace(raw)
}

// exportConversationFilename keeps the download name portable across OSes.
func exportConversationFilename(id string, format exportFormat) string {
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
	switch format {
	case exportFormatJSON:
		b.WriteString(".json")
	case exportFormatHTML:
		b.WriteString(".html")
	default:
		b.WriteString(".md")
	}
	return b.String()
}
