package com.anpfuel.data.worker

import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.Data
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.await
import java.util.UUID
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.suspendCancellableCoroutine

/**
 * Owned scheduler for public map downloads. Public maps require no login;
 * inputs are generation + base URL only, so account switch/logout never
 * cancels or leaks private state into the download.
 */
object MapDownloadScheduler {
    fun request(
        generation: String,
        baseUrl: String,
        requireUnmetered: Boolean = true,
        id: UUID = UUID.randomUUID(),
    ) = OneTimeWorkRequestBuilder<MapDownloadWorker>()
        .setId(id)
        .setInputData(
            Data.Builder()
                .putString(MapDownloadWorker.KEY_GENERATION, generation)
                .putString(MapDownloadWorker.KEY_BASE_URL, baseUrl)
                .build(),
        )
        .setConstraints(
            Constraints.Builder()
                .setRequiredNetworkType(if (requireUnmetered) NetworkType.UNMETERED else NetworkType.CONNECTED)
                .setRequiresStorageNotLow(true)
                .build(),
        )
        .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
        .addTag("map-download")
        .build()

    fun uniqueName(generation: String) = "${MapDownloadWorker.UNIQUE_PREFIX}$generation"

    suspend fun enqueue(
        workManager: WorkManager,
        generation: String,
        baseUrl: String,
        requireUnmetered: Boolean,
    ) {
        val name = uniqueName(generation)
        workManager.enqueueUniqueWork(
            name,
            ExistingWorkPolicy.KEEP,
            request(generation, baseUrl, requireUnmetered),
        ).await()
        if (requireUnmetered) return

        // Preserve the ID and progress of any previously queued Wi-Fi-only work.
        val queued = workManager.getWorkInfosForUniqueWorkFlow(name).first().firstOrNull {
            !it.state.isFinished && it.constraints.requiredNetworkType == NetworkType.UNMETERED
        } ?: return
        val update = workManager.updateWork(
            request(generation, baseUrl, requireUnmetered = false, id = queued.id),
        )
        val result = suspendCancellableCoroutine<WorkManager.UpdateResult> { continuation ->
            update.addListener(
                { continuation.resumeWith(runCatching { update.get() }) },
                { command -> command.run() },
            )
        }
        check(result != WorkManager.UpdateResult.NOT_APPLIED) { "map-work-update-not-applied" }
    }

    fun cancel(workManager: WorkManager, generation: String) {
        workManager.cancelUniqueWork(uniqueName(generation))
    }
}
