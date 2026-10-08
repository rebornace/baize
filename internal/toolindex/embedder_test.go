package toolindex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIEmbedder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float32{3, 4}},
				{"index": 1, "embedding": []float32{0, 1}},
			},
		})
	}))
	defer srv.Close()

	emb := NewOpenAIEmbedder(srv.URL, "k", "text-embedding-3-small")
	vecs, err := emb.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 {
		t.Fatalf("len=%d", len(vecs))
	}
	// 3-4-5 triangle ⇒ unit vector (0.6, 0.8)
	if vecs[0][0] < 0.59 || vecs[0][0] > 0.61 || vecs[0][1] < 0.79 || vecs[0][1] > 0.81 {
		t.Fatalf("normalized[0]=%v", vecs[0])
	}
}
