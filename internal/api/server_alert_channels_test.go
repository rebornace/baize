package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAlertChannelsCRUD(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPut, "/v0/settings/alert-channels",
		bytes.NewBufferString(`{"items":[{"name":"Ops","type":"feishu","url":"https://example.com/hook","enabled":true}]}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	var saved struct {
		Items []alertChannel `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Items) != 1 || saved.Items[0].ID == "" {
		t.Fatalf("items=%+v", saved.Items)
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/settings/alert-channels", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	var list struct {
		Items []alertChannel `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil || len(list.Items) != 1 {
		t.Fatalf("get=%+v err=%v", list.Items, err)
	}

	for _, payload := range []string{
		`{"items":[{"name":"x","type":"email","url":"https://example.com","enabled":true}]}`,
		`{"items":[{"name":"x","type":"generic","url":"ftp://example.com","enabled":true}]}`,
	} {
		bad := httptest.NewRequest(http.MethodPut, "/v0/settings/alert-channels", bytes.NewBufferString(payload))
		bad.Header.Set("Authorization", "Bearer adm")
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, bad)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("bad %s=%d", payload, rr.Code)
		}
	}
}

func TestAlertChannelsDelivery(t *testing.T) {
	var body []byte
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	srv := gateServer(t, "op", "adm")
	raw, _ := json.Marshal([]alertChannel{{
		ID: "a1", Name: "generic", Type: "generic", URL: target.URL, Enabled: true,
	}})
	if err := srv.Store.UpsertSetting(alertChannelsKey, raw); err != nil {
		t.Fatal(err)
	}
	srv.sendAlertChannels(t.Context(), "budget.alert", map[string]any{
		"scope": "day", "period": "2026-11-26", "level": "warning",
		"total_usd": 1.5, "budget_usd": 2.0, "ratio": 0.75,
	})
	var payload struct {
		Text  string `json:"text"`
		Event struct {
			Type string `json:"type"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("body=%q err=%v", body, err)
	}
	if payload.Event.Type != "budget.alert" || !strings.Contains(payload.Text, "预算告警") {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestAlertChannelPayloadShapes(t *testing.T) {
	data := map[string]any{"message": "ping"}
	cases := map[string]string{
		"feishu":   `"msg_type":"text"`,
		"dingtalk": `"msgtype":"text"`,
		"wecom":    `"msgtype":"text"`,
		"generic":  `"event"`,
	}
	for typ, want := range cases {
		raw := string(alertChannelPayload(alertChannel{Type: typ}, "alert.test", data))
		if !strings.Contains(raw, want) {
			t.Fatalf("%s payload=%s", typ, raw)
		}
	}
}

func TestSMTPSettingsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	get := httptest.NewRequest(http.MethodGet, "/v0/settings/smtp", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d", rr.Code)
	}
	var view smtpSettingsView
	if err := json.Unmarshal(rr.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Port != 587 || view.Security != "starttls" {
		t.Fatalf("view=%+v", view)
	}
	put := httptest.NewRequest(http.MethodPut, "/v0/settings/smtp",
		bytes.NewBufferString(`{"host":"smtp.example.com","port":587,"from":"alerts@example.com","security":"starttls"}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	bad := httptest.NewRequest(http.MethodPut, "/v0/settings/smtp",
		bytes.NewBufferString(`{"host":"smtp.example.com","port":587,"from":"alerts@example.com","security":"ssl"}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad=%d", rr.Code)
	}
}

func TestEmailAlertChannelValidation(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	put := func(payload string) int {
		req := httptest.NewRequest(http.MethodPut, "/v0/settings/alert-channels", bytes.NewBufferString(payload))
		req.Header.Set("Authorization", "Bearer adm")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}
	ok := `{"items":[{"name":"Mail","type":"email","to":["ops@example.com"],"enabled":true}]}`
	if code := put(ok); code != http.StatusOK {
		t.Fatalf("email channel=%d", code)
	}
	if code := put(`{"items":[{"name":"Mail","type":"email","to":["not-an-email"],"enabled":true}]}`); code != http.StatusBadRequest {
		t.Fatalf("bad email=%d", code)
	}
	if msg := string(buildEmailMessage("a@example.com", []string{"b@example.com"}, "hi", "body")); !strings.Contains(msg, "Subject:") || !strings.Contains(msg, "body") {
		t.Fatalf("message=%q", msg)
	}
}
