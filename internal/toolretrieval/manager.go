package toolretrieval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/toolindex"
)

// SettingsRW is the subset of store.Store used to persist tool-retrieval prefs.
type SettingsRW interface {
	GetSetting(key string) (jsonRaw []byte, ok bool, err error)
	UpsertSetting(key string, jsonRaw []byte) error
}

// Phase is the foolproof enablement progress shown in the UI.
type Phase string

const (
	PhaseIdle               Phase = "idle"
	PhaseChecking           Phase = "checking"
	PhaseDownloadingInstall Phase = "downloading_installer"
	PhaseLaunchingInstall   Phase = "launching_installer"
	PhaseWaitingOllama      Phase = "waiting_ollama"
	PhasePullingModel       Phase = "pulling_model"
	PhaseProbing            Phase = "probing"
	PhaseReady              Phase = "ready"
	PhaseFailed             Phase = "failed"
	PhaseDisabled           Phase = "disabled"
)

// Provider selects how dense embeddings are obtained.
const (
	ProviderLocal = "local" // foolproof Ollama on this machine
	ProviderAPI   = "api"   // OpenAI-compatible HTTP (cloud or self-hosted)
)

// Status is the GET /settings/tool-retrieval payload.
type Status struct {
	Mode                string            `json:"mode"` // "standard" | "enhanced"
	Phase               Phase             `json:"phase"`
	Detail              string            `json:"detail,omitempty"`
	Error               string            `json:"error,omitempty"`
	Provider            string            `json:"provider"`         // "local" | "api"
	OllamaInstalled     bool              `json:"ollama_installed"` // app/binary present on disk
	OllamaRunning       bool              `json:"ollama_running"`   // API answering
	EmbeddingOK         bool              `json:"embedding_ok"`
	Model               string            `json:"model"`
	BaseURL             string            `json:"base_url"` // OpenAI-compatible …/v1 root in use
	OllamaBaseURL       string            `json:"ollama_base_url"`
	OpenAIBaseURL       string            `json:"openai_base_url"`
	APIKeySet           bool              `json:"api_key_set"`
	ModelPresent        bool              `json:"model_present"`                   // local Ollama library has the embed model
	InstallerCacheBytes int64             `json:"installer_cache_bytes,omitempty"` // baize temp OllamaSetup cache
	Paths               LocalPaths        `json:"paths"`
	Installer           InstallerHint     `json:"installer"`
	Download            *DownloadProgress `json:"download,omitempty"`
	Busy                bool              `json:"busy"`
}

// CleanupOptions controls optional removal of the Ollama application itself.
type CleanupOptions struct {
	RemoveOllama bool `json:"remove_ollama"`
}

// CleanupReport summarizes what Cleanup removed.
type CleanupReport struct {
	Disabled            bool     `json:"disabled"`
	RemovedModel        bool     `json:"removed_model"`
	RemovedOllama       bool     `json:"removed_ollama"`
	InstallerCacheBytes int64    `json:"installer_cache_bytes_removed"`
	ClearedSetting      bool     `json:"cleared_setting"`
	Notes               []string `json:"notes,omitempty"`
}

// EnableAPIRequest is the body for cloud / self-hosted OpenAI-compatible enablement.
type EnableAPIRequest struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
}

type persisted struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider,omitempty"` // "local" | "api"; empty ⇒ local
	Model    string `json:"model"`
	// BaseURL is the OpenAI-compatible root (…/v1) for provider=api.
	BaseURL string `json:"base_url,omitempty"`
	// OllamaBaseURL is the Ollama root (no /v1) for provider=local.
	OllamaBaseURL string `json:"ollama_base_url,omitempty"`
	APIKey        string `json:"api_key,omitempty"`
	// ModelsDir overrides Ollama model storage (injected as OLLAMA_MODELS when we start it).
	ModelsDir string `json:"models_dir,omitempty"`
}

// ApplyEmbedder hot-swaps the engine embedder (nil = standard/lexical).
type ApplyEmbedder func(emb toolindex.Embedder)

