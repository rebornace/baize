//go:build !windows

package toolretrieval

func resolveAppDir() string {
	return DefaultAppDir()
}
