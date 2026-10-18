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
import com.anpfuel.domain.repository.UserPreferencesRepository
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import java.util.concurrent.TimeUnit

/**
 * Small public directory reads only; weekly ANP downloads keep their
 * existing policy. The opt-in public catalog preference gates every
 * preventive download: disabled (the default) performs no network and
 * leaves manual navigation, manual refresh, contributions and privacy
 * tasks untouched. A successful sync stamps the last-sync marker.
 */
@HiltWorker
class DirectorySyncWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val getServerStations: GetServerStationsUseCase,
    private val userPreferencesRepository: UserPreferencesRepository,
) : CoroutineWorker(appContext, params) {
    override suspend fun doWork(): Result {
        if (!userPreferencesRepository.getPreferences().syncPublicCityCatalog) {
            return Result.success()
        }
        return when (getServerStations(limit = PAGE_SIZE)) {
            ServerStationsOutcome.Disabled -> Result.success()
            is ServerStationsOutcome.Fresh -> {
                val preferences = userPreferencesRepository.getPreferences()
                userPreferencesRepository.savePreferences(
                    preferences.copy(publicCityCatalogLastSyncedAtMillis = System.currentTimeMillis()),
                )
                Result.success()
            }
            is ServerStationsOutcome.StaleCache, is ServerStationsOutcome.Unavailable ->
                if (runAttemptCount < MAX_RETRIES) Result.retry() else Result.failure()
        }
    }

    companion object {
        const val WORK_NAME = "station_directory_sync"

        /** First page only; continuations stay user-initiated from the list. */
        const val PAGE_SIZE = 20

        const val MAX_RETRIES = 3

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
