package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.anpfuel.application.usecase.alert.EvaluatePriceDropAlertsUseCase
import com.anpfuel.application.usecase.notification.EvaluateWeeklyLocalAlertsUseCase
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/**
 * UC-014 — Evaluates price drop alerts after weekly import completes, then
 * the B-BR-NOTIF-05 weekly local alerts (new week, city average drop).
 */
@HiltWorker
class PriceDropEvaluationWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val evaluatePriceDropAlertsUseCase: EvaluatePriceDropAlertsUseCase,
    private val weeklyLocalAlerts: EvaluateWeeklyLocalAlertsUseCase? = null,
) : CoroutineWorker(appContext, params) {

    override suspend fun doWork(): Result {
        return try {
            evaluatePriceDropAlertsUseCase()
            weeklyLocalAlerts?.invoke()
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
