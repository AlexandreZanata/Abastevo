package com.anpfuel.application.usecase.contribution

import com.anpfuel.domain.exception.ContributionPhotoExpired
import com.anpfuel.domain.exception.ContributionPhotoRejected
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.ContributionRateLimited
import com.anpfuel.domain.repository.ContributionReceipt
import com.anpfuel.domain.repository.ContributionRemoteStatus
import com.anpfuel.domain.repository.ContributionSubmissionGateway
import com.anpfuel.domain.repository.OutboxBudgets
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
        // SEC06-C bounded selection: one requested command by id, or
        // the head of one bounded eligible page. Full-history scans
        // (and loading all payloads to select one) are gone; owner,
        // origin, frozen revision and lease semantics are unchanged.
        val next = if (requestedId.isNullOrBlank()) {
            outbox.listDispatchablePage(now, OutboxBudgets.DISPATCH_PAGE_SIZE).firstOrNull()
        } else {
            outbox.findDispatchable(requestedId, now)
        }
        if (next == null) {
            val waiting = !requestedId.isNullOrBlank() && outbox.findOwned(requestedId)?.let {
                it.phase != com.anpfuel.domain.repository.OwnedContributionPhase.CANCELLED &&
                    it.remoteStatus !in setOf(ContributionRemoteStatus.VALIDATED, ContributionRemoteStatus.REJECTED, ContributionRemoteStatus.EXPIRED)
            } == true
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
        } catch (limited: ContributionRateLimited) {
            // SEC06-D: honor the server's Retry-After (bounded by the
            // backoff ceiling in the repository) instead of hammering
            // the fixed curve.
            outbox.failDispatch(next.commandId,next.revision,attempt,nowMillis(),limited.retryAfterSeconds * 1000L)
            DispatchOutcome.RETRY
        } catch (refused: DomainException) {
            // SEC06-D: permanent refusals park distinctly (never
            // eligible again, stable reason); anything else keeps the
            // transient backoff. Retrying a permanently refused intent
            // can never help.
            if (refused.message in PERMANENT_REFUSALS) {
                outbox.refusePermanently(next.commandId,next.revision,refused.message ?: "contribution.refused")
                DispatchOutcome.REFUSED
            } else {
                outbox.failDispatch(next.commandId,next.revision,attempt,nowMillis())
                DispatchOutcome.REFUSED
            }
        } catch (_: Exception) {
            outbox.failDispatch(next.commandId,next.revision,attempt,nowMillis())
            DispatchOutcome.RETRY
        }
    }

    companion object {
        /**
         * SEC06-D permanent refusal allowlist: inputs that can never
         * become valid by retrying (scope/environment changed under
         * the queued intent, or a malformed receipt). Everything else
         * stays transient. Closed set by review, never extended
         * silently.
         */
        val PERMANENT_REFUSALS: Set<String> = setOf(
            "contribution.owner-or-origin-changed",
            "contribution.invalid-or-unscoped",
            "contribution.invalid-dispatch-receipt",
        )
    }
}
