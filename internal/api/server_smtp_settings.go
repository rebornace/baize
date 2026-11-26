package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/settingscrypto"
)

const smtpSettingsKey = "smtp_settings"

type smtpSettings struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	PasswordSealed string `json:"password_sealed,omitempty"`
	From           string `json:"from"`
	Security       string `json:"security"` // starttls | tls | none
}

type smtpSettingsView struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	From        string `json:"from"`
	Security    string `json:"security"`
	HasPassword bool   `json:"has_password"`
}

func (s *Server) smtpSettings() smtpSettings {
	if s.Store == nil {
		return smtpSettings{}
	}
	raw, ok, err := s.Store.GetSetting(smtpSettingsKey)
	if err != nil || !ok {
		return smtpSettings{}
	}
	var v smtpSettings
	_ = json.Unmarshal(raw, &v)
	if v.Security == "" {
		v.Security = "starttls"
	}
	if v.Port == 0 {
		v.Port = 587
	}
	return v
}

func (s *Server) viewSMTP() smtpSettingsView {
	v := s.smtpSettings()
	if v.Port == 0 {
		v.Port = 587
	}
	if v.Security == "" {
		v.Security = "starttls"
	}
	return smtpSettingsView{
		Host: v.Host, Port: v.Port, Username: v.Username,
		From: v.From, Security: v.Security, HasPassword: v.PasswordSealed != "",
	}
}

func (s *Server) handleGetSMTP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.viewSMTP())
}

func (s *Server) handlePutSMTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host          string `json:"host"`
		Port          int    `json:"port"`
		Username      string `json:"username"`
		Password      string `json:"password"`
		ClearPassword bool   `json:"clear_password"`
		From          string `json:"from"`
		Security      string `json:"security"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	body.Host = strings.TrimSpace(body.Host)
	body.Username = strings.TrimSpace(body.Username)
	body.From = strings.TrimSpace(body.From)
	body.Security = strings.ToLower(strings.TrimSpace(body.Security))
	if body.Security == "" {
		body.Security = "starttls"
	}
	if body.Security != "starttls" && body.Security != "tls" && body.Security != "none" {
		writeError(w, http.StatusBadRequest, "invalid_request", "security must be starttls, tls or none")
		return
	}
	if body.Host == "" || body.Port < 1 || body.Port > 65535 {
		writeError(w, http.StatusBadRequest, "invalid_request", "host and port are required")
		return
	}
	if body.From == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "from is required")
		return
	}
	if _, err := mail.ParseAddress(body.From); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "from must be an email address")
		return
	}

	prev := s.smtpSettings()
	sealed := prev.PasswordSealed
	if body.ClearPassword {
		sealed = ""
	} else if body.Password != "" {
		key, err := settingscrypto.KeyFromEnv()
		if err != nil {
			writeError(w, http.StatusBadRequest, "settings_key_required",
				"BAIZE_SETTINGS_KEY is required to store the SMTP password")
			return
		}
		sealed, err = settingscrypto.Seal(key, body.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	v := smtpSettings{
		Host: body.Host, Port: body.Port, Username: body.Username,
		PasswordSealed: sealed, From: body.From, Security: body.Security,
	}
	raw, _ := json.Marshal(v)
	if err := s.Store.UpsertSetting(smtpSettingsKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.viewSMTP())
}

func (s *Server) smtpPassword() (string, error) {
	v := s.smtpSettings()
	if v.PasswordSealed == "" {
		return "", nil
	}
	key, err := settingscrypto.KeyFromEnv()
	if err != nil {
		return "", err
	}
	return settingscrypto.Open(key, v.PasswordSealed)
}

// buildEmailMessage creates a UTF-8 plain-text message.
func buildEmailMessage(from string, to []string, subject, text string) []byte {
	headers := []string{
		"From: " + from,
		"To: " + strings.Join(to, ", "),
		"Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + text + "\r\n")
}

func (s *Server) sendEmail(ctx context.Context, to []string, subject, text string) error {
	v := s.smtpSettings()
	if v.Host == "" || len(to) == 0 {
		return errors.New("smtp is not configured")
	}
	password, err := s.smtpPassword()
	if err != nil {
		return fmt.Errorf("smtp password: %w", err)
	}
	addr := net.JoinHostPort(v.Host, fmt.Sprintf("%d", v.Port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if v.Security == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			ServerName: v.Host, MinVersion: tls.VersionTLS12,
		})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	c, err := smtp.NewClient(conn, v.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if v.Security == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: v.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		}
	}
	if v.Username != "" {
		auth := smtp.PlainAuth("", v.Username, password, v.Host)
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(v.From); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := c.Rcpt(recipient); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(buildEmailMessage(v.From, to, subject, text)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
