package com.anpfuel.data.worker

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.workDataOf
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.concurrent.TimeUnit
import javax.inject.Inject
import javax.inject.Singleton

/**
 * MEX-14 ledger sync scheduling: unique one-shot work with connectivity
 * constraints and bounded linear backoff. Duplicate or
 * reconnect-triggered schedules converge on the same unique work (KEEP),
 * so two triggers never fork two uploads of one command; the atomic
 * claim inside the worker is the final guard. Pausing is cancelling the
 * unique work; held rows stay untouched.
 */
@Singleton
class LedgerSyncScheduler @Inject constructor(
    @ApplicationContext private val context: Context,
) {

    fun enqueue(commandId: String?) {
        val constraints = Constraints.Builder()
            .setRequiredNetworkType(NetworkType.CONNECTED)
            .build()
        val input = if (commandId == null) {
            androidx.work.Data.EMPTY
        } else {
            workDataOf(LedgerSyncWorker.KEY_COMMAND_ID to commandId)
        }
        val request = OneTimeWorkRequestBuilder<LedgerSyncWorker>()
            .setConstraints(constraints)
            .setBackoffCriteria(BackoffPolicy.LINEAR, BACKOFF_MINUTES, TimeUnit.MINUTES)
            .setInputData(input)
            .build()
        WorkManager.getInstance(context).enqueueUniqueWork(
            WORK_NAME,
            ExistingWorkPolicy.KEEP,
            request,
        )
    }

    fun cancel() {
        WorkManager.getInstance(context).cancelUniqueWork(WORK_NAME)
    }

    companion object {
        const val WORK_NAME = "ledger_sync"
        const val BACKOFF_MINUTES = 30L
    }
}
