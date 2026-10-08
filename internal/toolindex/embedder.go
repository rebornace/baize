package toolindex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// Embedder turns text into dense vectors for Tool-RAG. Implementations are
// typically OpenAI-compatible /embeddings endpoints or local multilingual
// models (bge-m3, e5, …). Nil embedder ⇒ lexical BM25-only fallback.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// OpenAIEmbedder calls an OpenAI-compatible POST /embeddings API.
type OpenAIEmbedder struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// NewOpenAIEmbedder builds an embedder. baseURL should be the API root (…/v1).
func NewOpenAIEmbedder(baseURL, apiKey, model string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  apiKey,
		Model:   strings.TrimSpace(model),
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// Embed implements Embedder.
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e == nil || e.Model == "" || e.BaseURL == "" {
		return nil, fmt.Errorf("toolindex: embedding model not configured")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(embedRequest{Model: e.Model, Input: texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
	}
	client := e.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("embeddings: status %d: %s", res.StatusCode, truncate(string(body), 256))
	}
	var parsed embedResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("embeddings: decode: %w", err)
	}
	out := make([][]float32, len(texts))
	for _, row := range parsed.Data {
		if row.Index < 0 || row.Index >= len(out) {
			continue
		}
		out[row.Index] = l2normalize(row.Embedding)
	}
	for i, v := range out {
		if v == nil {
			return nil, fmt.Errorf("embeddings: missing vector for input %d", i)
		}
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func l2normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum <= 0 {
		return v
	}
	inv := float32(1 / math.Sqrt(sum))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x * inv
	}
	return out
}

func cosineDense(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot // L2-normalized ⇒ cosine
}
