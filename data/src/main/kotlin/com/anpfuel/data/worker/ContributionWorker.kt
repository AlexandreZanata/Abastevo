package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.anpfuel.application.port.ContributionOutboxFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.ContributionSubmissionGateway
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import java.io.IOException
import java.util.UUID

/**
 * P10-T05 durable contribution dispatcher (BUC-003/004, B-BR-005).
 *
 * Flag OFF pauses the queue (success no-op, nothing dispatched, history
 * kept). Otherwise dispatches one eligible command per run in FIFO order:
 * stable command id kept, fresh nonce per send, ack matches the exact
 * revision (stale acks never delete newer work), transport/finalize
 * failures record bounded backoff and retry with a new nonce. Pausing is
 * cancelling the unique work; accepted history is never deleted here.
 */
@HiltWorker
class ContributionWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val outbox: ContributionOutboxRepository,
    private val gateway: ContributionSubmissionGateway,
    private val flagProvider: ContributionOutboxFlagProvider,
) : CoroutineWorker(appContext, params) {

    override suspend fun doWork(): Result {
        if (!flagProvider.isEnabled()) {
            return Result.success()
        }
        val now = System.currentTimeMillis()
        val pending = outbox.listDispatchable(now)
        if (pending.isEmpty()) {
            return Result.success()
        }
        val requestedId = inputData.getString(KEY_COMMAND_ID)
        val next = if (requestedId.isNullOrBlank()) {
            pending.first()
        } else {
            pending.firstOrNull { it.commandId == requestedId } ?: pending.first()
        }
        val payload = outbox.loadPayload(next.commandId) ?: return Result.success()
        val nonce = UUID.randomUUID().toString()
        return try {
            outbox.markDispatched(next.commandId, nonce)
            gateway.submit(next.commandId, next.revision, payload, nonce)
            outbox.markAcknowledged(next.commandId, next.revision)
            Result.success()
        } catch (_: DomainException) {
            try {
                outbox.markFailed(next.commandId, System.currentTimeMillis())
            } catch (_: Exception) {
            }
            Result.failure()
        } catch (_: IOException) {
            try {
                outbox.markFailed(next.commandId, System.currentTimeMillis())
            } catch (_: Exception) {
            }
            Result.retry()
        } catch (_: Exception) {
            try {
                outbox.markFailed(next.commandId, System.currentTimeMillis())
            } catch (_: Exception) {
            }
            Result.retry()
        }
    }

    companion object {
        const val KEY_COMMAND_ID = "contribution_command_id"

        internal fun createForTest(
            context: Context,
            params: WorkerParameters,
            outbox: ContributionOutboxRepository,
            gateway: ContributionSubmissionGateway,
            flagProvider: ContributionOutboxFlagProvider,
        ): ContributionWorker = ContributionWorker(
            context,
            params,
            outbox,
            gateway,
            flagProvider,
        )
    }
}
