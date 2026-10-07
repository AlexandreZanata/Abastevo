package com.anpfuel.application.usecase.contribution

import com.anpfuel.domain.exception.ContributionPhotoExpired
import com.anpfuel.domain.exception.ContributionPhotoRejected
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.ContributionReceipt
import com.anpfuel.domain.repository.ContributionRemoteStatus
import com.anpfuel.domain.repository.ContributionSubmissionGateway
import java.util.UUID
import kotlinx.coroutines.CancellationException

enum class DispatchOutcome { DONE, RETRY, REFUSED }

/** Atomic lease; exact frozen revision; durable pending receipts and signed owner refresh. */
class DispatchContributionUseCase(
    private val outbox: ContributionOutboxRepository,
    private val gateway: ContributionSubmissionGateway,
    private val nowMillis: () -> Long = System::currentTimeMillis,
    private val attemptId: () -> String = { UUID.randomUUID().toString() },
) {
    suspend fun invoke(requestedId: String?): DispatchOutcome {
        val now = nowMillis()
        val pending = outbox.listDispatchable(now)
        val next = if (requestedId.isNullOrBlank()) pending.firstOrNull() else pending.firstOrNull { it.commandId == requestedId }
        if (next == null) {
            val waiting = outbox.listOwned().any { it.commandId == requestedId &&
                it.phase != com.anpfuel.domain.repository.OwnedContributionPhase.CANCELLED &&
                it.remoteStatus !in setOf(ContributionRemoteStatus.VALIDATED, ContributionRemoteStatus.REJECTED, ContributionRemoteStatus.EXPIRED) }
            return if (waiting) DispatchOutcome.RETRY else DispatchOutcome.DONE
        }
        val attempt = attemptId()
        if (!outbox.claimDispatch(next.commandId, next.revision, attempt, now)) return DispatchOutcome.RETRY
        return try {
            val previous = outbox.receipt(next.commandId)
            val receipt = if (previous == null) gateway.submit(next.commandId, next.revision, next.payloadJson, attempt)
                else gateway.refresh(previous, next.payloadJson)
            if (receipt.commandId != next.commandId || receipt.revision != next.revision || receipt.status == ContributionRemoteStatus.QUEUED) {
                throw DomainException("contribution.invalid-dispatch-receipt")
            }
            outbox.recordReceipt(receipt, nowMillis())
            if (receipt.status in setOf(ContributionRemoteStatus.RECEIVED, ContributionRemoteStatus.VALIDATING)) DispatchOutcome.RETRY else DispatchOutcome.DONE
        } catch (cancelled: CancellationException) { throw cancelled
        } catch (_: ContributionPhotoExpired) {
            outbox.recordReceipt(ContributionReceipt(next.commandId,next.revision,ContributionRemoteStatus.EXPIRED,reason="contribution.photo-expired"),nowMillis())
            DispatchOutcome.DONE
        } catch (_: ContributionPhotoRejected) {
            outbox.recordReceipt(ContributionReceipt(next.commandId,next.revision,ContributionRemoteStatus.REJECTED,reason="contribution.photo-rejected"),nowMillis())
            DispatchOutcome.DONE
        } catch (_: DomainException) {
            outbox.failDispatch(next.commandId,next.revision,attempt,nowMillis())
            DispatchOutcome.REFUSED
        } catch (_: Exception) {
            outbox.failDispatch(next.commandId,next.revision,attempt,nowMillis())
            DispatchOutcome.RETRY
        }
    }
}
