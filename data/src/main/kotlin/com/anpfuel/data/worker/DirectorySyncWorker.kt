package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.anpfuel.application.usecase.directory.GetServerStationsUseCase
import com.anpfuel.application.usecase.directory.ServerStationsOutcome
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import java.util.concurrent.TimeUnit

/** Small public directory reads only; weekly ANP downloads keep their existing policy. */
@HiltWorker
class DirectorySyncWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val getServerStations: GetServerStationsUseCase,
) : CoroutineWorker(appContext, params) {
    override suspend fun doWork(): Result = when (getServerStations(limit = 100)) {
        ServerStationsOutcome.Disabled, is ServerStationsOutcome.Fresh -> Result.success()
        is ServerStationsOutcome.StaleCache, is ServerStationsOutcome.Unavailable ->
            if (runAttemptCount < 3) Result.retry() else Result.failure()
    }

    companion object {
        const val WORK_NAME = "station_directory_sync"

        internal fun request() = PeriodicWorkRequestBuilder<DirectorySyncWorker>(15, TimeUnit.MINUTES)
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
            .build()

        fun schedule(context: Context) {
            WorkManager.getInstance(context).enqueueUniquePeriodicWork(
                WORK_NAME, ExistingPeriodicWorkPolicy.KEEP, request(),
            )
        }
    }
}
