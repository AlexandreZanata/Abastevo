package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.*
import com.anpfuel.data.enterprise.EnterpriseVaultJanitor
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.concurrent.TimeUnit
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

@HiltWorker
class EnterpriseVaultMaintenanceWorker @AssistedInject constructor(
    @Assisted context: Context, @Assisted parameters: WorkerParameters,
    private val janitor: EnterpriseVaultJanitor,
) : CoroutineWorker(context, parameters) {
    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        try {
            // Every slice is bounded and credentials-free. Retries retain durable denial intent.
            val sweep = janitor.newSweep()
            var failed = false
            do {
                kotlinx.coroutines.currentCoroutineContext().ensureActive()
                val result = janitor.runSlice(sweep)
                failed = failed || result.failed > 0
                val more = result.hasMore
                kotlinx.coroutines.yield()
            } while (more)
            if (failed) Result.retry() else Result.success()
        } catch (cancelled: kotlinx.coroutines.CancellationException) { throw cancelled }
        catch (_: Exception) { Result.retry() }
    }
}

/** OS scheduling is best-effort; every storage access independently enforces privacy expiry. */
@Singleton
class EnterpriseVaultMaintenanceScheduler @Inject constructor(@ApplicationContext private val context: Context) {
    fun maintain() {
        val periodic = PeriodicWorkRequestBuilder<EnterpriseVaultMaintenanceWorker>(15, TimeUnit.MINUTES).build()
        val startup = OneTimeWorkRequestBuilder<EnterpriseVaultMaintenanceWorker>()
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS).build()
        WorkManager.getInstance(context).enqueueUniquePeriodicWork("enterprise-vault-maintenance", ExistingPeriodicWorkPolicy.KEEP, periodic)
        WorkManager.getInstance(context).enqueueUniqueWork("enterprise-vault-startup", ExistingWorkPolicy.KEEP, startup)
    }
}