// Manager orchestrates Ollama GUI install + model pull + engine wiring,
// or OpenAI-compatible remote/self-hosted embedding APIs.
type Manager struct {
	Store     SettingsRW
	Apply     ApplyEmbedder
	Model     string
	Base      string // ollama root for local, default DefaultOllamaBaseURL
	APIURL    string // openai-compatible …/v1 for api provider
	APIKey    string
	Prov      string // ProviderLocal | ProviderAPI
	ModelsDir string // optional override → OLLAMA_MODELS when starting Ollama

	mu       sync.Mutex
	phase    Phase
	detail   string
	errMsg   string
	busy     bool
	download *DownloadProgress
	cancel   context.CancelFunc
}

// NewManager builds a manager. Call Restore on bootstrap.
func NewManager(st SettingsRW, apply ApplyEmbedder) *Manager {
	return &Manager{
		Store: st,
		Apply: apply,
		Model: DefaultEmbedModel,
		Base:  DefaultOllamaBaseURL,
		Prov:  ProviderLocal,
		phase: PhaseDisabled,
	}
}

// Restore loads persisted preference and applies embedder if ready.
func (m *Manager) Restore(ctx context.Context) {
	if m == nil {
		return
	}
	p := m.load()
	m.ModelsDir = p.ModelsDir
	if !p.Enabled {
		m.setPhase(PhaseDisabled, "", "")
		if m.Apply != nil {
			m.Apply(nil)
		}
		return
	}
	m.applyPersisted(p)
	if m.provider() == ProviderAPI {
		m.restoreAPI(ctx)
		return
	}
	m.restoreLocal(ctx)
}

func (m *Manager) restoreLocal(ctx context.Context) {
	if err := ProbeOllama(ctx, m.Base); err != nil {
		m.setPhase(PhaseFailed, "", "ollama_not_running")
		if m.Apply != nil {
			m.Apply(nil)
		}
		return
	}
	openAI := OpenAIBaseFromOllama(m.Base)
	if err := ProbeEmbedding(ctx, openAI, m.Model, ""); err != nil {
		m.setPhase(PhaseFailed, "", err.Error())
		if m.Apply != nil {
			m.Apply(nil)
		}
		return
	}
	if m.Apply != nil {
		m.Apply(toolindex.NewOpenAIEmbedder(openAI, "", m.Model))
	}
	m.setPhase(PhaseReady, "", "")
}

func (m *Manager) restoreAPI(ctx context.Context) {
	base := m.openaiBase()
	if err := ProbeEmbedding(ctx, base, m.Model, m.APIKey); err != nil {
		m.setPhase(PhaseFailed, "", err.Error())
		if m.Apply != nil {
			m.Apply(nil)
		}
		return
	}
	if m.Apply != nil {
		m.Apply(toolindex.NewOpenAIEmbedder(base, m.APIKey, m.Model))
	}
	m.setPhase(PhaseReady, "", "")
}

// Snapshot returns current status for the UI (never returns the raw API key).
func (m *Manager) Snapshot(ctx context.Context) Status {
	if m == nil {
		return Status{
			Mode: "standard", Phase: PhaseDisabled, Provider: ProviderLocal,
			Installer: InstallerForGOOS(), Model: DefaultEmbedModel,
			Paths: DiscoverPaths(""),
		}
	}
	m.mu.Lock()
	phase, detail, errMsg, busy := m.phase, m.detail, m.errMsg, m.busy
	var dlCopy *DownloadProgress
	if m.download != nil {
		cp := *m.download
		dlCopy = &cp
	}
	model, base, apiURL, apiKey, prov, modelsDir := m.Model, m.Base, m.APIURL, m.APIKey, m.provider(), m.ModelsDir
	m.mu.Unlock()
	if modelsDir == "" {
		if p := m.load(); p.ModelsDir != "" {
			modelsDir = p.ModelsDir
			m.mu.Lock()
			m.ModelsDir = modelsDir
			m.mu.Unlock()
		}
	}

	openAI := OpenAIBaseFromOllama(base)
	if prov == ProviderAPI {
		openAI = NormalizeOpenAIBase(apiURL)
	}

	ollamaOK := false
	embedOK := false
	modelPresent := false
	installed := OllamaInstalled()
	if prov == ProviderAPI {
		embedOK = ProbeEmbedding(ctx, openAI, model, apiKey) == nil
	} else {
		ollamaOK = ProbeOllama(ctx, base) == nil
		if ollamaOK {
			installed = true
			modelPresent = HasModel(ctx, base, model)
			if modelPresent {
				embedOK = ProbeEmbedding(ctx, openAI, model, "") == nil
			}
		}
	}
	mode := "standard"
	if phase == PhaseReady && embedOK {
		mode = "enhanced"
	}
	return Status{
		Mode:                mode,
		Phase:               phase,
		Detail:              detail,
		Error:               errMsg,
		Provider:            prov,
		OllamaInstalled:     installed,
		OllamaRunning:       ollamaOK,
		EmbeddingOK:         embedOK,
		Model:               model,
		BaseURL:             openAI,
		OllamaBaseURL:       base,
		OpenAIBaseURL:       openAI,
		APIKeySet:           strings.TrimSpace(apiKey) != "",
		ModelPresent:        modelPresent,
		InstallerCacheBytes: InstallerCacheBytes(),
		Paths:               DiscoverPaths(modelsDir),
		Installer:           InstallerForGOOS(),
		Download:            dlCopy,
		Busy:                busy,
	}
}

