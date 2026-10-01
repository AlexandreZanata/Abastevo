package com.anpfuel.data.worker

import android.content.Context
import androidx.work.Constraints
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.workDataOf
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P10-T05 contribution queue scheduler (MIGRATION_PLAN outbox).
 *
 * Bounded retries with connectivity constraints, same operation id kept;
 * cancelling a non-sent draft is a distinct audited action via the
 * repository, never a queue delete of accepted history. Pausing is
 * cancelling the unique work per command id.
 */
@Singleton
class ContributionWorkScheduler @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    fun enqueueContribution(commandId: String) {
        val constraints = Constraints.Builder()
            .setRequiredNetworkType(NetworkType.CONNECTED)
            .build()
        val request = OneTimeWorkRequestBuilder<ContributionWorker>()
            .setConstraints(constraints)
            .setInputData(workDataOf(ContributionWorker.KEY_COMMAND_ID to commandId))
            .build()
        WorkManager.getInstance(context).enqueueUniqueWork(
            workNameFor(commandId),
            ExistingWorkPolicy.KEEP,
            request,
        )
    }

    fun cancelContribution(commandId: String) {
        WorkManager.getInstance(context).cancelUniqueWork(workNameFor(commandId))
    }

    companion object {
        fun workNameFor(commandId: String): String = "contribution_$commandId"
    }
}
