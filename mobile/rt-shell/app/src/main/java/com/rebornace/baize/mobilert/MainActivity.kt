package com.rebornace.baize.mobilert

import android.os.Bundle
import android.view.View
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.Spinner
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import mobileagent.Agent
import mobileagent.Mobileagent
import org.json.JSONObject
import kotlin.concurrent.thread

/**
 * MOBILE-RT shell: vault + remote config + multi-session Chat.
 * Streaming uses StartStream/PollStream; each session keeps its own history
 * and pending approval. Does not talk to baize serve for the Run loop.
 */
class MainActivity : AppCompatActivity() {
    private var pendingCallId: String? = null
    private lateinit var vault: CredentialStore

    private val agents = mutableListOf<Agent>()
    private val sessionNames = mutableListOf<String>()
    private val sessionTexts = mutableListOf<String>()
    private var activeIndex = 0
    private var streaming = false
    private var suppressSpinner = false

    private fun activeAgent(): Agent = agents[activeIndex]

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
        val stop = findViewById<Button>(R.id.stop)
        val saveVault = findViewById<Button>(R.id.saveVault)
        val fetchConfig = findViewById<Button>(R.id.fetchConfig)
        val hitlRow = findViewById<LinearLayout>(R.id.hitlRow)
        val approve = findViewById<Button>(R.id.approve)
        val deny = findViewById<Button>(R.id.deny)
        val readonlyStatus = findViewById<TextView>(R.id.readonlyStatus)
        val sessionSpinner = findViewById<Spinner>(R.id.sessionSpinner)
        val newSession = findViewById<Button>(R.id.newSession)
        val clearSession = findViewById<Button>(R.id.clearSession)

        // Restore non-secret fields before creating the first session.
        if (vault.apiBase().isNotEmpty()) apiBase.setText(vault.apiBase())
        if (vault.apiKey().isNotEmpty()) apiKey.setText(vault.apiKey())
        if (vault.model().isNotEmpty()) model.setText(vault.model())
        if (vault.configURL().isNotEmpty()) configUrl.setText(vault.configURL())

        val first = Mobileagent.newAgent()
        agents.add(first)
        sessionNames.add("Session 1")
        sessionTexts.add("")
        try {
            if (vault.apiBase().isNotEmpty() && vault.model().isNotEmpty()) {
                first.configure(vault.apiBase(), vault.apiKey(), vault.model())
            }
        } catch (_: Exception) {
        }

        fun refreshReadonlyStatus() {
            val names = try {
                activeAgent().remoteToolNames()
            } catch (_: Exception) {
                ""
            }
            val writes = try {
                activeAgent().remoteWriteToolCount()
            } catch (_: Exception) {
                0
            }
            readonlyStatus.text = when {
                names.isBlank() -> "Tools: none"
                writes > 0 -> "Tools: $names ($writes write)"
                else -> "Tools: $names"
            }
        }

        fun refreshSpinner(select: Int) {
            suppressSpinner = true
            val adapter = ArrayAdapter(
                this,
                android.R.layout.simple_spinner_dropdown_item,
                sessionNames.toList(),
            )
            sessionSpinner.adapter = adapter
            sessionSpinner.setSelection(select, false)
            suppressSpinner = false
        }

        fun saveActiveText() {
            if (agents.isNotEmpty()) sessionTexts[activeIndex] = reply.text.toString()
        }

        fun showPending() {
            val raw = try {
                activeAgent().pendingApproval()
            } catch (_: Exception) {
                """{"status":"none"}"""
            }
            val o = try {
                JSONObject(raw)
            } catch (_: Exception) {
                JSONObject()
            }
            if (o.optString("status") == "needs_approval") {
                pendingCallId = o.optString("call_id")
                hitlRow.visibility = View.VISIBLE
                send.isEnabled = false
                reply.text = "HITL: ${o.optString("summary")}\n$raw"
                sessionTexts[activeIndex] = reply.text.toString()
            } else {
                pendingCallId = null
                hitlRow.visibility = View.GONE
                send.isEnabled = !streaming
            }
        }