// StartEnable begins the local (Ollama) foolproof flow in the background.
func (m *Manager) StartEnable(parent context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return errBusy
	}
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	m.busy = true
	m.errMsg = ""
	m.Prov = ProviderLocal
	m.mu.Unlock()

	go m.runEnable(ctx)
	return nil
}

// EnableAPI probes an OpenAI-compatible embeddings endpoint and enables enhanced matching.
// Empty apiKey keeps the previously stored key when one exists.
func (m *Manager) EnableAPI(ctx context.Context, req EnableAPIRequest) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return errBusy
	}
	base := NormalizeOpenAIBase(req.BaseURL)
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = DefaultEmbedModel
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		key = m.APIKey // keep previous
	}
	m.busy = true
	m.errMsg = ""
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.busy = false
		m.mu.Unlock()
	}()

	m.setPhase(PhaseProbing, "", "")
	if err := ProbeEmbedding(ctx, base, model, key); err != nil {
		m.setPhase(PhaseFailed, "", err.Error())
		return err
	}
	m.mu.Lock()
	m.Prov = ProviderAPI
	m.APIURL = base
	m.Model = model
	m.APIKey = key
	m.mu.Unlock()
	if m.Apply != nil {
		m.Apply(toolindex.NewOpenAIEmbedder(base, key, model))
	}
	if err := m.save(persisted{
		Enabled:  true,
		Provider: ProviderAPI,
		Model:    model,
		BaseURL:  base,
		APIKey:   key,
	}); err != nil {
		m.setPhase(PhaseFailed, "", err.Error())
		return err
	}
	m.setPhase(PhaseReady, "", "")
	return nil
}

// Cleanup disables enhanced matching, removes the local embed model from Ollama
// (when reachable), clears baize's installer download cache, and clears the
// persisted preference. When opts.RemoveOllama is set, also uninstalls the
// desktop app (Windows) and leftover data dirs.
func (m *Manager) Cleanup(ctx context.Context, opts CleanupOptions) (CleanupReport, error) {
	rep := CleanupReport{}
	if m == nil {
		return rep, nil
	}
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.busy = false
	model, base := m.Model, m.Base
	if model == "" {
		model = DefaultEmbedModel
	}
	m.mu.Unlock()

	if m.Apply != nil {
		m.Apply(nil)
	}
	rep.Disabled = true

	if !opts.RemoveOllama {
		if ProbeOllama(ctx, base) == nil {
			if HasModel(ctx, base, model) {
				if err := RemoveModel(ctx, base, model); err != nil {
					rep.Notes = append(rep.Notes, "remove_model: "+err.Error())
				} else {
					rep.RemovedModel = true
				}
			} else {
				rep.Notes = append(rep.Notes, "model_already_absent")
			}
		} else {
			rep.Notes = append(rep.Notes, "ollama_not_running_skip_model_delete")
		}
	} else {
		if err := UninstallOllama(ctx); err != nil {
			rep.Notes = append(rep.Notes, "remove_ollama: "+err.Error())
		} else {
			rep.RemovedOllama = true
			rep.RemovedModel = true
		}
	}

	n, err := ClearInstallerCache()
	if err != nil {
		rep.Notes = append(rep.Notes, "installer_cache: "+err.Error())
	} else {
		rep.InstallerCacheBytes = n
	}

	if err := m.save(persisted{}); err != nil {
		return rep, err
	}
	rep.ClearedSetting = true
	m.mu.Lock()
	m.Prov = ProviderLocal
	m.APIKey = ""
	m.APIURL = ""
	m.ModelsDir = ""
	m.download = nil
	m.mu.Unlock()
	m.setPhase(PhaseDisabled, "", "")
	return rep, nil
}

