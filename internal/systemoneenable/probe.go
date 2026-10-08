package systemoneenable

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/toolretrieval"
)

const (
	DefaultOllamaBaseURL = toolretrieval.DefaultOllamaBaseURL
	DefaultModel         = decide.DefaultSystemOneModel // tev1
	systemOnePath        = "/v1/systemone"
)

// NormalizeSystemOneBase returns the service root (no /v1/systemone suffix).
// Empty input defaults to local Ollama.
func NormalizeSystemOneBase(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return DefaultOllamaBaseURL
	}
	for _, suf := range []string{"/v1/systemone", "/systemone"} {
		if strings.HasSuffix(strings.ToLower(base), suf) {
			base = strings.TrimRight(base[:len(base)-len(suf)], "/")
			break
		}
	}
	return base
}

// ProbeSystemOne checks that baseURL answers POST /v1/systemone with a tiny noul.
// Empty model defaults to DefaultModel (tev1). apiKey is optional.
func ProbeSystemOne(ctx context.Context, baseURL, model, apiKey string) error {
	base := NormalizeSystemOneBase(baseURL)
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultModel
	}
	payload, _ := json.Marshal(map[string]any{
		"model": model,
		"state": "baize systemone probe",
		"questions": map[string]any{
			"probe": map[string]any{
				"type":         "noul",
				"instructions": "Is this a connectivity probe?",
			},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+systemOnePath, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if k := strings.TrimSpace(apiKey); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("systemone status %d: %s", res.StatusCode, truncate(string(body), 200))
	}
	var parsed struct {
		Answers map[string]json.RawMessage `json:"answers"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("systemone decode: %w", err)
	}
	if len(parsed.Answers) == 0 {
		return fmt.Errorf("systemone: empty answers")
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
