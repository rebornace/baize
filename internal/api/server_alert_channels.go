package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const alertChannelsKey = "alert_channels"
const alertChannelsMax = 20

type alertChannel struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	URL     string   `json:"url"`
	To      []string `json:"to,omitempty"`
	Enabled bool     `json:"enabled"`
}

var alertChannelTypes = map[string]bool{
	"feishu":   true,
	"dingtalk": true,
	"wecom":    true,
	"generic":  true,
	"email":    true,
}

func (s *Server) alertChannels() []alertChannel {
	if s.Store == nil {
		return nil
	}
	raw, ok, err := s.Store.GetSetting(alertChannelsKey)
	if err != nil || !ok {
		return nil
	}
	var list []alertChannel
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	return list
}

func validateAlertChannel(c *alertChannel) error {
	c.Name = strings.TrimSpace(c.Name)
	c.Type = strings.ToLower(strings.TrimSpace(c.Type))
	c.URL = strings.TrimSpace(c.URL)
	if !alertChannelTypes[c.Type] {
		return errors.New("type must be feishu, dingtalk, wecom, generic or email")
	}
	if c.Type == "email" {
		if len(c.To) == 0 {
			return errors.New("email channel requires at least one recipient")
		}
		for _, raw := range c.To {
			if _, err := mail.ParseAddress(strings.TrimSpace(raw)); err != nil {
				return fmt.Errorf("invalid email recipient %q", raw)
			}
		}
		if len([]rune(c.Name)) > 100 {
			return errors.New("name is too long")
		}
		return nil
	}
	u, err := url.Parse(c.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("url must be an http(s) URL")
	}
	if len([]rune(c.Name)) > 100 {
		return errors.New("name is too long")
	}
	return nil
}

func (s *Server) handleGetAlertChannels(w http.ResponseWriter, r *http.Request) {
	items := s.alertChannels()
	if items == nil {
		items = []alertChannel{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "max": alertChannelsMax})
}

func (s *Server) handlePutAlertChannels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []alertChannel `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if len(body.Items) > alertChannelsMax {
		writeError(w, http.StatusBadRequest, "invalid_request", "at most 20 channels")
		return
	}
	seen := map[string]bool{}
	for i := range body.Items {
		c := &body.Items[i]
		if err := validateAlertChannel(c); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("channel %d: %v", i+1, err))
			return
		}
		if c.ID == "" {
			c.ID = "alert_" + uuid.NewString()
		}
		if seen[c.ID] {
			writeError(w, http.StatusBadRequest, "invalid_request", "duplicate channel id")
			return
		}
		seen[c.ID] = true
	}
	raw, _ := json.Marshal(body.Items)
	if err := s.Store.UpsertSetting(alertChannelsKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": body.Items, "max": alertChannelsMax})
}

func (s *Server) handleTestAlertChannel(w http.ResponseWriter, r *http.Request) {
	var body alertChannel
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.ID != "" {
		found := false
		for _, c := range s.alertChannels() {
			if c.ID == body.ID {
				body = c
				found = true
				break
			}
		}
		if !found {
			writeError(w, http.StatusNotFound, "not_found", "alert channel not found")
			return
		}
	}
	if err := validateAlertChannel(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := s.postAlertChannel(r.Context(), body, "alert.test", map[string]any{
		"message": "这是一条来自 Baize 的测试通知。",
	}); err != nil {
		writeError(w, http.StatusBadGateway, "delivery_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "sent"})
}

func alertChannelText(eventType string, data map[string]any) string {
	switch eventType {
	case "budget.alert":
		target := "全局"
		if scope, _ := data["scope"].(string); scope != "" {
			target = scope
		}
		if id, _ := data["target_id"].(string); id != "" {
			target += ":" + id
		}
		return fmt.Sprintf(
			"[Baize] 预算告警\n范围：%s\n周期：%v\n级别：%v\n已用：$%.4f / 预算：$%.4f（%.0f%%）",
			target, data["period"], data["level"],
			floatValue(data["total_usd"]), floatValue(data["budget_usd"]),
			floatValue(data["ratio"])*100,
		)
	case "cost.anomaly":
		scope := fmt.Sprintf("%v", data["scope"])
		if id, _ := data["conversation_id"].(string); id != "" {
			scope += ":" + id
		}
		if id, _ := data["agent_id"].(string); id != "" {
			scope += ":" + id
		}
		if id, _ := data["model_id"].(string); id != "" {
			scope += ":" + id
		}
		return fmt.Sprintf(
			"[Baize] 费用异常\n范围：%s\n今日：$%.4f\n7 日均值：$%.4f\n倍数：%.1fx",
			scope, floatValue(data["today_usd"]), floatValue(data["baseline_usd"]),
			floatValue(data["ratio"]),
		)
	default:
		if msg, _ := data["message"].(string); msg != "" {
			return "[Baize] " + msg
		}
		return "[Baize] " + eventType
	}
}

func floatValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

func alertChannelPayload(ch alertChannel, eventType string, data map[string]any) []byte {
	text := alertChannelText(eventType, data)
	var payload any
	switch ch.Type {
	case "feishu":
		payload = map[string]any{"msg_type": "text", "content": map[string]any{"text": text}}
	case "dingtalk":
		payload = map[string]any{"msgtype": "text", "text": map[string]any{"content": text}}
	case "wecom":
		payload = map[string]any{"msgtype": "text", "text": map[string]any{"content": text}}
	default:
		payload = map[string]any{
			"text":  text,
			"event": map[string]any{"type": eventType, "data": data},
		}
	}
	raw, _ := json.Marshal(payload)
	return raw
}

var alertChannelHTTPClient = &http.Client{Timeout: 5 * time.Second}

func (s *Server) postAlertChannel(
	ctx context.Context, ch alertChannel, eventType string, data map[string]any,
) error {
	text := alertChannelText(eventType, data)
	if ch.Type == "email" {
		return s.sendEmail(ctx, ch.To, "Baize 告警", text)
	}
	body := alertChannelPayload(ch, eventType, data)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ch.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := alertChannelHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return fmt.Errorf("%s returned HTTP %d", ch.Type, res.StatusCode)
	}
	return nil
}

// sendAlertChannels fans an alert out to every enabled channel. Failures are
// logged but never affect budget evaluation.
func (s *Server) sendAlertChannels(ctx context.Context, eventType string, data map[string]any) {
	for _, ch := range s.alertChannels() {
		if !ch.Enabled {
			continue
		}
		if err := s.postAlertChannel(ctx, ch, eventType, data); err != nil {
			log.Printf("alert channel %s (%s): %v", ch.ID, ch.Type, err)
		}
	}
}
