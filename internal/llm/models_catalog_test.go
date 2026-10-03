package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchModelsSuccessAndAuth(t *testing.T) {
	var gotAuth, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "m-1", "owned_by": "team-a"},
				{"id": "  "}, // dropped: blank
				{"id": "m-2"},
			},
		})
	}))
	defer srv.Close()

	models, err := FetchModels(context.Background(), srv.URL+"/v1/", "sk-secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "m-1" || models[1].ID != "m-2" {
		t.Fatalf("unexpected models: %+v", models)
	}
	if models[0].OwnedBy != "team-a" {
		t.Fatalf("owned_by not decoded: %+v", models[0])
	}
	if gotAuth != "Bearer sk-secret" {
		t.Fatalf("auth header wrong: %q", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Fatalf("accept header wrong: %q", gotAccept)
	}
}

func TestFetchModelsErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := FetchModels(context.Background(), srv.URL, "k"); err == nil {
		t.Fatal("want error on 403")
	}
}

func TestFetchModelsEmptyBaseURL(t *testing.T) {
	if _, err := FetchModels(context.Background(), "  ", "k"); err == nil {
		t.Fatal("want error on empty base url")
	}
}
