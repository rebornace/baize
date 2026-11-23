package api

import (
	"errors"
	"net/http"

	"github.com/rebornace/baize/internal/settingscrypto"
)

const settingsKeyRequiredMsg = "set BAIZE_SETTINGS_KEY to store secrets"

// writeIfSettingsKeyRequired maps settingscrypto.ErrNoKey to HTTP 400.
func writeIfSettingsKeyRequired(w http.ResponseWriter, err error) bool {
	if errors.Is(err, settingscrypto.ErrNoKey) {
		writeError(w, http.StatusBadRequest, "settings_key_required", settingsKeyRequiredMsg)
		return true
	}
	return false
}

// writeIfSealedSecretError maps missing-key errors when reading sealed settings.
func writeIfSealedSecretError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, settingscrypto.ErrNoKey) || errors.Is(err, settingscrypto.ErrCiphertext) {
		writeError(w, http.StatusBadRequest, "settings_key_required", settingsKeyRequiredMsg)
		return true
	}
	return false
}
