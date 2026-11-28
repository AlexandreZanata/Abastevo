package com.anpfuel.app.notification

import android.content.Intent
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/** A tap on an Abastevo Android notification asks the app to open Notifications once. */
object NotificationOpenRequest {
    const val EXTRA_OPEN = "com.anpfuel.app.OPEN"
    const val OPEN_NOTIFICATIONS = "notifications"

    private val pending = MutableStateFlow(false)
    val requested: StateFlow<Boolean> = pending.asStateFlow()

    fun from(intent: Intent?) {
        if (intent?.getStringExtra(EXTRA_OPEN) == OPEN_NOTIFICATIONS) {
            pending.value = true
            intent.removeExtra(EXTRA_OPEN)
        }
    }

    fun consume() {
        pending.value = false
    }
}
