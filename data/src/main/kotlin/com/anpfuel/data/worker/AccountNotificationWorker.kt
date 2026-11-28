package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.anpfuel.application.usecase.notification.SyncAccountNotificationsUseCase
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CancellationException

/**
 * B-BR-NOTIF-04 — reads the account inbox in the background (no push SDK) and
 * posts Android notifications for new unread items. Signed out it does
 * nothing; failures wait for the next period instead of retrying hard.
 */
@HiltWorker
class AccountNotificationWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val sync: SyncAccountNotificationsUseCase,
) : CoroutineWorker(appContext, params) {

    override suspend fun doWork(): Result = try {
        sync()
        Result.success()
    } catch (cancelled: CancellationException) {
        throw cancelled
    } catch (_: Exception) {
        Result.success()
    }

    companion object {
        private const val WORK_NAME = "account-notifications"
        private const val PERIOD_HOURS = 3L

        fun schedule(context: Context) {
            val constraints = Constraints.Builder()
                .setRequiredNetworkType(NetworkType.CONNECTED)
                .setRequiresBatteryNotLow(true)
                .build()
            val request = PeriodicWorkRequestBuilder<AccountNotificationWorker>(PERIOD_HOURS, TimeUnit.HOURS)
                .setConstraints(constraints)
                .build()
            WorkManager.getInstance(context).enqueueUniquePeriodicWork(WORK_NAME, ExistingPeriodicWorkPolicy.KEEP, request)
        }
    }
}
