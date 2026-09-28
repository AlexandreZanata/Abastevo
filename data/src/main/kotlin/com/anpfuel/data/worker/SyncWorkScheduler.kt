package com.anpfuel.data.worker

import android.content.Context
import androidx.work.Constraints
import androidx.work.Data
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.workDataOf
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.AutoSyncOnWifiRule
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.concurrent.TimeUnit
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Enqueues periodic and manual UC-001 sync work (BR-014).
 */
@Singleton
class SyncWorkScheduler @Inject constructor(
    @ApplicationContext private val context: Context,
    private val userPreferencesRepository: UserPreferencesRepository,
) {

    suspend fun schedulePeriodicSync() {
        val preferences = userPreferencesRepository.getPreferences()
        val networkType = if (AutoSyncOnWifiRule.requiresUnmeteredNetwork(preferences)) {
            NetworkType.UNMETERED
        } else {
            NetworkType.CONNECTED
        }

        val constraints = Constraints.Builder()
            .setRequiredNetworkType(networkType)
            .build()

        val request = PeriodicWorkRequestBuilder<SyncWorker>(PERIODIC_INTERVAL_DAYS, TimeUnit.DAYS)
            .setConstraints(constraints)
            .setInputData(scheduledInputData())
            .build()

        WorkManager.getInstance(context).enqueueUniquePeriodicWork(
            PERIODIC_SYNC_WORK_NAME,
            ExistingPeriodicWorkPolicy.UPDATE,
            request,
        )
    }

    fun enqueueManualSync() {
        val constraints = Constraints.Builder()
            .setRequiredNetworkType(NetworkType.CONNECTED)
            .build()

        val request = OneTimeWorkRequestBuilder<SyncWorker>()
            .setConstraints(constraints)
            .setInputData(manualInputData())
            .build()

        WorkManager.getInstance(context).enqueueUniqueWork(
            MANUAL_SYNC_WORK_NAME,
            ExistingWorkPolicy.KEEP,
            request,
        )
    }

    fun enqueueFirstLaunchSync() {
        val constraints = Constraints.Builder()
            .setRequiredNetworkType(NetworkType.CONNECTED)
            .build()

        val request = OneTimeWorkRequestBuilder<SyncWorker>()
            .setConstraints(constraints)
            .setInputData(firstLaunchInputData())
            .build()

        WorkManager.getInstance(context).enqueueUniqueWork(
            FIRST_LAUNCH_SYNC_WORK_NAME,
            ExistingWorkPolicy.KEEP,
            request,
        )
    }

    fun enqueuePriceDropEvaluation() {
        SyncWorkScheduler.enqueuePriceDropEvaluation(context)
    }

    companion object {
        const val PERIODIC_SYNC_WORK_NAME = "anp_periodic_sync"
        const val MANUAL_SYNC_WORK_NAME = "anp_manual_sync"
        const val FIRST_LAUNCH_SYNC_WORK_NAME = "anp_first_launch_sync"
        const val RETENTION_CLEANUP_WORK_NAME = "anp_retention_cleanup"
        const val PRICE_DROP_EVALUATION_WORK_NAME = "anp_price_drop_evaluation"
        private const val PERIODIC_INTERVAL_DAYS = 7L

        fun scheduledInputData(): Data = workDataOf(
            SyncWorker.KEY_SOURCE to SyncWorker.SOURCE_SCHEDULED,
        )

        fun manualInputData(): Data = workDataOf(
            SyncWorker.KEY_SOURCE to SyncWorker.SOURCE_MANUAL,
        )

        fun firstLaunchInputData(): Data = workDataOf(
            SyncWorker.KEY_SOURCE to SyncWorker.SOURCE_FIRST_LAUNCH,
        )

        fun enqueueRetentionCleanup(context: Context) {
            val request = OneTimeWorkRequestBuilder<RetentionCleanupWorker>().build()
            WorkManager.getInstance(context).enqueueUniqueWork(
                RETENTION_CLEANUP_WORK_NAME,
                ExistingWorkPolicy.KEEP,
                request,
            )
        }

        fun enqueuePriceDropEvaluation(context: Context) {
            val request = OneTimeWorkRequestBuilder<PriceDropEvaluationWorker>().build()
            WorkManager.getInstance(context).enqueueUniqueWork(
                PRICE_DROP_EVALUATION_WORK_NAME,
                ExistingWorkPolicy.REPLACE,
                request,
            )
        }
    }
}
