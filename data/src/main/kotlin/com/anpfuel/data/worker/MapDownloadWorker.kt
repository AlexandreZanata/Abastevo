package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.anpfuel.data.map.MapDownloadException
import com.anpfuel.data.map.MapPackageRepository
import com.anpfuel.data.map.MapStorageException
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import java.io.IOException

@HiltWorker
class MapDownloadWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val repository: MapPackageRepository,
) : CoroutineWorker(appContext, params) {
    override suspend fun doWork(): Result {
        val generation = inputData.getString(KEY_GENERATION) ?: return Result.failure()
        val baseUrl = inputData.getString(KEY_BASE_URL) ?: return Result.failure()
        return try {
            val downloaded = repository.downloadMissing(generation, baseUrl)
            if (downloaded && repository.activate(generation)) Result.success() else Result.retry()
        } catch (_: MapStorageException) {
            Result.failure()
        } catch (_: MapDownloadException) {
            Result.failure()
        } catch (_: IOException) {
            Result.retry()
        }
    }

    companion object {
        const val KEY_GENERATION = "generation"
        const val KEY_BASE_URL = "base_url"
        const val UNIQUE_PREFIX = "map-download-"
    }
}