// SetModelsDir persists a custom Ollama models directory and restarts the local
// app with OLLAMA_MODELS so new pulls land there. Empty dir clears the override.
func (m *Manager) SetModelsDir(ctx context.Context, dir string) error {
	if m == nil {
		return nil
	}
	dir = strings.TrimSpace(dir)
	if dir != "" {
		dir = filepath.Clean(dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.ModelsDir = dir
	enabled := m.phase == PhaseReady
	model, base, apiURL, apiKey, prov := m.Model, m.Base, m.APIURL, m.APIKey, m.provider()
	m.mu.Unlock()

	p := m.load()
	p.ModelsDir = dir
	p.Enabled = enabled || p.Enabled
	p.Model = model
	p.Provider = prov
	p.BaseURL = apiURL
	p.OllamaBaseURL = base
	p.APIKey = apiKey
	if err := m.save(p); err != nil {
		return err
	}
	if OllamaInstalled() || ProbeOllama(ctx, base) == nil {
		stopLocalOllama()
		_ = tryStartLocalOllama(ctx, base, dir, nil)
	}
	return nil
}

// Disable turns off enhanced matching (lexical fallback) and persists.
func (m *Manager) Disable() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.busy = false
	model, base, apiURL, apiKey, prov := m.Model, m.Base, m.APIURL, m.APIKey, m.provider()
	m.mu.Unlock()
	if m.Apply != nil {
		m.Apply(nil)
	}
	p := persisted{
		Enabled:       false,
		Provider:      prov,
		Model:         model,
		BaseURL:       apiURL,
		OllamaBaseURL: base,
		APIKey:        apiKey,
	}
	if err := m.save(p); err != nil {
		return err
	}
	m.setPhase(PhaseDisabled, "", "")
	return nil
}

func (m *Manager) runEnable(ctx context.Context) {
	defer func() {
		m.mu.Lock()
		m.busy = false
		m.mu.Unlock()
	}()

	m.setPhase(PhaseChecking, "", "")
	base, model, modelsDir := m.Base, m.Model, m.ModelsDir

	if err := ProbeOllama(ctx, base); err != nil {
		installed := OllamaInstalled()
		if installed {
			// Already installed — only start the app; never re-download the installer.
			m.setPhase(PhaseWaitingOllama, "starting_local_ollama", "")
			if !tryStartLocalOllama(ctx, base, modelsDir, func(s string) {
				m.setPhase(PhaseWaitingOllama, s, "")
			}) {
				m.setPhase(PhaseFailed, "", "ollama_installed_but_not_running")
				return
			}
		} else {
			m.setPhase(PhaseDownloadingInstall, "downloading_installer", "")
			if _, err := downloadAndLaunchInstaller(ctx, func(p InstallProgress) {
				m.applyInstallProgress(p)
			}); err != nil {
				if ctx.Err() != nil {
					m.setPhase(PhaseFailed, "", "cancelled")
					return
				}
				m.setPhase(PhaseFailed, "", err.Error())
				return
			}
			if ctx.Err() != nil {
				m.setPhase(PhaseFailed, "", "cancelled")
				return
			}
			m.setPhase(PhaseWaitingOllama, "waiting_for_installer", "")
			deadline := time.Now().Add(15 * time.Minute)
			var lastGUIStart time.Time
			for time.Now().Before(deadline) {
				if ctx.Err() != nil {
					m.setPhase(PhaseFailed, "", "cancelled")
					return
				}
				if ProbeOllama(ctx, base) == nil {
					break
				}
				if time.Since(lastGUIStart) >= 20*time.Second {
					_ = tryStartLocalOllama(ctx, base, modelsDir, nil)
					lastGUIStart = time.Now()
				}
				time.Sleep(2 * time.Second)
			}
			if ProbeOllama(ctx, base) != nil {
				m.setPhase(PhaseFailed, "", "ollama_not_detected_after_install")
				return
			}
		}
	}

	m.setPhase(PhasePullingModel, model, "")
	if err := PullModel(ctx, base, model, func(p InstallProgress) {
		m.mu.Lock()
		m.phase = PhasePullingModel
		m.detail = p.Note
		if p.Note == "" {
			m.detail = model
		}
		m.download = &DownloadProgress{
			Mirror:  "model",
			Bytes:   p.Bytes,
			Total:   p.Total,
			Percent: downloadPercent(p.Bytes, p.Total),
			Note:    p.Note,
		}
		m.mu.Unlock()
	}); err != nil {
		m.setPhase(PhaseFailed, "", err.Error())
		return
	}

	m.setPhase(PhaseProbing, "", "")
	openAI := OpenAIBaseFromOllama(base)
	if err := ProbeEmbedding(ctx, openAI, model, ""); err != nil {
		m.setPhase(PhaseFailed, "", err.Error())
		return
	}
	m.mu.Lock()
	m.Prov = ProviderLocal
	m.mu.Unlock()
	if m.Apply != nil {
		m.Apply(toolindex.NewOpenAIEmbedder(openAI, "", model))
	}
	_ = m.save(persisted{
		Enabled:       true,
		Provider:      ProviderLocal,
		Model:         model,
		OllamaBaseURL: base,
		ModelsDir:     modelsDir,
	})
	m.setPhase(PhaseReady, "", "")
}

func (m *Manager) setPhase(p Phase, detail, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.phase = p
	m.detail = detail
	m.errMsg = errMsg
	if p != PhaseDownloadingInstall && p != PhaseLaunchingInstall && p != PhasePullingModel {
		m.download = nil
	}
}

func (m *Manager) applyInstallProgress(p InstallProgress) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch p.Event {
	case "switch_mirror":
		m.phase = PhaseDownloadingInstall
		m.detail = "switch_mirror"
		m.download = &DownloadProgress{
			Mirror:  p.Mirror,
			Bytes:   0,
			Total:   0,
			Percent: 0,
			Note:    p.Note,
		}
	case "download":
		m.phase = PhaseDownloadingInstall
		m.detail = "downloading_installer"
		m.download = &DownloadProgress{
			Mirror:  p.Mirror,
			Bytes:   p.Bytes,
			Total:   p.Total,
			Percent: downloadPercent(p.Bytes, p.Total),
			Note:    p.Note,
		}
	case "launch":
		m.phase = PhaseLaunchingInstall
		m.detail = "launching_installer"
		m.download = nil
	case "open_page":
		m.phase = PhaseLaunchingInstall
		m.detail = "open_download_page"
		m.download = &DownloadProgress{Note: p.Note}
	}
}

