package systemoneenable

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/toolretrieval"
)

// SettingsRW is the subset of store used to persist enablement prefs.
type SettingsRW interface {
	GetSetting(key string) (jsonRaw []byte, ok bool, err error)
	UpsertSetting(key string, jsonRaw []byte) error
}

// Phase matches toolretrieval phases so the UI can share labels.
type Phase = toolretrieval.Phase

const (
	PhaseIdle               = toolretrieval.PhaseIdle
	PhaseChecking           = toolretrieval.PhaseChecking
	PhaseDownloadingInstall = toolretrieval.PhaseDownloadingInstall
	PhaseLaunchingInstall   = toolretrieval.PhaseLaunchingInstall
	PhaseWaitingOllama      = toolretrieval.PhaseWaitingOllama
	PhasePullingModel       = toolretrieval.PhasePullingModel
	PhaseProbing            = toolretrieval.PhaseProbing
	PhaseReady              = toolretrieval.PhaseReady
	PhaseFailed             = toolretrieval.PhaseFailed
	PhaseDisabled           = toolretrieval.PhaseDisabled
)

const (
	ProviderLocal = toolretrieval.ProviderLocal
	ProviderAPI   = toolretrieval.ProviderAPI
)

// Config is applied to runtimecfg when enablement succeeds or is cleared.
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

// ApplyConfig hot-swaps System One knobs (empty BaseURL disables).
type ApplyConfig func(cfg Config)

// Status is the GET /settings/systemone payload.
type Status struct {
	Mode            string                          `json:"mode"` // "off" | "on"
	Phase           Phase                           `json:"phase"`
	Detail          string                          `json:"detail,omitempty"`
	Error           string                          `json:"error,omitempty"`
	Provider        string                          `json:"provider"`
	OllamaInstalled bool                            `json:"ollama_installed"`
	OllamaRunning   bool                            `json:"ollama_running"`
	SystemOneOK     bool                            `json:"systemone_ok"`
	Model           string                          `json:"model"`
	BaseURL         string                          `json:"base_url"`
	APIKeySet       bool                            `json:"api_key_set"`
	ModelPresent    bool                            `json:"model_present"`
	Installer       toolretrieval.InstallerHint     `json:"installer"`
	Download        *toolretrieval.DownloadProgress `json:"download,omitempty"`
	Busy            bool                            `json:"busy"`
}

// EnableAPIRequest is the body for cloud / self-hosted System One enablement.
type EnableAPIRequest struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
}

// CleanupReport summarizes what Cleanup removed.
type CleanupReport struct {
	Disabled       bool     `json:"disabled"`
	RemovedModel   bool     `json:"removed_model"`
	ClearedSetting bool     `json:"cleared_setting"`
	Notes          []string `json:"notes,omitempty"`
}

type persisted struct {
	Enabled       bool   `json:"enabled"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model"`
	BaseURL       string `json:"base_url,omitempty"`        // API root
	OllamaBaseURL string `json:"ollama_base_url,omitempty"` // local root
	APIKey        string `json:"api_key,omitempty"`
}

// Manager orchestrates local Ollama + tev1 pull, or remote System One API.
type Manager struct {
	Store  SettingsRW
	Apply  ApplyConfig
	Model  string
	Base   string // ollama root for local
	APIURL string
	APIKey string
	Prov   string

	mu       sync.Mutex
	phase    Phase
	detail   string
	errMsg   string
	busy     bool
	download *toolretrieval.DownloadProgress
	cancel   context.CancelFunc
}

// NewManager builds a manager. Call Restore on bootstrap.
func NewManager(st SettingsRW, apply ApplyConfig) *Manager {
	return &Manager{
		Store: st,
		Apply: apply,
		Model: DefaultModel,
		Base:  DefaultOllamaBaseURL,
		Prov:  ProviderLocal,
		phase: PhaseDisabled,
	}
}

