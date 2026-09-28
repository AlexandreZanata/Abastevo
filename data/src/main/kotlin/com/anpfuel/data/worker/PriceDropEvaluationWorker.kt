package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.anpfuel.application.usecase.alert.EvaluatePriceDropAlertsUseCase
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/**
 * UC-014 — Evaluates price drop alerts after weekly import completes.
 */
@HiltWorker
class PriceDropEvaluationWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val evaluatePriceDropAlertsUseCase: EvaluatePriceDropAlertsUseCase,
) : CoroutineWorker(appContext, params) {

    override suspend fun doWork(): Result {
        return try {
            evaluatePriceDropAlertsUseCase()
            Result.success()
        } catch (_: Exception) {
            Result.retry()
        }
    }

    companion object {
        internal fun createForTest(
            context: Context,
            params: WorkerParameters,
            evaluatePriceDropAlertsUseCase: EvaluatePriceDropAlertsUseCase,
        ): PriceDropEvaluationWorker = PriceDropEvaluationWorker(
            context,
            params,
            evaluatePriceDropAlertsUseCase,
        )
    }
}
