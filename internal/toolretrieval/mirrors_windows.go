//go:build windows

package toolretrieval

import "golang.org/x/sys/windows"

var modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
var procGetUserDefaultUILanguage = modKernel32.NewProc("GetUserDefaultUILanguage")

// systemLocaleChinese is true when the Windows UI language is Chinese
// (e.g. zh-CN), so Ollama installers prefer domestic mirrors.
func systemLocaleChinese() bool {
	r, _, _ := procGetUserDefaultUILanguage.Call()
	const langChinese = 0x04 // PRIMARYLANGID(LANG_CHINESE)
	return r&0x3ff == langChinese
}