        fun applyResult(raw: String) {
            refreshReadonlyStatus()
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
                        send.isEnabled = !streaming
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
                        send.isEnabled = !streaming
                        reply.text = raw
                    }
                }
            } catch (_: Exception) {
                pendingCallId = null
                hitlRow.visibility = View.GONE
                send.isEnabled = !streaming
                reply.text = raw
            }
            sessionTexts[activeIndex] = reply.text.toString()
        }

        fun ensureConfigured() {
            activeAgent().configure(
                apiBase.text.toString().trim(),
                apiKey.text.toString(),
                model.text.toString().trim(),
            )
        }

        fun pollStream(agent: Agent) {
            thread {
                while (true) {
                    val raw = try {
                        agent.pollStream()
                    } catch (e: Exception) {
                        """{"status":"error","message":"${e.message}"}"""
                    }
                    val o = try {
                        JSONObject(raw)
                    } catch (_: Exception) {
                        JSONObject()
                    }
                    val status = o.optString("status")
                    runOnUiThread {
                        if (agent !== activeAgent()) return@runOnUiThread
                        if (status == "streaming" || status == "cancelling") {
                            val text = o.optString("text")
                            if (text.isNotEmpty()) {
                                reply.text = text
                                sessionTexts[activeIndex] = text
                            }
                            return@runOnUiThread
                        }
                        streaming = false
                        stop.isEnabled = false
                        if (status == "cancelled") {
                            pendingCallId = null
                            hitlRow.visibility = View.GONE
                            send.isEnabled = true
                            val text = o.optString("text")
                            reply.text = if (text.isNotEmpty()) "$text\n[cancelled]" else "Cancelled"
                            sessionTexts[activeIndex] = reply.text.toString()
                        } else {
                            val result = o.optString("result")
                            applyResult(if (result.isNotEmpty()) result else raw)
                        }
                    }
                    if (status != "streaming" && status != "cancelling") break
                    Thread.sleep(120)
                }
            }
        }

        fun switchTo(index: Int) {
            if (index == activeIndex || index !in agents.indices) return
            if (streaming) {
                reply.text = "Cancel the current stream before switching sessions."
                refreshSpinner(activeIndex)
                return
            }
            saveActiveText()
            activeIndex = index
            reply.text = sessionTexts[index].ifEmpty { sessionNames[index] }
            showPending()
            refreshReadonlyStatus()
        }

        refreshSpinner(0)
        sessionSpinner.onItemSelectedListener = object : android.widget.AdapterView.OnItemSelectedListener {
            override fun onItemSelected(
                parent: android.widget.AdapterView<*>?,
                view: View?,
                position: Int,
                id: Long,
            ) {
                if (suppressSpinner) return
                switchTo(position)
            }

            override fun onNothingSelected(parent: android.widget.AdapterView<*>?) {}
        }

        newSession.setOnClickListener {
            if (streaming) {
                reply.text = "Cancel the current stream before starting a new session."
                return@setOnClickListener
            }
            saveActiveText()
            val next = try {
                activeAgent().newSession()
            } catch (e: Exception) {
                reply.text = "new session error: ${e.message}"
                return@setOnClickListener
            }
            agents.add(next)
            sessionNames.add("Session ${agents.size}")
            sessionTexts.add("")
            activeIndex = agents.lastIndex
            refreshSpinner(activeIndex)
            reply.text = "New session"
            showPending()
            refreshReadonlyStatus()
        }

        clearSession.setOnClickListener {
            if (streaming) {
                reply.text = "Cancel the current stream before clearing the session."
                return@setOnClickListener
            }
            activeAgent().reset()
            sessionTexts[activeIndex] = ""
            pendingCallId = null
            hitlRow.visibility = View.GONE
            send.isEnabled = true
            reply.text = "Session cleared"
            refreshReadonlyStatus()
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
                sessionTexts[activeIndex] = reply.text.toString()
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
                    ensureConfigured()
                    activeAgent().fetchRemoteConfig(url)
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

        fun restoreRemoteConfig(url: String) {
            thread {
                val raw = try {
                    activeAgent().fetchRemoteConfig(url)
                } catch (_: Exception) {
                    ""
                }
                val restored = raw.isNotEmpty() && try {
                    JSONObject(raw).optString("status") == "done"
                } catch (_: Exception) {
                    false
                }
                runOnUiThread {
                    if (!restored) {
                        refreshReadonlyStatus()
                        return@runOnUiThread
                    }
                    applyResult(raw)
                    reply.text = "Remote config restored. Tools: " +
                        (activeAgent().remoteToolNames().ifBlank { "none" })
                    sessionTexts[activeIndex] = reply.text.toString()
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
            if (streaming) return@setOnClickListener
            streaming = true
            send.isEnabled = false
            stop.isEnabled = true
            hitlRow.visibility = View.GONE
            reply.text = "…"
            val msg = userInput.text.toString()
            val current = activeAgent()
            thread {
                val raw = try {
                    ensureConfigured()
                    current.startStream(msg)
                } catch (e: Exception) {
                    """{"status":"error","message":"${e.message}"}"""
                }
                runOnUiThread {
                    val status = try {
                        JSONObject(raw).optString("status")
                    } catch (_: Exception) {
                        ""
                    }
                    if (status == "streaming") {
                        pollStream(current)
                    } else {
                        streaming = false
                        stop.isEnabled = false
                        applyResult(raw)
                    }
                }
            }
        }

        stop.setOnClickListener {
            stop.isEnabled = false
            val current = activeAgent()
            thread {
                try {
                    current.cancelStream()
                } catch (_: Exception) {
                }
            }
        }

        approve.setOnClickListener {
            val id = pendingCallId ?: return@setOnClickListener
            approve.isEnabled = false
            deny.isEnabled = false
            val current = activeAgent()
            thread {
                val raw = try {
                    current.approve(id)
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
            val current = activeAgent()
            thread {
                val raw = try {
                    current.deny(id)
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

        refreshReadonlyStatus()
        showPending()

        val savedConfigUrl = vault.configURL()
        if (savedInstanceState == null && savedConfigUrl.isNotEmpty()) {
            restoreRemoteConfig(savedConfigUrl)
        }
    }
}
