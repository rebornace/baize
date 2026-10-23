package com.rebornace.baize.mobilert

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * MOBILE-RT M2 key vault (Android).
 * API keys never persist in the Go mobileagent package — only here.
 * iOS counterpart: Keychain (document only until Mac build).
 */
class CredentialStore(context: Context) {
    private val prefs = EncryptedSharedPreferences.create(
        context,
        "baize_mobilert_vault",
        MasterKey.Builder(context).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )

    fun save(apiBase: String, apiKey: String, model: String, configURL: String) {
        prefs.edit()
            .putString(KEY_BASE, apiBase)
            .putString(KEY_API, apiKey)
            .putString(KEY_MODEL, model)
            .putString(KEY_CFG, configURL)
            .apply()
    }

    fun apiBase(): String = prefs.getString(KEY_BASE, "") ?: ""
    fun apiKey(): String = prefs.getString(KEY_API, "") ?: ""
    fun model(): String = prefs.getString(KEY_MODEL, "") ?: ""
    fun configURL(): String = prefs.getString(KEY_CFG, "") ?: ""

    fun clearKey() {
        prefs.edit().remove(KEY_API).apply()
    }

    companion object {
        private const val KEY_BASE = "api_base"
        private const val KEY_API = "api_key"
        private const val KEY_MODEL = "model"
        private const val KEY_CFG = "config_url"
    }
}
