package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// UpstreamModel is one entry returned by an OpenAI-compatible GET /models
// endpoint. Only the fields we surface are decoded; providers may add more.
type UpstreamModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type listModelsResponse struct {
	Object string          `json:"object"`
	Data   []UpstreamModel `json:"data"`
}

// FetchModels queries the OpenAI-compatible catalog at baseURL and returns the
// advertised model IDs. It issues GET {baseURL}/models with the same Bearer
// authorization used for chat completions. A bounded timeout keeps a hanging
// provider from stalling the request. Non-2xx responses and malformed bodies
// are reported so the caller can fall back to manual entry.
func FetchModels(ctx context.Context, baseURL, apiKey string) ([]UpstreamModel, error) {
	base := strings.TrimSpace(strings.TrimRight(baseURL, "/"))
	if base == "" {
		return nil, fmt.Errorf("base_url is required")
	}

	reqCtx := ctx
	if _, hasDeadline := reqCtx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if key := strings.TrimSpace(apiKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("models endpoint returned status %d", resp.StatusCode)
	}

	var parsed listModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	out := make([]UpstreamModel, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			out = append(out, UpstreamModel{
				ID:      id,
				Object:  m.Object,
				Created: m.Created,
				OwnedBy: m.OwnedBy,
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("models endpoint returned no models")
	}
	return out, nil
}
