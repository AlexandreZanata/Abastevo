package com.anpfuel.data.notification

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build
import androidx.core.app.NotificationManagerCompat
import com.anpfuel.data.R
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * UC-014 — Notification channel for local fuel price drop alerts.
 */
@Singleton
class PriceDropNotificationChannel @Inject constructor(
    @ApplicationContext private val context: Context,
) {

    init {
        ensureChannelCreated()
    }

    fun ensureChannelCreated() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
            return
        }
        val channel = NotificationChannel(
            CHANNEL_ID,
            context.getString(R.string.notification_price_drop_channel_name),
            NotificationManager.IMPORTANCE_DEFAULT,
        ).apply {
            description = context.getString(R.string.notification_price_drop_channel_description)
        }
        notificationManager.createNotificationChannel(channel)
    }

    private val notificationManager: NotificationManager
        get() = context.getSystemService(NotificationManager::class.java)

    companion object {
        const val CHANNEL_ID = "price_drop_alerts"
    }
}
