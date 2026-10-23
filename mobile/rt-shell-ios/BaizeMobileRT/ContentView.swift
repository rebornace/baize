import SwiftUI

/// MOBILE-RT iOS shell: Keychain vault + remote config + Send/Approve/Deny.
/// Does not talk to baize serve for the Run loop.
struct ContentView: View {
    @State private var apiBase = "http://127.0.0.1:18080/v1"
    @State private var apiKey = ""
    @State private var model = "mock"
    @State private var configURL = ""
    @State private var userInput = "你好"
    @State private var reply = ""
    @State private var pendingCallId: String?
    @State private var busy = false

    private let vault = CredentialStore()

    var body: some View {
        NavigationStack {
            Form {
                Section("LLM (device → API; no baize serve)") {
                    TextField("API base", text: $apiBase)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    SecureField("API key", text: $apiKey)
                    TextField("Model", text: $model)
                        .textInputAutocapitalization(.never)
                    TextField("Remote config URL", text: $configURL)
                        .textInputAutocapitalization(.never)
                    Button("Save vault (Keychain)") { saveVault() }
                    Button("Fetch remote config") { fetchConfig() }
                        .disabled(busy || configURL.trimmingCharacters(in: .whitespaces).isEmpty)
                }
                Section("Chat") {
                    TextField("Message", text: $userInput, axis: .vertical)
                        .lineLimit(2...6)
                    Button("Send") { send() }
                        .disabled(busy || pendingCallId != nil)
                    if pendingCallId != nil {
                        HStack {
                            Button("Approve") { approve() }.disabled(busy)
                            Button("Deny", role: .destructive) { deny() }.disabled(busy)
                        }
                    }
                }
                Section("Reply") {
                    Text(reply.isEmpty ? "—" : reply)
                        .font(.system(.body, design: .monospaced))
                        .textSelection(.enabled)
                }
            }
            .navigationTitle("Baize MOBILE-RT")
            .onAppear(perform: restoreVault)
        }
    }

    private func restoreVault() {
        if !vault.apiBase().isEmpty { apiBase = vault.apiBase() }
        if !vault.apiKey().isEmpty { apiKey = vault.apiKey() }
        if !vault.model().isEmpty { model = vault.model() }
        if !vault.configURL().isEmpty { configURL = vault.configURL() }
        if !vault.apiBase().isEmpty && !vault.model().isEmpty {
            var err: NSError?
            _ = MobileagentConfigure(vault.apiBase(), vault.apiKey(), vault.model(), &err)
        }
    }

    private func ensureConfigured() throws {
        var err: NSError?
        guard MobileagentConfigure(apiBase.trimmingCharacters(in: .whitespaces), apiKey, model.trimmingCharacters(in: .whitespaces), &err) else {
            throw err ?? NSError(domain: "mobileagent", code: 1, userInfo: [NSLocalizedDescriptionKey: "configure failed"])
        }
    }

    private func saveVault() {
        vault.save(
            apiBase: apiBase.trimmingCharacters(in: .whitespaces),
            apiKey: apiKey,
            model: model.trimmingCharacters(in: .whitespaces),
            configURL: configURL.trimmingCharacters(in: .whitespaces)
        )
        do {
            try ensureConfigured()
            reply = "Vault saved (Keychain). hasKey=\(MobileagentHasAPIKey())"
        } catch {
            reply = "vault/configure error: \(error.localizedDescription)"
        }
    }

    private func fetchConfig() {
        let url = configURL.trimmingCharacters(in: .whitespaces)
        busy = true
        DispatchQueue.global(qos: .userInitiated).async {
            let raw = MobileagentFetchRemoteConfig(url)
            DispatchQueue.main.async {
                busy = false
                applyResult(raw)
                vault.save(
                    apiBase: apiBase.trimmingCharacters(in: .whitespaces),
                    apiKey: apiKey,
                    model: model.trimmingCharacters(in: .whitespaces),
                    configURL: url
                )
            }
        }
    }

    private func send() {
        busy = true
        reply = "…"
        let msg = userInput
        DispatchQueue.global(qos: .userInitiated).async {
            let raw: String
            do {
                try ensureConfigured()
                raw = MobileagentSend(msg)
            } catch {
                raw = #"{"status":"error","message":"\#(error.localizedDescription)"}"#
            }
            DispatchQueue.main.async {
                busy = false
                applyResult(raw)
            }
        }
    }

    private func approve() {
        guard let id = pendingCallId else { return }
        busy = true
        DispatchQueue.global(qos: .userInitiated).async {
            let raw = MobileagentApprove(id)
            DispatchQueue.main.async {
                busy = false
                applyResult(raw)
            }
        }
    }

    private func deny() {
        guard let id = pendingCallId else { return }
        busy = true
        DispatchQueue.global(qos: .userInitiated).async {
            let raw = MobileagentDeny(id)
            DispatchQueue.main.async {
                busy = false
                applyResult(raw)
            }
        }
    }

    private func applyResult(_ raw: String) {
        guard let data = raw.data(using: .utf8),
              let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let status = obj["status"] as? String
        else {
            pendingCallId = nil
            reply = raw
            return
        }
        switch status {
        case "needs_approval":
            pendingCallId = obj["call_id"] as? String
            let summary = obj["summary"] as? String ?? ""
            reply = "HITL: \(summary)\n\(raw)"
        case "done":
            pendingCallId = nil
            if let text = obj["text"] as? String, !text.isEmpty {
                reply = text
            } else {
                reply = raw
            }
            if let b = obj["api_base"] as? String { apiBase = b }
            if let m = obj["model"] as? String { model = m }
        default:
            pendingCallId = nil
            reply = raw
        }
    }
}
