package com.rebornace.baize.mobilert

import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import mobileagent.Mobileagent
import kotlin.concurrent.thread

/**
 * MOBILE-RT M0 empty shell: calls the gomobile AAR on-device.
 * Does not talk to baize serve for the Run loop.
 */
class MainActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        val apiBase = findViewById<EditText>(R.id.apiBase)
        val apiKey = findViewById<EditText>(R.id.apiKey)
        val model = findViewById<EditText>(R.id.model)
        val userInput = findViewById<EditText>(R.id.userInput)
        val reply = findViewById<TextView>(R.id.reply)
        val send = findViewById<Button>(R.id.send)

        send.setOnClickListener {
            send.isEnabled = false
            reply.text = "…"
            val base = apiBase.text.toString().trim()
            val key = apiKey.text.toString()
            val mid = model.text.toString().trim()
            val msg = userInput.text.toString()
            thread {
                val text = try {
                    Mobileagent.configure(base, key, mid)
                    Mobileagent.chatOnce(msg)
                } catch (e: Exception) {
                    "error: ${e.message}"
                }
                runOnUiThread {
                    reply.text = text
                    send.isEnabled = true
                }
            }
        }
    }
}
