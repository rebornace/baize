//go:build !windows

package toolretrieval

// systemLocaleChinese: non-Windows relies on LANG/LC_* (see preferChineseMirrors).
func systemLocaleChinese() bool {
	return false
}
