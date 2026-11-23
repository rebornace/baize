package toolretrieval

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LocalPaths describes where Ollama app / models / baize caches live.
type LocalPaths struct {
	AppDir            string `json:"app_dir,omitempty"`
	ConfigDir         string `json:"config_dir,omitempty"`
	ModelsDir         string `json:"models_dir"`
	ModelsDirSource   string `json:"models_dir_source"` // "baize" | "env" | "default"
	InstallerCacheDir string `json:"installer_cache_dir"`
}

// DefaultModelsDir is the platform default used when neither baize nor env overrides.
func DefaultModelsDir() string {
	switch runtime.GOOS {
	case "windows":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".ollama", "models")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".ollama", "models")
	default:
		// Official Linux package default.
		return "/usr/share/ollama/.ollama/models"
	}
}

// DefaultConfigDir is the Ollama user config root (keys, etc.).
func DefaultConfigDir() string {
	if runtime.GOOS == "linux" {
		return "/usr/share/ollama/.ollama"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ollama")
}

// DefaultAppDir is the desktop install location when known.
func DefaultAppDir() string {
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			return ""
		}
		return filepath.Join(local, "Programs", "Ollama")
	}
	return ""
}

// ResolveModelsDir picks baize override, then OLLAMA_MODELS, then platform default.
func ResolveModelsDir(baizeOverride string) (dir, source string) {
	if d := strings.TrimSpace(baizeOverride); d != "" {
		return filepath.Clean(d), "baize"
	}
	if d := strings.TrimSpace(os.Getenv("OLLAMA_MODELS")); d != "" {
		return filepath.Clean(d), "env"
	}
	return DefaultModelsDir(), "default"
}

// DiscoverPaths returns current path info for the UI.
func DiscoverPaths(baizeModelsDir string) LocalPaths {
	models, src := ResolveModelsDir(baizeModelsDir)
	return LocalPaths{
		AppDir:            resolveAppDir(),
		ConfigDir:         DefaultConfigDir(),
		ModelsDir:         models,
		ModelsDirSource:   src,
		InstallerCacheDir: InstallerCacheDir(),
	}
}
