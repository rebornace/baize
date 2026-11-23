package toolretrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultOllamaBaseURL = "http://127.0.0.1:11434"
	DefaultEmbedModel    = "bge-m3"
)

// ProbeOllama reports whether a local Ollama API answers.
// Tries the given base, then localhost / 127.0.0.1 variants (Windows quirks).
func ProbeOllama(ctx context.Context, baseURL string) error {
	bases := ollamaProbeBases(baseURL)
	var last error
	for _, base := range bases {
		if err := probeOllamaOnce(ctx, base); err != nil {
			last = err
			continue
		}
		return nil
	}
	if last == nil {
		return fmt.Errorf("ollama unreachable")
	}
	return last
}

func ollamaProbeBases(baseURL string) []string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultOllamaBaseURL
	}
	out := []string{base}
	// Prefer both loopback spellings when probing the default port.
	if strings.Contains(base, "127.0.0.1:11434") {
		out = append(out, "http://localhost:11434")
	} else if strings.Contains(base, "localhost:11434") {
		out = append(out, "http://127.0.0.1:11434")
	} else if base == DefaultOllamaBaseURL {
		out = append(out, "http://localhost:11434")
	}
	return out
}

func probeOllamaOnce(ctx context.Context, base string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/tags", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("ollama status %d", res.StatusCode)
	}
	return nil
}

// ProbeEmbedding checks that model can embed a short probe string via /v1/embeddings.
// apiKey is optional (local Ollama / open gateways often omit auth).
func ProbeEmbedding(ctx context.Context, openAIBase, model, apiKey string) error {
	base := NormalizeOpenAIBase(openAIBase)
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultEmbedModel
	}
	payload, _ := json.Marshal(map[string]any{
		"model": model,
		"input": []string{"tool retrieval probe"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/embeddings", bytes.NewReader(payload))
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
		return fmt.Errorf("embeddings status %d: %s", res.StatusCode, truncate(string(body), 200))
	}
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("embeddings decode: %w", err)
	}
	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return fmt.Errorf("embeddings: empty vector")
	}
	return nil
}

// HasModel reports whether model is already present in the local Ollama library.
func HasModel(ctx context.Context, ollamaBase, model string) bool {
	base := strings.TrimRight(strings.TrimSpace(ollamaBase), "/")
	if base == "" {
		base = DefaultOllamaBaseURL
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultEmbedModel
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/tags", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return false
	}
	var parsed struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if json.NewDecoder(res.Body).Decode(&parsed) != nil {
		return false
	}
	want := strings.ToLower(model)
	for _, m := range parsed.Models {
		name := strings.ToLower(m.Name)
		if name == want || strings.HasPrefix(name, want+":") {
			return true
		}
	}
	return false
}

// RemoveModel deletes a model from the local Ollama library (ollama rm).
func RemoveModel(ctx context.Context, ollamaBase, model string) error {
	base := strings.TrimRight(strings.TrimSpace(ollamaBase), "/")
	if base == "" {
		base = DefaultOllamaBaseURL
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultEmbedModel
	}
	payload, _ := json.Marshal(map[string]any{"name": model})
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/api/delete", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 120 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Already absent is OK for cleanup.
		msg := strings.ToLower(string(body))
		if strings.Contains(msg, "not found") || res.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("delete model status %d: %s", res.StatusCode, truncate(string(body), 200))
	}
	return nil
}

// PullModel streams Ollama /api/pull until success or error.
func PullModel(ctx context.Context, ollamaBase, model string, onProgress func(InstallProgress)) error {
	base := strings.TrimRight(strings.TrimSpace(ollamaBase), "/")
	if base == "" {
		base = DefaultOllamaBaseURL
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultEmbedModel
	}
	payload, _ := json.Marshal(map[string]any{"name": model, "stream": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/pull", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 0} // long pull
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("pull status %d: %s", res.StatusCode, truncate(string(b), 200))
	}
	dec := json.NewDecoder(res.Body)
	for {
		var line struct {
			Status    string `json:"status"`
			Error     string `json:"error"`
			Completed int64  `json:"completed"`
			Total     int64  `json:"total"`
		}
		if err := dec.Decode(&line); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if line.Error != "" {
			return fmt.Errorf("pull: %s", classifyPullError(line.Error))
		}
		if onProgress != nil {
			onProgress(InstallProgress{
				Event:  "download",
				Mirror: "model",
				Bytes:  line.Completed,
				Total:  line.Total,
				Note:   line.Status,
			})
		}
		if line.Status == "success" {
			return nil
		}
	}
	return nil
}

// InstallerHint describes how the current OS installs Ollama without a CLI.
type InstallerHint struct {
	GOOS            string   `json:"goos"`
	Mode            string   `json:"mode"`                    // "windows_exe" | "open_download_page"
	InstallerURL    string   `json:"installer_url,omitempty"` // first mirror for this host (CN or official)
	MirrorURLs      []string `json:"mirror_urls,omitempty"`
	DownloadPageURL string   `json:"download_page_url"`
}

func InstallerForGOOS() InstallerHint {
	h := InstallerHint{
		GOOS:            runtime.GOOS,
		DownloadPageURL: preferredDownloadPage(),
		Mode:            "open_download_page",
	}
	if runtime.GOOS == "windows" {
		mirrors := windowsInstallerMirrors()
		h.Mode = "windows_exe"
		if len(mirrors) > 0 {
			h.InstallerURL = mirrors[0].URL
			for _, m := range mirrors {
				h.MirrorURLs = append(h.MirrorURLs, m.URL)
			}
		}
	}
	return h
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// classifyPullError maps known Ollama pull failures to stable codes the UI can
// localize. Unrecognized messages are returned unchanged.
func classifyPullError(msg string) string {
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "requires a newer version of ollama") ||
		(strings.Contains(lower, "412") && strings.Contains(lower, "newer version")) {
		return "ollama_version_too_old"
	}
	return msg
}

// OpenAIBaseFromOllama converts http://host:11434 → http://host:11434/v1.
func OpenAIBaseFromOllama(ollamaBase string) string {
	return NormalizeOpenAIBase(ollamaBase)
}

// NormalizeOpenAIBase ensures an OpenAI-compatible root ending in /v1.
// Accepts either the vendor root (…/v1) or a bare host (appends /v1).
func NormalizeOpenAIBase(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultOllamaBaseURL
	}
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	return base + "/v1"
}
