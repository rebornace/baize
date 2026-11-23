package decide

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSystemOneDisabledWithoutBaseURL(t *testing.T) {
	s := NewSystemOne("", "", "", nil)
	if s.Enabled() {
		t.Fatal("empty base URL must disable SystemOne")
	}
	_, err := s.Ask(context.Background(), Question{Kind: KindMemoryExtract, Context: "x", OnFail: VerdictYes})
	if err != ErrUnavailable {
		t.Fatalf("err=%v want ErrUnavailable", err)
	}
}

func TestSystemOneEmptyModelDefaultsToTev1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req systemOneRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != DefaultSystemOneModel {
			t.Fatalf("model=%q want %q", req.Model, DefaultSystemOneModel)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"verdict": map[string]any{"noul": 0.8}},
		})
	}))
	defer srv.Close()

	s := NewSystemOne(srv.URL, "", "", srv.Client())
	if _, err := s.Ask(context.Background(), Question{Kind: KindMemoryExtract, Context: "x", OnFail: VerdictNo}); err != nil {
		t.Fatal(err)
	}
}

func TestSystemOneNoulBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("auth=%q", r.Header.Get("Authorization"))
		}
		var req systemOneRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "tev1" || req.Questions["verdict"].Type != "noul" {
			t.Fatalf("req=%+v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"verdict": map[string]any{"noul": 0.91}},
		})
	}))
	defer srv.Close()

	s := NewSystemOne(srv.URL, "secret", "tev1", srv.Client())
	ans, err := s.Ask(context.Background(), Question{Kind: KindMemoryExtract, Context: "user likes tea"})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Verdict != VerdictYes || ans.Source != SourceSystemOne {
		t.Fatalf("ans=%+v", ans)
	}
}

func TestSystemOneNoulNo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"verdict": map[string]any{"noul": 0.12}},
		})
	}))
	defer srv.Close()
	s := NewSystemOne(srv.URL, "", "", srv.Client())
	ans, err := s.Ask(context.Background(), Question{Kind: KindPruneToolResult, Context: "noise"})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Verdict != VerdictNo {
		t.Fatalf("ans=%+v", ans)
	}
}

func TestSystemOneChoiceRouteTier(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req systemOneRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		q := req.Questions["pick"]
		if q.Type != "choice" {
			t.Fatalf("type=%s", q.Type)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"pick": map[string]any{"choice": "power", "confidence": 0.8}},
		})
	}))
	defer srv.Close()
	s := NewSystemOne(srv.URL, "", "", srv.Client())
	ans, err := s.Ask(context.Background(), Question{
		Kind:    KindRouteTier,
		Context: "design a distributed cache",
		Options: []string{"light", "power"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Value != "power" || ans.Source != SourceSystemOne {
		t.Fatalf("ans=%+v", ans)
	}
}

func TestSystemOnePickManyViaNoul(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "opt_mall") {
			t.Fatalf("body=%s", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				"opt_mall":  map[string]any{"noul": 0.88},
				"opt_pets":  map[string]any{"noul": 0.1},
				"opt_other": map[string]any{"noul": 0.6},
			},
		})
	}))
	defer srv.Close()
	s := NewSystemOne(srv.URL, "", "", srv.Client())
	ans, err := s.Ask(context.Background(), Question{
		Kind:    KindSystemTargets,
		Context: "查询商城订单",
		Options: []string{"mall", "pets", "other"},
		Descriptions: map[string]string{
			"mall": "ecommerce",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ans.Values) != 2 || ans.Values[0] != "mall" || ans.Values[1] != "other" {
		t.Fatalf("values=%v", ans.Values)
	}
}

func TestSystemOneHTTPErrorUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	s := NewSystemOne(srv.URL, "", "", srv.Client())
	_, err := s.Ask(context.Background(), Question{Kind: KindMemoryExtract, Context: "x"})
	if err != ErrUnavailable {
		t.Fatalf("err=%v", err)
	}
}

func TestSystemOneRejectsUnknownChoice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"pick": map[string]any{"choice": "turbo"}},
		})
	}))
	defer srv.Close()
	s := NewSystemOne(srv.URL, "", "", srv.Client())
	_, err := s.Ask(context.Background(), Question{
		Kind: KindRouteTier, Options: []string{"light", "power"}, Context: "x",
	})
	if err != ErrUnavailable {
		t.Fatalf("err=%v", err)
	}
}
