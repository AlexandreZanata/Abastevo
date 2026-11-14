package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.ListenableWorker
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.anpfuel.data.enterprise.EnterpriseOperationWorkNext
import com.anpfuel.data.enterprise.EnterpriseOperationWorkRunner
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.concurrent.TimeUnit
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** Actual protected-source delivery uses the application's existing Hilt worker factory. */
@HiltWorker
class EnterpriseOperationWorker @AssistedInject constructor(
    @Assisted context: Context,
    @Assisted parameters: WorkerParameters,
    private val runner: EnterpriseOperationWorkRunner,
) : CoroutineWorker(context, parameters) {
    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        try {
            val fromContinuation = EnterpriseOperationScheduler.CONTINUATION in tags
            EnterpriseOperationWorkDecision.result(runner.runOnce().next) { delay ->
                EnterpriseOperationScheduler.continueDelivery(applicationContext, delay, fromContinuation)
            }
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            Result.retry()
        }
    }
}

/**
 * Maps one invocation's outcome to WorkManager (EDR-122): signed-out/idle work ends without a
 * reschedule, a known due time becomes a delayed continuation instead of blind backoff, and an
 * unknown failure keeps the ordinary system backoff. A continuation that cannot be enqueued
 * falls back to that backoff, so retained work is never stranded.
 */
internal object EnterpriseOperationWorkDecision {
    fun result(next: EnterpriseOperationWorkNext, continueAfter: (Long) -> Boolean): ListenableWorker.Result = when (next) {
        EnterpriseOperationWorkNext.Done -> ListenableWorker.Result.success()
        EnterpriseOperationWorkNext.Backoff -> ListenableWorker.Result.retry()
        is EnterpriseOperationWorkNext.Continue -> {
            val scheduled = try { continueAfter(next.delayMillis) }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (_: Exception) { false }
            if (scheduled) ListenableWorker.Result.success() else ListenableWorker.Result.retry()
        }
    }

    /** A running continuation appends its successor; any other run replaces a pending one with fresher timing. */
    fun policy(fromContinuation: Boolean): ExistingWorkPolicy =
        if (fromContinuation) ExistingWorkPolicy.APPEND_OR_REPLACE else ExistingWorkPolicy.REPLACE
}

/** Work input contains no owner, partition, enrollment, credential or queue cursor. */
@Singleton
class EnterpriseOperationScheduler @Inject constructor(@ApplicationContext private val context: Context) {
    fun schedule() {
        val constraints = Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build()
        val periodic = PeriodicWorkRequestBuilder<EnterpriseOperationWorker>(15, TimeUnit.MINUTES)
            .setConstraints(constraints).build()
        WorkManager.getInstance(context).enqueueUniquePeriodicWork(PERIODIC, ExistingPeriodicWorkPolicy.KEEP, periodic)
        requestDelivery()
    }

    fun requestDelivery() {
        val request = OneTimeWorkRequestBuilder<EnterpriseOperationWorker>()
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS).build()
        WorkManager.getInstance(context).enqueueUniqueWork(STARTUP, ExistingWorkPolicy.KEEP, request)
    }

    companion object {
        const val PERIODIC = "enterprise-operation-delivery"
        const val STARTUP = "enterprise-operation-delivery-startup"
        const val CONTINUATION = "enterprise-operation-delivery-continuation"

        /** Delayed one-time continuation with empty input; false when it could not be enqueued. */
        internal fun continueDelivery(context: Context, delayMillis: Long, fromContinuation: Boolean): Boolean = try {
            val request = OneTimeWorkRequestBuilder<EnterpriseOperationWorker>()
                .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
                .setInitialDelay(delayMillis, TimeUnit.MILLISECONDS).addTag(CONTINUATION)
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS).build()
            WorkManager.getInstance(context).enqueueUniqueWork(CONTINUATION,
                EnterpriseOperationWorkDecision.policy(fromContinuation), request)
            true
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            false
        }
    }
}
