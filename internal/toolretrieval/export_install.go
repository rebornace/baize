package toolretrieval

import "context"

// TryStartLocalOllama attempts to start a local Ollama app/serve process.
// Used by System One enablement so decision and matching share one installer path.
func TryStartLocalOllama(ctx context.Context, baseURL, modelsDir string, onProgress func(string)) bool {
	return tryStartLocalOllama(ctx, baseURL, modelsDir, onProgress)
}

// DownloadAndLaunchInstaller runs the OS-specific Ollama GUI install flow.
func DownloadAndLaunchInstaller(ctx context.Context, onProgress func(InstallProgress)) (string, error) {
	return downloadAndLaunchInstaller(ctx, onProgress)
}

// DownloadPercent maps bytes/total to 0–100 for UI progress bars.
func DownloadPercent(bytes, total int64) int {
	return downloadPercent(bytes, total)
}