// Restore loads persisted preference and applies knobs if ready.
func (m *Manager) Restore(ctx context.Context) {
	if m == nil {
		return
	}
	p := m.load()
	if !p.Enabled {
		m.setPhase(PhaseDisabled, "", "")
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
	if err := toolretrieval.ProbeOllama(ctx, m.Base); err != nil {
		m.setPhase(PhaseFailed, "", "ollama_not_running")
		return
	}
	if err := ProbeSystemOne(ctx, m.Base, m.Model, ""); err != nil {
		m.setPhase(PhaseFailed, "", err.Error())
		return
	}
	m.applyLive(Config{BaseURL: m.Base, Model: m.Model})
	m.setPhase(PhaseReady, "", "")
}

func (m *Manager) restoreAPI(ctx context.Context) {
	base := NormalizeSystemOneBase(m.APIURL)
	if err := ProbeSystemOne(ctx, base, m.Model, m.APIKey); err != nil {
		m.setPhase(PhaseFailed, "", err.Error())
		return
	}
	m.applyLive(Config{BaseURL: base, APIKey: m.APIKey, Model: m.Model})
	m.setPhase(PhaseReady, "", "")
}

func (m *Manager) applyLive(cfg Config) {
	if m.Apply != nil {
		m.Apply(cfg)
	}
}

// Snapshot returns current status (never echoes the raw API key).
func (m *Manager) Snapshot(ctx context.Context) Status {
	if m == nil {
		return Status{
			Mode: "off", Phase: PhaseDisabled, Provider: ProviderLocal,
			Installer: toolretrieval.InstallerForGOOS(), Model: DefaultModel,
		}
	}
	m.mu.Lock()
	phase, detail, errMsg, busy := m.phase, m.detail, m.errMsg, m.busy
	var dlCopy *toolretrieval.DownloadProgress
	if m.download != nil {
		cp := *m.download
		dlCopy = &cp
	}
	model, base, apiURL, apiKey, prov := m.Model, m.Base, m.APIURL, m.APIKey, m.provider()
	m.mu.Unlock()

	displayBase := base
	if prov == ProviderAPI {
		displayBase = NormalizeSystemOneBase(apiURL)
	}

	ollamaOK := false
	systemOK := false
	modelPresent := false
	installed := toolretrieval.OllamaInstalled()
	if prov == ProviderAPI {
		systemOK = ProbeSystemOne(ctx, displayBase, model, apiKey) == nil
	} else {
		ollamaOK = toolretrieval.ProbeOllama(ctx, base) == nil
		if ollamaOK {
			installed = true
			modelPresent = toolretrieval.HasModel(ctx, base, model)
			if modelPresent {
				systemOK = ProbeSystemOne(ctx, base, model, "") == nil
			}
		}
	}
	mode := "off"
	if phase == PhaseReady && systemOK {
		mode = "on"
	}
	return Status{
		Mode:            mode,
		Phase:           phase,
		Detail:          detail,
		Error:           errMsg,
		Provider:        prov,
		OllamaInstalled: installed,
		OllamaRunning:   ollamaOK,
		SystemOneOK:     systemOK,
		Model:           model,
		BaseURL:         displayBase,
		APIKeySet:       strings.TrimSpace(apiKey) != "",
		ModelPresent:    modelPresent,
		Installer:       toolretrieval.InstallerForGOOS(),
		Download:        dlCopy,
		Busy:            busy,
	}
}

// StartEnable begins the local (Ollama + tev1) foolproof flow in the background.
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
	if m.Model == "" {
		m.Model = DefaultModel
	}
	m.mu.Unlock()

	go m.runEnable(ctx)
	return nil
}

// EnableAPI probes a System One endpoint and enables the decision backend.
func (m *Manager) EnableAPI(ctx context.Context, req EnableAPIRequest) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return errBusy
	}
	base := NormalizeSystemOneBase(req.BaseURL)
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = DefaultModel
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		key = m.APIKey
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
	if err := ProbeSystemOne(ctx, base, model, key); err != nil {
		m.setPhase(PhaseFailed, "", err.Error())
		return err
	}
	m.mu.Lock()
	m.Prov = ProviderAPI
	m.APIURL = base
	m.Model = model
	m.APIKey = key
	m.mu.Unlock()
	m.applyLive(Config{BaseURL: base, APIKey: key, Model: model})
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

// Disable turns off System One (clears knobs) and persists.
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
	m.applyLive(Config{})
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

