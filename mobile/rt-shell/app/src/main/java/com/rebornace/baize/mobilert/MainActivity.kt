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
 * MOBILE-RT M1 shell: Send / Approve / Deny on-device.
 * Does not talk to baize serve for the Run loop.
 */
class MainActivity : AppCompatActivity() {
    private var pendingCallId: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        val apiBase = findViewById<EditText>(R.id.apiBase)
        val apiKey = findViewById<EditText>(R.id.apiKey)
        val model = findViewById<EditText>(R.id.model)
        val userInput = findViewById<EditText>(R.id.userInput)
        val reply = findViewById<TextView>(R.id.reply)
        val send = findViewById<Button>(R.id.send)
        val hitlRow = findViewById<LinearLayout>(R.id.hitlRow)
        val approve = findViewById<Button>(R.id.approve)
        val deny = findViewById<Button>(R.id.deny)

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
                        reply.text = o.optString("text")
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
