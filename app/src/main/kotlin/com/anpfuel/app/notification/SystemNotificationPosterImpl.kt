package com.anpfuel.app.notification

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import com.anpfuel.app.R
import com.anpfuel.app.ui.notifications.NotificationTextFormatter
import com.anpfuel.data.notification.PriceDropNotificationChannel
import com.anpfuel.domain.notification.AppNotification
import com.anpfuel.domain.notification.SystemNotificationPoster
import com.anpfuel.domain.repository.UserPreferencesRepository
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.Locale
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * B-BR-NOTIF-04/05 Android notifications. Account items use the "Account
 * updates" channel, local alerts the existing price-alert channel; every post
 * is skipped without the OS permission. A tap opens the Notifications screen.
 */
@Singleton
class SystemNotificationPosterImpl @Inject constructor(
    @ApplicationContext private val context: Context,
    private val preferences: UserPreferencesRepository,
    private val priceAlertChannel: PriceDropNotificationChannel,
) : SystemNotificationPoster {

    override fun canPost(): Boolean =
        Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED

    override suspend fun postAccount(notification: AppNotification) {
        val localized = localizedContext()
        ensureAccountChannel(localized)
        val text = NotificationTextFormatter.format(localized, notification.content, localized.resources.configuration.locales[0])
        post(localized, ACCOUNT_CHANNEL_ID, notification.id.hashCode(), text.title, text.body)
    }

    override suspend fun postAccountSummary(moreCount: Int) {
        val localized = localizedContext()
        ensureAccountChannel(localized)
        post(
            localized, ACCOUNT_CHANNEL_ID, SUMMARY_ID,
            localized.getString(R.string.notifications_screen_title),
            localized.resources.getQuantityString(R.plurals.notifications_more_updates, moreCount, moreCount),
        )
    }

    override suspend fun postLocal(notification: AppNotification) {
        val localized = localizedContext()
        priceAlertChannel.ensureChannelCreated()
        val text = NotificationTextFormatter.format(localized, notification.content, localized.resources.configuration.locales[0])
        post(localized, PriceDropNotificationChannel.CHANNEL_ID, notification.id.hashCode(), text.title, text.body)
    }

    private suspend fun post(localized: Context, channel: String, id: Int, title: String, body: String) {
        if (!canPost()) return
        withContext(Dispatchers.Main) {
            val builder = NotificationCompat.Builder(localized, channel)
                .setSmallIcon(android.R.drawable.ic_dialog_info)
                .setContentTitle(title)
                .setContentText(body)
                .setStyle(NotificationCompat.BigTextStyle().bigText(body))
                .setAutoCancel(true)
                .setContentIntent(openNotifications())
            try {
                NotificationManagerCompat.from(localized).notify(id, builder.build())
            } catch (_: SecurityException) {
                // The permission may be revoked between the check and the post.
            }
        }
    }

    private fun openNotifications(): PendingIntent {
        val intent = (context.packageManager.getLaunchIntentForPackage(context.packageName) ?: Intent())
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP)
            .putExtra(NotificationOpenRequest.EXTRA_OPEN, NotificationOpenRequest.OPEN_NOTIFICATIONS)
        return PendingIntent.getActivity(context, OPEN_REQUEST_CODE, intent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
    }

    private fun ensureAccountChannel(localized: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val channel = NotificationChannel(
            ACCOUNT_CHANNEL_ID,
            localized.getString(R.string.notifications_account_channel_name),
            NotificationManager.IMPORTANCE_DEFAULT,
        ).apply { description = localized.getString(R.string.notifications_account_channel_description) }
        context.getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
    }

    private suspend fun localizedContext(): Context {
        val tag = preferences.getPreferences().localeTag
        if (tag.isBlank()) return context
        val configuration = android.content.res.Configuration(context.resources.configuration)
        configuration.setLocale(Locale.forLanguageTag(tag))
        return context.createConfigurationContext(configuration)
    }

    companion object {
        const val ACCOUNT_CHANNEL_ID = "account_updates"
        private const val SUMMARY_ID = 0x4E4F5449
        private const val OPEN_REQUEST_CODE = 0x4E54
    }
}
