package com.anpfuel.data.worker

import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.Data
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import java.util.concurrent.TimeUnit

/**
 * Owned scheduler for public map downloads. Public maps require no login;
 * inputs are generation + base URL only, so account switch/logout never
 * cancels or leaks private state into the download.
 */
object MapDownloadScheduler {
    fun request(generation: String, baseUrl: String) =
        OneTimeWorkRequestBuilder<MapDownloadWorker>()
            .setInputData(
                Data.Builder()
                    .putString(MapDownloadWorker.KEY_GENERATION, generation)
                    .putString(MapDownloadWorker.KEY_BASE_URL, baseUrl)
                    .build(),
            )
            .setConstraints(
                Constraints.Builder()
                    .setRequiredNetworkType(NetworkType.CONNECTED)
                    .setRequiresStorageNotLow(true)
                    .build(),
            )
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
            .addTag("map-download")
            .build()

    fun uniqueName(generation: String) = "${MapDownloadWorker.UNIQUE_PREFIX}$generation"

    fun enqueue(workManager: WorkManager, generation: String, baseUrl: String) {
        workManager.enqueueUniqueWork(
            uniqueName(generation),
            ExistingWorkPolicy.KEEP,
            request(generation, baseUrl),
        )
    }

    fun cancel(workManager: WorkManager, generation: String) {
        workManager.cancelUniqueWork(uniqueName(generation))
    }
}
