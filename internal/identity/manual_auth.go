package identity

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// JWT-shaped tokens (three base64url segments). Min length avoids matching short dotted words.
	jwtPattern = regexp.MustCompile(`[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)
	tokenLine  = regexp.MustCompile(`(?i)^\s*(?:authorization|token|access[_-]?token)\s*[:：]\s*(.+)\s*$`)
	tokenLead  = regexp.MustCompile(`(?i)(?:我直接给你|给你|这是|这是我的|使用|用)?\s*(?:的)?\s*(?:authorization|token|access[_-]?token|jwt)\s*[:：]?\s*`)
)

const SessionAuthReadyHint = "【系统】会话访问凭证已就绪（用户已提供 Token，已写入会话鉴权）。请直接调用业务 API 完成用户请求，不要要求用户名/密码登录，也不要声称无法使用该 Token。"

// CredentialFromUserToken normalizes a pasted token or Authorization header value.
func CredentialFromUserToken(raw string) (headers map[string]string, subject string, claims map[string]any) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", nil
	}
	authValue := buildAuthHeader("", extractBareToken(raw))
	if authValue == "" {
		return nil, "", nil
	}
	claims = ParseJWTClaimsSummary(authValue)
	subject = ""
	if sub, _ := claims["sub"].(string); sub != "" {
		subject = sub
	} else if email, _ := claims["email"].(string); email != "" {
		subject = email
	} else if uid := claims["userId"]; uid != nil {
		subject = fmt.Sprintf("userId:%v", uid)
	}
	if subject == "" {
		subject = "manual"
	}
	return map[string]string{"Authorization": authValue}, subject, claims
}

func extractBareToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if m := tokenLine.FindStringSubmatch(raw); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	if strings.HasPrefix(strings.ToLower(raw), "authorization:") {
		return strings.TrimSpace(raw[len("authorization:"):])
	}
	if jwt := jwtPattern.FindString(raw); jwt != "" {
		return jwt
	}
	return raw
}

// ExtractAndStripToken removes an embedded session token from user text and returns
// credential headers when a token was found. JWTs may appear mid-sentence.
func ExtractAndStripToken(input string) (cleaned string, headers map[string]string, ok bool) {
	if jwt := jwtPattern.FindString(input); jwt != "" {
		h, _, _ := CredentialFromUserToken(jwt)
		if h == nil {
			return input, nil, false
		}
		cleaned = stripTokenResidue(strings.ReplaceAll(input, jwt, " "))
		return cleaned, h, true
	}

	lines := strings.Split(input, "\n")
	var kept []string
	var found string
	for _, line := range lines {
		lineTrim := strings.TrimSpace(line)
		if lineTrim == "" {
			kept = append(kept, line)
			continue
		}
		if m := tokenLine.FindStringSubmatch(lineTrim); len(m) == 2 {
			found = strings.TrimSpace(m[1])
			continue
		}
		if strings.HasPrefix(strings.ToLower(lineTrim), "authorization:") {
			found = strings.TrimSpace(lineTrim[len("authorization:"):])
			continue
		}
		kept = append(kept, line)
	}
	cleaned = strings.TrimSpace(strings.Join(kept, "\n"))
	if found == "" {
		return input, nil, false
	}
	h, _, _ := CredentialFromUserToken(found)
	if h == nil {
		return input, nil, false
	}
	cleaned = stripTokenResidue(cleaned)
	return cleaned, h, true
}

func stripTokenResidue(s string) string {
	s = tokenLead.ReplaceAllString(s, " ")
	s = regexp.MustCompile(`[ \t]{2,}`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// UpsertManualToken stores a pasted token as the default bearer identity for the conversation.
func UpsertManualToken(store Store, conversationID, raw, label string) (string, error) {
	headers, subject, claims := CredentialFromUserToken(raw)
	if headers == nil {
		return "", fmt.Errorf("invalid session token")
	}
	return upsertManualIdentity(store, conversationID, headers, subject, claims, label)
}

func upsertManualIdentity(store Store, conversationID string, headers map[string]string, subject string, claims map[string]any, label string) (string, error) {
	lab := strings.TrimSpace(label)
	if lab == "" {
		lab = "临时 Token"
		if subject != "" && subject != "manual" {
			lab = subject
		}
	}
	return store.Upsert(conversationID, Identity{
		Label:             lab,
		Scheme:            "bearer",
		CredentialHeaders: headers,
		Source:            SourceManual,
		Subject:           subject,
		ClaimsSummary:     claims,
		IsDefault:         true,
	})
}

// PrepareSessionAuth applies session_token and/or embedded message tokens before a chat run.
// When a token is applied, cleanedInput includes SessionAuthReadyHint so the model does not ask to log in.
func PrepareSessionAuth(store Store, conversationID, input, sessionToken string) (cleanedInput, identityID string, err error) {
	cleaned := input
	applied := false
	var id string
	if tok := strings.TrimSpace(sessionToken); tok != "" {
		id, err = UpsertManualToken(store, conversationID, tok, "")
		if err != nil {
			return input, "", err
		}
		applied = true
	}
	if c, headers, ok := ExtractAndStripToken(cleaned); ok {
		subject := "manual"
		claims := ParseJWTClaimsSummary(headers["Authorization"])
		if sub, _ := claims["sub"].(string); sub != "" {
			subject = sub
		} else if uid := claims["userId"]; uid != nil {
			subject = fmt.Sprintf("userId:%v", uid)
		}
		newID, err := upsertManualIdentity(store, conversationID, headers, subject, claims, "")
		if err != nil {
			return input, id, err
		}
		if id == "" {
			id = newID
		}
		cleaned = c
		applied = true
	}
	if applied {
		cleaned = withSessionAuthHint(cleaned)
	}
	return cleaned, id, nil
}

func withSessionAuthHint(cleaned string) string {
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return SessionAuthReadyHint + "\n请按用户意图调用 API。"
	}
	if strings.Contains(cleaned, SessionAuthReadyHint) {
		return cleaned
	}
	return cleaned + "\n\n" + SessionAuthReadyHint
}

// ConversationHasSessionAuth reports whether the conversation already has a usable identity.
func ConversationHasSessionAuth(store Store, conversationID string) bool {
	if store == nil || conversationID == "" {
		return false
	}
	return len(store.List(conversationID)) > 0
}
