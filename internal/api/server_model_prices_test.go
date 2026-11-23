package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestModelPricesGetPut(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()

	get := httptest.NewRequest(http.MethodGet, "/v0/settings/model-prices", nil)
	get.Header.Set("Authorization", "Bearer adm")
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get=%d %s", getRR.Code, getRR.Body.String())
	}
	var initial struct {
		Entries  []map[string]any `json:"entries"`
		Defaults map[string]any   `json:"defaults"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if len(initial.Entries) == 0 || initial.Defaults["entries"] == nil {
		t.Fatalf("initial=%+v", initial)
	}

	body := `{"currency":"USD","entries":[{"model":"custom-model","input_per_mtok":1.5,"output_per_mtok":3,"cached_input_per_mtok":0.5}]}`
	if err := srv.Store.PutRunCost(store.RunCost{RunID: "r-stale", Day: "2026-11-23", CostUSD: 9, Priced: true}); err != nil {
		t.Fatal(err)
	}
	put := httptest.NewRequest(http.MethodPut, "/v0/settings/model-prices", bytes.NewBufferString(body))
	put.Header.Set("Authorization", "Bearer adm")
	put.Header.Set("Content-Type", "application/json")
	putRR := httptest.NewRecorder()
	h.ServeHTTP(putRR, put)
	if putRR.Code != http.StatusOK {
		t.Fatalf("put=%d %s", putRR.Code, putRR.Body.String())
	}
	// New prices invalidate costs computed with the old table.
	if _, ok, _ := srv.Store.GetRunCost("r-stale"); ok {
		t.Fatal("cached run cost survived a price change")
	}

	bad := httptest.NewRequest(http.MethodPut, "/v0/settings/model-prices",
		bytes.NewBufferString(`{"entries":[{"model":"x","input_per_mtok":-1}]}`))
	bad.Header.Set("Authorization", "Bearer adm")
	badRR := httptest.NewRecorder()
	h.ServeHTTP(badRR, bad)
	if badRR.Code != http.StatusBadRequest {
		t.Fatalf("bad=%d %s", badRR.Code, badRR.Body.String())
	}
}