// Cleanup disables, removes local tev1 when reachable, clears persist. Does not uninstall Ollama.
func (m *Manager) Cleanup(ctx context.Context) (CleanupReport, error) {
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
		model = DefaultModel
	}
	m.mu.Unlock()

	m.applyLive(Config{})
	rep.Disabled = true

	if toolretrieval.ProbeOllama(ctx, base) == nil {
		if toolretrieval.HasModel(ctx, base, model) {
			if err := toolretrieval.RemoveModel(ctx, base, model); err != nil {
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

	if err := m.save(persisted{}); err != nil {
		return rep, err
	}
	rep.ClearedSetting = true
	m.mu.Lock()
	m.Prov = ProviderLocal
	m.APIKey = ""
	m.APIURL = ""
	m.download = nil
	m.Model = DefaultModel
	m.mu.Unlock()
	m.setPhase(PhaseDisabled, "", "")
	return rep, nil
}

func (m *Manager) runEnable(ctx context.Context) {
	defer func() {
		m.mu.Lock()
		m.busy = false
		m.mu.Unlock()
	}()

	m.setPhase(PhaseChecking, "", "")
	base, model := m.Base, m.Model
	if model == "" {
		model = DefaultModel
	}
	modelsDir := modelsDirFromToolRetrieval(m.Store)

	if err := toolretrieval.ProbeOllama(ctx, base); err != nil {
		installed := toolretrieval.OllamaInstalled()
		if installed {
			m.setPhase(PhaseWaitingOllama, "starting_local_ollama", "")
			if !toolretrieval.TryStartLocalOllama(ctx, base, modelsDir, func(s string) {
				m.setPhase(PhaseWaitingOllama, s, "")
			}) {
				m.setPhase(PhaseFailed, "", "ollama_installed_but_not_running")
				return
			}
		} else {
			m.setPhase(PhaseDownloadingInstall, "downloading_installer", "")
			if _, err := toolretrieval.DownloadAndLaunchInstaller(ctx, func(p toolretrieval.InstallProgress) {
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
				if toolretrieval.ProbeOllama(ctx, base) == nil {
					break
				}
				if time.Since(lastGUIStart) >= 20*time.Second {
					_ = toolretrieval.TryStartLocalOllama(ctx, base, modelsDir, nil)
					lastGUIStart = time.Now()
				}
				time.Sleep(2 * time.Second)
			}
			if toolretrieval.ProbeOllama(ctx, base) != nil {
				m.setPhase(PhaseFailed, "", "ollama_not_detected_after_install")
				return
			}
		}
	}

	m.setPhase(PhasePullingModel, model, "")
	if err := toolretrieval.PullModel(ctx, base, model, func(p toolretrieval.InstallProgress) {
		m.mu.Lock()
		m.phase = PhasePullingModel
		m.detail = p.Note
		if p.Note == "" {
			m.detail = model
		}
		m.download = &toolretrieval.DownloadProgress{
			Mirror:  "model",
			Bytes:   p.Bytes,
			Total:   p.Total,
			Percent: toolretrieval.DownloadPercent(p.Bytes, p.Total),
			Note:    p.Note,
		}
		m.mu.Unlock()
	}); err != nil {
		m.setPhase(PhaseFailed, "", err.Error())
		return
	}

	m.setPhase(PhaseProbing, "", "")
	if err := ProbeSystemOne(ctx, base, model, ""); err != nil {
		m.setPhase(PhaseFailed, "", err.Error())
		return
	}
	m.mu.Lock()
	m.Prov = ProviderLocal
	m.Model = model
	m.mu.Unlock()
	m.applyLive(Config{BaseURL: base, Model: model})
	_ = m.save(persisted{
		Enabled:       true,
		Provider:      ProviderLocal,
		Model:         model,
		OllamaBaseURL: base,
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

func (m *Manager) applyInstallProgress(p toolretrieval.InstallProgress) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch p.Event {
	case "switch_mirror":
		m.phase = PhaseDownloadingInstall
		m.detail = "switch_mirror"
		m.download = &toolretrieval.DownloadProgress{
			Mirror: p.Mirror, Note: p.Note,
		}
	case "download":
		m.phase = PhaseDownloadingInstall
		m.detail = "downloading_installer"
		m.download = &toolretrieval.DownloadProgress{
			Mirror: p.Mirror, Bytes: p.Bytes, Total: p.Total,
			Percent: toolretrieval.DownloadPercent(p.Bytes, p.Total), Note: p.Note,
		}
	case "launch":
		m.phase = PhaseLaunchingInstall
		m.detail = "launching_installer"
		m.download = nil
	case "open_page":
		m.phase = PhaseLaunchingInstall
		m.detail = "open_download_page"
		m.download = &toolretrieval.DownloadProgress{Note: p.Note}
	}
}

func (m *Manager) provider() string {
	if m.Prov == ProviderAPI {
		return ProviderAPI
	}
	return ProviderLocal
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
		m.APIURL = NormalizeSystemOneBase(p.BaseURL)
	}
	m.APIKey = p.APIKey
}

func (m *Manager) load() persisted {
	if m.Store == nil {
		return persisted{}
	}
	raw, ok, err := m.Store.GetSetting(store.SettingKeySystemOneEnable)
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
	return m.Store.UpsertSetting(store.SettingKeySystemOneEnable, raw)
}

func modelsDirFromToolRetrieval(st SettingsRW) string {
	if st == nil {
		return ""
	}
	raw, ok, err := st.GetSetting(store.SettingKeyToolRetrieval)
	if err != nil || !ok || len(raw) == 0 {
		return ""
	}
	var p struct {
		ModelsDir string `json:"models_dir"`
	}
	_ = json.Unmarshal(raw, &p)
	return strings.TrimSpace(p.ModelsDir)
}

var errBusy = errString("systemone_busy")

type errString string

func (e errString) Error() string { return string(e) }