func (m *Manager) provider() string {
	if m.Prov == ProviderAPI {
		return ProviderAPI
	}
	return ProviderLocal
}

func (m *Manager) openaiBase() string {
	if m.provider() == ProviderAPI {
		return NormalizeOpenAIBase(m.APIURL)
	}
	return OpenAIBaseFromOllama(m.Base)
}

func (m *Manager) applyPersisted(p persisted) {
	if p.Model != "" {
		m.Model = p.Model
	}
	prov := p.Provider
	if prov == "" {
		if strings.TrimSpace(p.BaseURL) != "" && strings.TrimSpace(p.OllamaBaseURL) == "" {
			prov = ProviderAPI
		} else {
			prov = ProviderLocal
		}
	}
	m.Prov = prov
	if p.OllamaBaseURL != "" {
		m.Base = p.OllamaBaseURL
	}
	if p.BaseURL != "" {
		m.APIURL = NormalizeOpenAIBase(p.BaseURL)
	}
	m.APIKey = p.APIKey
	m.ModelsDir = p.ModelsDir
}

func (m *Manager) load() persisted {
	if m.Store == nil {
		return persisted{}
	}
	raw, ok, err := m.Store.GetSetting(store.SettingKeyToolRetrieval)
	if err != nil || !ok || len(raw) == 0 {
		return persisted{}
	}
	var p persisted
	_ = json.Unmarshal(raw, &p)
	return p
}

func (m *Manager) save(p persisted) error {
	if m.Store == nil {
		return nil
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return m.Store.UpsertSetting(store.SettingKeyToolRetrieval, raw)
}

var errBusy = errString("tool_retrieval_busy")

type errString string

func (e errString) Error() string { return string(e) }
