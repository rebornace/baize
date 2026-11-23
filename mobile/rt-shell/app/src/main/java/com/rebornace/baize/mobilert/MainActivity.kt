package com.rebornace.baize.mobilert

import android.net.Uri
import android.os.Bundle
import android.provider.OpenableColumns
import android.view.View
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.Spinner
import android.widget.TextView
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import mobileagent.Agent
import mobileagent.Mobileagent
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.util.Locale
import java.util.UUID
import kotlin.concurrent.thread

/**
 * MOBILE-RT shell: vault + remote config + multi-session Chat.
 * Streaming uses StartStream/PollStream; each session keeps its own history
 * and pending approval. Does not talk to baize serve for the Run loop.
 */
class MainActivity : AppCompatActivity() {
    private var pendingCallId: String? = null
    private lateinit var vault: CredentialStore

    private val agents: MutableList<Agent> get() = RtSessions.agents
    private val sessionNames: MutableList<String> get() = RtSessions.names
    private val sessionTexts: MutableList<String> get() = RtSessions.texts
    private var activeIndex: Int
        get() = RtSessions.activeIndex
        set(value) {
            RtSessions.activeIndex = value
        }
    private var streaming: Boolean
        get() = RtSessions.streaming
        set(value) {
            RtSessions.streaming = value
        }
    private var saveAll: (() -> Unit)? = null
    private var resumePoll: (() -> Unit)? = null
    private var addAttachment: ((Uri) -> Unit)? = null
    private val pendingAttachments = mutableListOf<PendingAttachment>()
    private var suppressSpinner = false

    private data class PendingAttachment(
        val id: String,
        val name: String,
        val mime: String,
        val size: Long,
        val path: String,
    )

