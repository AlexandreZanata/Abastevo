package com.anpfuel.data.notification

import android.Manifest
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import com.anpfuel.data.R
import com.anpfuel.domain.model.PriceDropAlertNotification
import com.anpfuel.domain.repository.PriceDropNotificationRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.DomainId
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.Locale
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

@Singleton
class PriceDropNotificationRepositoryImpl @Inject constructor(
    @ApplicationContext private val context: Context,
    private val channel: PriceDropNotificationChannel,
    private val userPreferencesRepository: UserPreferencesRepository,
) : PriceDropNotificationRepository {

    init {
        channel.ensureChannelCreated()
    }

    override fun hasPostNotificationsPermission(): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
            return true
        }
        return ContextCompat.checkSelfPermission(
            context,
            Manifest.permission.POST_NOTIFICATIONS,
        ) == PackageManager.PERMISSION_GRANTED
    }

    override suspend fun showPriceDropAlert(notification: PriceDropAlertNotification) {
        if (!hasPostNotificationsPermission()) {
            return
        }

        withContext(Dispatchers.Main) {
            val localizedContext = localizedContext()
            val contentTitle = localizedContext.getString(R.string.notification_price_drop_title)
            val contentText = localizedContext.getString(
                R.string.notification_price_drop_body,
                notification.vehicleDisplayName,
                notification.currentPriceFormatted,
            )
            val builder = NotificationCompat.Builder(localizedContext, PriceDropNotificationChannel.CHANNEL_ID)
                .setSmallIcon(android.R.drawable.ic_dialog_info)
                .setContentTitle(contentTitle)
                .setContentText(contentText)
                .setStyle(NotificationCompat.BigTextStyle().bigText(contentText))
                .setAutoCancel(true)
                .setContentIntent(createContentIntent())

            NotificationManagerCompat.from(localizedContext).notify(
                notificationId(notification.vehicleId),
                builder.build(),
            )
        }
    }

    override suspend fun cancelForVehicle(vehicleId: DomainId) {
        withContext(Dispatchers.Main) {
            NotificationManagerCompat.from(context).cancel(notificationId(vehicleId))
        }
    }

    private suspend fun localizedContext(): Context {
        val localeTag = userPreferencesRepository.getPreferences().localeTag
        val locale = Locale.forLanguageTag(localeTag)
        val configuration = context.resources.configuration
        configuration.setLocale(locale)
        return context.createConfigurationContext(configuration)
    }

    private fun createContentIntent(): PendingIntent {
        val launchIntent = context.packageManager.getLaunchIntentForPackage(context.packageName)
            ?: Intent()
        return PendingIntent.getActivity(
            context,
            0,
            launchIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }

    private fun notificationId(vehicleId: DomainId): Int = vehicleId.value.hashCode()
}
