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
import androidx.work.workDataOf
import com.anpfuel.application.usecase.directory.ServerStationsOutcome
import com.anpfuel.application.usecase.directory.SyncPublicCatalogUseCase
import com.anpfuel.domain.repository.UserPreferencesRepository
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import java.util.concurrent.TimeUnit

/**
 * Optional public city catalog sync. Small public directory reads
 * only: pages the city list up to [SyncPublicCatalogUseCase.MAX_PAGES],
 * never station details. Weekly ANP downloads keep their existing
 * policy.
 *
 * Unique periodic work carrying its city: scheduling the same city
 * replaces constraints/input, a stale city no-ops so only the current
 * schedule downloads. Disabling the opt-in stops preventive downloads
 * (manual navigation, refresh, contributions and privacy tasks are
 * untouched); a cancelled run resumes on the next period. Only a fully
 * drained traversal stamps the last-sync marker — a partial run keeps
 * its cached pages but stays unmarked.
 */
@HiltWorker
class DirectorySyncWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val syncPublicCatalog: SyncPublicCatalogUseCase,
    private val userPreferencesRepository: UserPreferencesRepository,
) : CoroutineWorker(appContext, params) {
    override suspend fun doWork(): Result {
        val preferences = userPreferencesRepository.getPreferences()
        val scheduledCity = inputData.getString(KEY_MUNICIPALITY)
        if (scheduledCity != null && scheduledCity != preferences.preferredMunicipality) {
            return Result.success()
        }
        val result = syncPublicCatalog.sync()
        return when (result.outcome) {
            ServerStationsOutcome.Disabled -> Result.success()
            is ServerStationsOutcome.Fresh -> if (result.complete) {
                userPreferencesRepository.savePreferences(
                    preferences.copy(publicCityCatalogLastSyncedAtMillis = System.currentTimeMillis()),
                )
                Result.success()
            } else {
                Result.success()
            }
            is ServerStationsOutcome.StaleCache, is ServerStationsOutcome.Unavailable ->
                if (runAttemptCount < MAX_RETRIES) Result.retry() else Result.failure()
        }
    }

    companion object {
        const val WORK_NAME = "station_directory_sync"
        const val KEY_MUNICIPALITY = "municipality_code"
        const val MAX_RETRIES = 3

        internal fun request(municipalityCode: String?, requireUnmetered: Boolean) =
            PeriodicWorkRequestBuilder<DirectorySyncWorker>(15, TimeUnit.MINUTES)
                .setInputData(workDataOf(KEY_MUNICIPALITY to municipalityCode))
                .setConstraints(
                    Constraints.Builder().setRequiredNetworkType(
                        if (requireUnmetered) NetworkType.UNMETERED else NetworkType.CONNECTED,
                    ).build(),
                )
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
                .build()

        /** Unique per app; REPLACE keeps one schedule with fresh input/constraints. */
        fun schedule(context: Context, municipalityCode: String?, requireUnmetered: Boolean) {
            WorkManager.getInstance(context).enqueueUniquePeriodicWork(
                WORK_NAME, ExistingPeriodicWorkPolicy.REPLACE, request(municipalityCode, requireUnmetered),
            )
        }

        fun cancel(context: Context) {
            WorkManager.getInstance(context).cancelUniqueWork(WORK_NAME)
        }
    }
}