    private val pickAttachment =
        registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
            uri?.let { addAttachment?.invoke(it) }
        }

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
        val retry = findViewById<Button>(R.id.retry)
        val attach = findViewById<Button>(R.id.attach)
        val attachmentList = findViewById<TextView>(R.id.attachmentList)
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

        if (!RtSessions.initialized) {
            var savedActive = 0
            var restoredAny = false
            var savedStreaming = false
            var savedStreamingIndex = 0
            val savedRaw = vault.sessions()
            if (savedRaw.isNotBlank()) {
                try {
                    val root = JSONObject(savedRaw)
                    savedActive = root.optInt("active", 0)
                    savedStreaming = root.optBoolean("streaming", false)
                    savedStreamingIndex = root.optInt("streaming_index", savedActive)
                    val arr = root.optJSONArray("sessions")
                    if (arr != null) {
                        for (i in 0 until arr.length()) {
                            val o = arr.optJSONObject(i) ?: continue
                            val agent = Mobileagent.newAgent()
                            try {
                                if (vault.apiBase().isNotEmpty() && vault.model().isNotEmpty()) {
                                    agent.configure(vault.apiBase(), vault.apiKey(), vault.model())
                                }
                            } catch (_: Exception) {
                            }
                            val snap = o.optString("snapshot", "")
                            if (snap.isNotBlank()) {
                                val res = try {
                                    JSONObject(agent.importSession(snap))
                                } catch (_: Exception) {
                                    null
                                }
                                if (res?.optString("status") != "done") continue
                            }
                            var text = o.optString("text", "")
                            if (text.startsWith("HITL:")) {
                                text = "Previous approval expired after restart."
                            }
                            agents.add(agent)
                            sessionNames.add(o.optString("name", "Session ${agents.size}"))
                            sessionTexts.add(text)
                            restoredAny = true
                        }
                    }
                } catch (_: Exception) {
                }
            }
            if (!restoredAny) {
                val first = Mobileagent.newAgent()
                try {
                    if (vault.apiBase().isNotEmpty() && vault.model().isNotEmpty()) {
                        first.configure(vault.apiBase(), vault.apiKey(), vault.model())
                    }
                } catch (_: Exception) {
                }
                agents.add(first)
                sessionNames.add("Session 1")
                sessionTexts.add("")
                savedActive = 0
            }
            activeIndex = savedActive.coerceIn(0, agents.lastIndex)
            if (savedStreaming && agents.isNotEmpty()) {
                val idx = savedStreamingIndex.coerceIn(0, agents.lastIndex)
                val kept = sessionTexts[idx]
                sessionTexts[idx] = if (kept.isBlank()) {
                    "[stream interrupted when the app closed. Tap Retry or send again.]"
                } else {
                    "$kept\n\n[stream interrupted when the app closed. Partial reply kept; tap Retry or send again.]"
                }
                retry.isEnabled = true
            }
            RtSessions.initialized = true
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

        fun saveSessions() {
            if (agents.isEmpty()) return
            try {
                val arr = JSONArray()
                for (i in agents.indices) {
                    val o = JSONObject()
                    o.put("name", sessionNames[i])
                    o.put("text", sessionTexts[i])
                    o.put("snapshot", agents[i].exportSession())
                    arr.put(o)
                }
                val root = JSONObject()
                root.put("active", activeIndex)
                root.put("streaming", streaming)
                root.put("streaming_index", activeIndex)
                root.put("sessions", arr)
                vault.saveSessions(root.toString())
            } catch (_: Exception) {
            }
        }

        fun formatAttachmentSize(bytes: Long): String {
            if (bytes < 1024) return "$bytes B"
            if (bytes < 1024 * 1024) return String.format(Locale.US, "%.1f KB", bytes / 1024.0)
            return String.format(Locale.US, "%.1f MB", bytes / (1024.0 * 1024.0))
        }

        fun pendingAttachmentsJSON(): String {
            val arr = JSONArray()
            for (a in pendingAttachments) {
                val o = JSONObject()
                o.put("id", a.id)
                o.put("name", a.name)
                o.put("mime", a.mime)
                o.put("size", a.size)
                o.put("local_ref", a.path)
                arr.put(o)
            }
            return arr.toString()
        }

        fun renderAttachmentsNow() {
            val lines = mutableListOf<String>()
            if (pendingAttachments.isNotEmpty()) {
                for (a in pendingAttachments) {
                    lines.add("📎 ${a.name} · ${formatAttachmentSize(a.size)} · ${a.mime}")
                }
                lines.add("(pending; metadata only, file stays on device)")
            } else {
                val raw = try {
                    activeAgent().sessionAttachments()
                } catch (_: Exception) {
                    ""
                }
                val groups = try {
                    JSONObject(raw).optJSONArray("attachments")
                } catch (_: Exception) {
                    null
                }
                if (groups != null) {
                    val start = maxOf(0, groups.length() - 3)
                    for (i in start until groups.length()) {
                        val group = groups.optJSONObject(i) ?: continue
                        val items = group.optJSONArray("attachments") ?: continue
                        for (j in 0 until items.length()) {
                            val a = items.optJSONObject(j) ?: continue
                            val name = a.optString("name")
                            val path = a.optString("local_ref")
                            val missing = path.isBlank() || !File(path).exists()
                            val mark = if (missing) "⚠" else "📎"
                            val suffix = if (missing) " · file missing" else ""
                            lines.add("$mark $name · ${formatAttachmentSize(a.optLong("size"))} · ${a.optString("mime")}$suffix")
                        }
                    }
                }
            }
            attachmentList.text = lines.joinToString("\n")
            attachmentList.visibility = if (lines.isEmpty()) View.GONE else View.VISIBLE
        }

        addAttachment = { uri ->
            thread {
                try {
                    if (pendingAttachments.size >= 8) {
                        throw IllegalStateException("at most 8 attachments per message")
                    }
                    var name = "attachment"
                    var declaredSize = 0L
                    contentResolver.query(
                        uri,
                        arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE),
                        null,
                        null,
                        null,
                    )?.use { c ->
                        if (c.moveToFirst()) {
                            val ni = c.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                            if (ni >= 0) name = c.getString(ni) ?: name
                            val si = c.getColumnIndex(OpenableColumns.SIZE)
                            if (si >= 0) declaredSize = c.getLong(si)
                        }
                    }
                    val limit = 10L * 1024 * 1024
                    if (declaredSize > limit) {
                        throw IllegalStateException("attachment exceeds 10 MB")
                    }
                    val dir = File(filesDir, "attachments")
                    if (!dir.exists() && !dir.mkdirs()) {
                        throw IllegalStateException("cannot create attachments dir")
                    }
                    val id = UUID.randomUUID().toString()
                    val ext = name.substringAfterLast('.', "")
                    val target = File(dir, if (ext.isBlank()) id else "$id.$ext")
                    contentResolver.openInputStream(uri)?.use { input ->
                        target.outputStream().use { output ->
                            val buf = ByteArray(64 * 1024)
                            var total = 0L
                            while (true) {
                                val n = input.read(buf)
                                if (n < 0) break
                                total += n
                                if (total > limit) {
                                    target.delete()
                                    throw IllegalStateException("attachment exceeds 10 MB")
                                }
                                output.write(buf, 0, n)
                            }
                        }
                    } ?: throw IllegalStateException("cannot read selected file")
                    if (target.length() > limit) {
                        target.delete()
                        throw IllegalStateException("attachment exceeds 10 MB")
                    }
                    val mime = contentResolver.getType(uri) ?: "application/octet-stream"
                    runOnUiThread {
                        pendingAttachments.add(
                            PendingAttachment(id, name, mime, target.length(), target.absolutePath),
                        )
                        renderAttachmentsNow()
                    }
                } catch (e: Exception) {
                    runOnUiThread {
                        attachmentList.text = "Attach failed: ${e.message}"
                        attachmentList.visibility = View.VISIBLE
                    }
                }
            }
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
            saveSessions()
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
            saveSessions()
        }

        fun ensureConfigured() {
            activeAgent().configure(
                apiBase.text.toString().trim(),
                apiKey.text.toString(),
                model.text.toString().trim(),
            )
        }

        fun applyStreamFinal(o: JSONObject, raw: String) {
            streaming = false
            stop.isEnabled = false
            send.isEnabled = true
            retry.isEnabled = false
            attach.isEnabled = true
            val status = o.optString("status")
            val text = o.optString("text")
            when (status) {
                "cancelled" -> {
                    pendingCallId = null
                    hitlRow.visibility = View.GONE
                    reply.text = if (text.isNotEmpty()) {
                        "$text\n\n[cancelled; Retry restarts this turn]"
                    } else {
                        "Cancelled; Retry restarts this turn"
                    }
                    retry.isEnabled = true
                    sessionTexts[activeIndex] = reply.text.toString()
                    saveSessions()
                }
                "error" -> {
                    pendingCallId = null
                    hitlRow.visibility = View.GONE
                    val detail = try {
                        JSONObject(o.optString("result")).optString("message")
                    } catch (_: Exception) {
                        ""
                    }.ifBlank { o.optString("message") }
                    reply.text = if (text.isNotEmpty()) {
                        "$text\n\n[stream interrupted: $detail. Retry or edit and send]"
                    } else {
                        "Stream interrupted: $detail. Retry or send again."
                    }
                    retry.isEnabled = true
                    sessionTexts[activeIndex] = reply.text.toString()
                    saveSessions()
                }
                else -> {
                    val result = o.optString("result")
                    applyResult(if (result.isNotEmpty()) result else raw)
                }
            }
            renderAttachmentsNow()
        }

        fun pollStream(agent: Agent) {
            val token = ++RtSessions.pollToken
            thread {
                while (true) {
                    if (token != RtSessions.pollToken) return@thread
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
                    if (status == "streaming" || status == "cancelling") {
                        runOnUiThread {
                            if (token != RtSessions.pollToken) return@runOnUiThread
                            if (agent !== activeAgent()) return@runOnUiThread
                            val text = o.optString("text")
                            if (text.isNotEmpty()) {
                                reply.text = text
                                sessionTexts[activeIndex] = text
                            }
                        }
                        Thread.sleep(120)
                        continue
                    }
                    runOnUiThread {
                        if (token != RtSessions.pollToken) return@runOnUiThread
                        if (agent !== activeAgent()) return@runOnUiThread
                        applyStreamFinal(o, raw)
                    }
                    return@thread
                }
            }
        }

        fun resumeActiveStream() {
            val agent = activeAgent()
            val raw = try {
                agent.pollStream()
            } catch (_: Exception) {
                """{"status":"idle"}"""
            }
            val o = try {
                JSONObject(raw)
            } catch (_: Exception) {
                JSONObject()
            }
            when (o.optString("status")) {
                "streaming", "cancelling" -> {
                    streaming = true
                    send.isEnabled = false
                    stop.isEnabled = true
                    retry.isEnabled = false
                    attach.isEnabled = false
                    pollStream(agent)
                }
                "idle" -> {
                    streaming = false
                    send.isEnabled = true
                    stop.isEnabled = false
                    attach.isEnabled = true
                }
                else -> applyStreamFinal(o, raw)
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
            pendingAttachments.clear()
            reply.text = sessionTexts[index].ifEmpty { sessionNames[index] }
            showPending()
            refreshReadonlyStatus()
            renderAttachmentsNow()
            saveSessions()
        }

        reply.text = sessionTexts[activeIndex].ifEmpty { sessionNames[activeIndex] }
        refreshSpinner(activeIndex)
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
            pendingAttachments.clear()
            renderAttachmentsNow()
            saveSessions()
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
            pendingAttachments.clear()
            renderAttachmentsNow()
            saveSessions()
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
                saveSessions()
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
                    saveSessions()
                    vault.save(
                        apiBase.text.toString().trim(),
                        apiKey.text.toString(),
                        model.text.toString().trim(),
                        url,
                    )
                }
            }
        }

        attach.setOnClickListener {
            if (streaming) return@setOnClickListener
            pickAttachment.launch(arrayOf("*/*"))
        }

        send.setOnClickListener {
            if (streaming) return@setOnClickListener
            streaming = true
            send.isEnabled = false
            stop.isEnabled = true
            retry.isEnabled = false
            attach.isEnabled = false
            hitlRow.visibility = View.GONE
            reply.text = "…"
            val msg = userInput.text.toString()
            val atts = pendingAttachmentsJSON()
            val current = activeAgent()
            thread {
                val raw = try {
                    ensureConfigured()
                    current.startStreamWithAttachments(msg, atts)
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
                        pendingAttachments.clear()
                        renderAttachmentsNow()
                        pollStream(current)
                    } else {
                        streaming = false
                        stop.isEnabled = false
                        attach.isEnabled = true
                        applyResult(raw)
                    }
                }
            }
        }

        retry.setOnClickListener {
            if (streaming) return@setOnClickListener
            streaming = true
            send.isEnabled = false
            stop.isEnabled = true
            retry.isEnabled = false
            attach.isEnabled = false
            hitlRow.visibility = View.GONE
            reply.text = "…"
            val current = activeAgent()
            thread {
                val raw = try {
                    ensureConfigured()
                    current.retryStream()
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
                        retry.isEnabled = true
                        attach.isEnabled = true
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
        attach.isEnabled = !streaming
        renderAttachmentsNow()

        val savedConfigUrl = vault.configURL()
        if (savedInstanceState == null && savedConfigUrl.isNotEmpty()) {
            restoreRemoteConfig(savedConfigUrl)
        }

        saveAll = {
            saveActiveText()
            saveSessions()
        }
        resumePoll = {
            if (streaming) resumeActiveStream()
        }
    }

    override fun onPause() {
        RtSessions.pollToken++
        saveAll?.invoke()
        super.onPause()
    }

    override fun onResume() {
        super.onResume()
        resumePoll?.invoke()
    }
}
