import Foundation
import Security

/// MOBILE-RT M2 key vault (iOS).
/// API keys never persist in the Go mobileagent package — only Keychain.
/// Android counterpart: EncryptedSharedPreferences in `mobile/rt-shell`.
final class CredentialStore {
    private let service = "com.rebornace.baize.mobilert"

    private enum Key: String {
        case apiBase = "api_base"
        case apiKey = "api_key"
        case model = "model"
        case configURL = "config_url"
    }

    func save(apiBase: String, apiKey: String, model: String, configURL: String) {
        set(Key.apiBase, apiBase)
        set(Key.apiKey, apiKey)
        set(Key.model, model)
        set(Key.configURL, configURL)
    }

    func apiBase() -> String { get(Key.apiBase) }
    func apiKey() -> String { get(Key.apiKey) }
    func model() -> String { get(Key.model) }
    func configURL() -> String { get(Key.configURL) }

    func clearKey() {
        delete(Key.apiKey)
    }

    private func set(_ key: Key, _ value: String) {
        let data = Data(value.utf8)
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key.rawValue,
        ]
        SecItemDelete(query as CFDictionary)
        var add = query
        add[kSecValueData as String] = data
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        SecItemAdd(add as CFDictionary, nil)
    }

    private func get(_ key: Key) -> String {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key.rawValue,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var out: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &out)
        guard status == errSecSuccess, let data = out as? Data else { return "" }
        return String(data: data, encoding: .utf8) ?? ""
    }

    private func delete(_ key: Key) {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key.rawValue,
        ]
        SecItemDelete(query as CFDictionary)
    }
}
