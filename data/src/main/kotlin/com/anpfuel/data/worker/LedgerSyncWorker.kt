package com.anpfuel.data.worker

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.anpfuel.application.usecase.ledger.DispatchLedgerCommandUseCase
import com.anpfuel.application.usecase.ledger.LedgerDispatchOutcome
import androidx.hilt.work.HiltWorker
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import dagger.hilt.android.qualifiers.ApplicationContext

/**
 * MEX-14 durable ledger sync worker. One head-of-queue command per run
 * with the atomic-claim backstop: a duplicate worker loses the claim
 * and retries later, never double-submitting. Opt-in OFF is a quiet
 * success (nothing dispatched, queue kept). Lapsed sessions keep work
 * pending without I/O. Bounded retries via WorkManager backoff;
 * terminal conflicts park in the outbox (no auto-delete, no
 * silent drop).
 */
@HiltWorker
class LedgerSyncWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val dispatch: DispatchLedgerCommandUseCase,
) : CoroutineWorker(appContext, params) {

    override suspend fun doWork(): Result = toWorkResult(dispatch.invoke(inputData.getString(KEY_COMMAND_ID)))

    companion object {
        const val KEY_COMMAND_ID = "ledger_command_id"

        internal fun toWorkResult(outcome: LedgerDispatchOutcome): Result = when (outcome) {
            LedgerDispatchOutcome.DONE -> Result.success()
            LedgerDispatchOutcome.RETRY -> Result.retry()
            LedgerDispatchOutcome.REFUSED -> Result.failure()
        }
    }
}
