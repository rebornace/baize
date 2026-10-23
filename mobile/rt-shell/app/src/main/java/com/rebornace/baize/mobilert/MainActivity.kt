package com.rebornace.baize.mobilert

import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import mobileagent.Mobileagent
import org.json.JSONObject
import kotlin.concurrent.thread

/**
 * MOBILE-RT M2 shell: vault + remote config + Send/Approve/Deny.
 * Does not talk to baize serve for the Run loop.
 */
class MainActivity : AppCompatActivity() {
    private var pendingCallId: String? = null
    private lateinit var vault: CredentialStore

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        vault = CredentialStore(this)

        val apiBase = findViewById<EditText>(R.id.apiBase)
        val apiKey = findViewById<EditText>(R.id.apiKey)
        val model = findViewById<EditText>(R.id.model)
        val configUrl = findViewById<EditText>(R.id.configUrl)
        val userInput = findViewById<EditText>(R.id.userInput)
        val reply = findViewById<TextView>(R.id.reply)
        val send = findViewById<Button>(R.id.send)
        val saveVault = findViewById<Button>(R.id.saveVault)
        val fetchConfig = findViewById<Button>(R.id.fetchConfig)
        val hitlRow = findViewById<LinearLayout>(R.id.hitlRow)
        val approve = findViewById<Button>(R.id.approve)
        val deny = findViewById<Button>(R.id.deny)

        // Restore from vault into fields + Go memory.
        if (vault.apiBase().isNotEmpty()) apiBase.setText(vault.apiBase())
        if (vault.apiKey().isNotEmpty()) apiKey.setText(vault.apiKey())
        if (vault.model().isNotEmpty()) model.setText(vault.model())
        if (vault.configURL().isNotEmpty()) configUrl.setText(vault.configURL())
        try {
            if (vault.apiBase().isNotEmpty() && vault.model().isNotEmpty()) {
                Mobileagent.configure(vault.apiBase(), vault.apiKey(), vault.model())
            }
        } catch (_: Exception) {
        }

        fun applyResult(raw: String) {
            try {
                val o = JSONObject(raw)
                when (o.optString("status")) {
                    "needs_approval" -> {
                        pendingCallId = o.optString("call_id")
                        hitlRow.visibility = View.VISIBLE
                        send.isEnabled = false
                        reply.text = "HITL: ${o.optString("summary")}\n$raw"
                    }
                    "done" -> {
                        pendingCallId = null
                        hitlRow.visibility = View.GONE
                        send.isEnabled = true
                        val text = o.optString("text")
                        reply.text = if (text.isNotEmpty()) text else raw
                        if (o.has("api_base")) {
                            apiBase.setText(o.optString("api_base"))
                            model.setText(o.optString("model"))
                        }
                    }
                    else -> {
                        pendingCallId = null
                        hitlRow.visibility = View.GONE
                        send.isEnabled = true
                        reply.text = raw
                    }
                }
            } catch (_: Exception) {
                pendingCallId = null
                hitlRow.visibility = View.GONE
                send.isEnabled = true
                reply.text = raw
            }
        }

        fun ensureConfigured() {
            Mobileagent.configure(
                apiBase.text.toString().trim(),
                apiKey.text.toString(),
                model.text.toString().trim(),
            )
        }

        saveVault.setOnClickListener {
            vault.save(
                apiBase.text.toString().trim(),
                apiKey.text.toString(),
                model.text.toString().trim(),
                configUrl.text.toString().trim(),
            )
            try {
                ensureConfigured()
                reply.text = "Vault saved (EncryptedSharedPreferences). hasKey=${Mobileagent.hasAPIKey()}"
            } catch (e: Exception) {
                reply.text = "vault/configure error: ${e.message}"
            }
        }

        fetchConfig.setOnClickListener {
            val url = configUrl.text.toString().trim()
            if (url.isEmpty()) {
                reply.text = "Set remote config URL first"
                return@setOnClickListener
            }
            fetchConfig.isEnabled = false
            thread {
                val raw = try {
                    Mobileagent.fetchRemoteConfig(url)
                } catch (e: Exception) {
                    """{"status":"error","message":"${e.message}"}"""
                }
                runOnUiThread {
                    fetchConfig.isEnabled = true
                    applyResult(raw)
                    // Persist non-secret fields; keep existing key unless remote sent one into Go.
                    vault.save(
                        apiBase.text.toString().trim(),
                        apiKey.text.toString(),
                        model.text.toString().trim(),
                        url,
                    )
                }
            }
        }

        send.setOnClickListener {
            send.isEnabled = false
            hitlRow.visibility = View.GONE
            reply.text = "…"
            val msg = userInput.text.toString()
            thread {
                val raw = try {
                    ensureConfigured()
                    Mobileagent.send(msg)
                } catch (e: Exception) {
                    """{"status":"error","message":"${e.message}"}"""
                }
                runOnUiThread { applyResult(raw) }
            }
        }

        approve.setOnClickListener {
            val id = pendingCallId ?: return@setOnClickListener
            approve.isEnabled = false
            deny.isEnabled = false
            thread {
                val raw = try {
                    Mobileagent.approve(id)
                } catch (e: Exception) {
                    """{"status":"error","message":"${e.message}"}"""
                }
                runOnUiThread {
                    approve.isEnabled = true
                    deny.isEnabled = true
                    applyResult(raw)
                }
            }
        }

        deny.setOnClickListener {
            val id = pendingCallId ?: return@setOnClickListener
            approve.isEnabled = false
            deny.isEnabled = false
            thread {
                val raw = try {
                    Mobileagent.deny(id)
                } catch (e: Exception) {
                    """{"status":"error","message":"${e.message}"}"""
                }
                runOnUiThread {
                    approve.isEnabled = true
                    deny.isEnabled = true
                    applyResult(raw)
                }
            }
        }
    }
}
