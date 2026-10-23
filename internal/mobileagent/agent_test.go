package mobileagent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestChatOnceRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "pong"}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	a := New()
	if err := a.Configure(srv.URL+"/v1", "k", "m"); err != nil {
		t.Fatal(err)
	}
	out, err := a.ChatOnce("ping")
	if err != nil {
		t.Fatal(err)
	}
	if out != "pong" {
		t.Fatalf("got %q", out)
	}
}

func TestSendGetTimeNoHITL(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id":   "c1",
							"type": "function",
							"function": map[string]string{
								"name":      "get_time",
								"arguments": "{}",
							},
						}},
					},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{"role": "assistant", "content": "time-ok"},
			}},
		})
	}))
	t.Cleanup(srv.Close)

	a := New()
	_ = a.Configure(srv.URL, "k", "m")
	raw := a.Send("what time?")
	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "done" || resp["text"] != "time-ok" {
		t.Fatalf("unexpected %v", resp)
	}
	if a.FetchHits() != 0 {
		t.Fatalf("fetch hits %d", a.FetchHits())
	}
}

func TestSendHTTPGetNeedsApprovalNoNetwork(t *testing.T) {
	var fetchHits atomic.Int32
	fetchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchHits.Add(1)
		_, _ = w.Write([]byte("secret-body"))
	}))
	t.Cleanup(fetchSrv.Close)

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		args, _ := json.Marshal(map[string]string{"url": fetchSrv.URL})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []map[string]any{{
						"id":   "c-http",
						"type": "function",
						"function": map[string]any{
							"name":      "http_get",
							"arguments": string(args),
						},
					}},
				},
			}},
		})
	}))
	t.Cleanup(llm.Close)

	a := New()
	_ = a.Configure(llm.URL, "k", "m")
	raw := a.Send("fetch it")
	var resp map[string]any
	_ = json.Unmarshal([]byte(raw), &resp)
	if resp["status"] != "needs_approval" {
		t.Fatalf("got %v", resp)
	}
	if fetchHits.Load() != 0 || a.FetchHits() != 0 {
		t.Fatal("network should not run before Approve")
	}
	if resp["call_id"] != "c-http" || resp["name"] != "http_get" {
		t.Fatalf("fields %v", resp)
	}
}

func TestApproveHTTPGetThenDone(t *testing.T) {
	fetchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello-body"))
	}))
	t.Cleanup(fetchSrv.Close)

	var llmCalls atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := llmCalls.Add(1)
		if n == 1 {
			args, _ := json.Marshal(map[string]string{"url": fetchSrv.URL})
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id":   "c-http",
							"type": "function",
							"function": map[string]any{
								"name":      "http_get",
								"arguments": string(args),
							},
						}},
					},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{"role": "assistant", "content": "got-body"},
			}},
		})
	}))
	t.Cleanup(llm.Close)

	a := New()
	_ = a.Configure(llm.URL, "k", "m")
	raw := a.Send("go")
	var pause map[string]any
	_ = json.Unmarshal([]byte(raw), &pause)
	if pause["status"] != "needs_approval" {
		t.Fatal(raw)
	}
	raw = a.Approve(pause["call_id"].(string))
	var done map[string]any
	_ = json.Unmarshal([]byte(raw), &done)
	if done["status"] != "done" || done["text"] != "got-body" {
		t.Fatalf("%v", done)
	}
	if a.FetchHits() != 1 {
		t.Fatalf("fetch hits %d", a.FetchHits())
	}
}

func TestDenyHTTPGetNoNetwork(t *testing.T) {
	var fetchHits atomic.Int32
	fetchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchHits.Add(1)
	}))
	t.Cleanup(fetchSrv.Close)

	var llmCalls atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := llmCalls.Add(1)
		if n == 1 {
			args, _ := json.Marshal(map[string]string{"url": fetchSrv.URL})
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id":   "c-deny",
							"type": "function",
							"function": map[string]any{
								"name":      "http_get",
								"arguments": string(args),
							},
						}},
					},
				}},
			})
			return
		}
		// Second call: ensure tool result contains denied_by_user
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		found := false
		for _, m := range req.Messages {
			if m.Role == "tool" && strings.Contains(m.Content, "denied_by_user") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected denied tool result in history")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{"role": "assistant", "content": "denied-ok"},
			}},
		})
	}))
	t.Cleanup(llm.Close)

	a := New()
	_ = a.Configure(llm.URL, "k", "m")
	raw := a.Send("go")
	var pause map[string]any
	_ = json.Unmarshal([]byte(raw), &pause)
	raw = a.Deny(pause["call_id"].(string))
	var done map[string]any
	_ = json.Unmarshal([]byte(raw), &done)
	if done["status"] != "done" || done["text"] != "denied-ok" {
		t.Fatalf("%v", done)
	}
	if fetchHits.Load() != 0 || a.FetchHits() != 0 {
		t.Fatal("deny must not hit network")
	}
}

func TestPackageLevelSendApprove(t *testing.T) {
	fetchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	t.Cleanup(fetchSrv.Close)
	var n atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			args, _ := json.Marshal(map[string]string{"url": fetchSrv.URL})
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id": "p1", "type": "function",
							"function": map[string]any{"name": "http_get", "arguments": string(args)},
						}},
					},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "ok"}}},
		})
	}))
	t.Cleanup(llm.Close)
	Reset()
	_ = Configure(llm.URL, "k", "m")
	raw := Send("x")
	var pause map[string]any
	_ = json.Unmarshal([]byte(raw), &pause)
	raw = Approve(pause["call_id"].(string))
	if !strings.Contains(raw, `"done"`) {
		t.Fatal(raw)
	}
}
