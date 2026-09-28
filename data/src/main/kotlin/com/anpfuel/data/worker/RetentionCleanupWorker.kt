package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.anpfuel.application.usecase.settings.ApplyStationDetailRetentionUseCase
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/**
 * BR-013 — Applies station detail retention outside the main sync transaction.
 */
@HiltWorker
class RetentionCleanupWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val applyStationDetailRetentionUseCase: ApplyStationDetailRetentionUseCase,
) : CoroutineWorker(appContext, params) {

    override suspend fun doWork(): Result {
        return try {
            applyStationDetailRetentionUseCase()
            Result.success()
        } catch (_: Exception) {
            Result.retry()
        }
    }

    companion object {
        internal fun createForTest(
            context: Context,
            params: WorkerParameters,
            applyStationDetailRetentionUseCase: ApplyStationDetailRetentionUseCase,
        ): RetentionCleanupWorker = RetentionCleanupWorker(
            context,
            params,
            applyStationDetailRetentionUseCase,
        )
    }
}
