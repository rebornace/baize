package com.rebornace.baize.mobilert

import mobileagent.Agent

/**
 * In-process session runtime shared by Activity instances.
 *
 * Rotation or background-recreate keeps the same Go agents, so an in-flight
 * stream can be polled again. A cold process start rebuilds from the vault.
 */
object RtSessions {
    val agents = mutableListOf<Agent>()
    val names = mutableListOf<String>()
    val texts = mutableListOf<String>()
    var activeIndex = 0
    var streaming = false
    var pollToken = 0
    var initialized = false
}
