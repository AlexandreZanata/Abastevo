package com.anpfuel.application.usecase.safety

import com.anpfuel.domain.safety.BlockReceipt
import com.anpfuel.domain.safety.BlockedUser
import com.anpfuel.domain.safety.CommunitySafetyException
import com.anpfuel.domain.safety.CommunitySafetyGateway
import com.anpfuel.domain.safety.SafetyContentRef
import com.anpfuel.domain.safety.UserReportDetail
import com.anpfuel.domain.safety.UserReportReason
import com.anpfuel.domain.safety.UserReportReceipt
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.withTimeoutOrNull

private val BLOCK_ID = Regex("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")

private fun requireWellFormed(ref: SafetyContentRef) {
    if (!ref.isWellFormed) throw CommunitySafetyException.BadRequest("safety.bad-id")
}

/**
 * Reports the author of a public content reference (B-BR-SAFE-04,
 * BUC-SAFE-02). The optional detail is normalized and refused locally over
 * 200 Unicode scalars; a repeat returns the first receipt as replayed.
 */
class ReportCommunityUserUseCase(private val gateway: CommunitySafetyGateway) {
    suspend operator fun invoke(
        ref: SafetyContentRef,
        reason: UserReportReason,
        detail: String? = null,
    ): UserReportReceipt {
        requireWellFormed(ref)
        if (!UserReportDetail.isValid(detail)) throw CommunitySafetyException.BadRequest("safety.detail-too-long")
        return gateway.reportUser(ref, reason, UserReportDetail.normalize(detail))
    }
}

/** Blocks the author of a public content reference (B-BR-SAFE-05, BUC-SAFE-03). */
class BlockCommunityUserUseCase(
    private val gateway: CommunitySafetyGateway,
    private val state: CommunityBlockState,
) {
    suspend operator fun invoke(ref: SafetyContentRef): BlockReceipt {
        requireWellFormed(ref)
        val receipt = gateway.blockUser(ref)
        state.onBlocked(receipt.replayed)
        return receipt
    }
}

/** Removes one of the caller's blocks; effective at once (B-BR-SAFE-07, BUC-SAFE-04). */
class UnblockCommunityUserUseCase(
    private val gateway: CommunitySafetyGateway,
    private val state: CommunityBlockState,
) {
    suspend operator fun invoke(blockId: String) {
        val id = blockId.trim()
        if (!BLOCK_ID.matches(id)) throw CommunitySafetyException.BadRequest("safety.bad-id")
        try {
            gateway.removeBlock(id.lowercase())
        } catch (e: CommunitySafetyException.NotFound) {
            // Someone else's view of the list was stale: recount on the next question.
            state.invalidate()
            throw e
        }
        state.onUnblocked()
    }
}

/** The caller's blocks, newest first (B-BR-SAFE-07). */
class ListBlockedUsersUseCase(
    private val gateway: CommunitySafetyGateway,
    private val state: CommunityBlockState,
) {
    suspend operator fun invoke(): List<BlockedUser> {
        val items = gateway.listBlocks()
        state.onListed(items)
        return items
    }
}

/**
 * Which of the shown content references were written by accounts the
 * caller blocked (B-BR-SAFE-06). Skips the question while the account has
 * no blocks, asks in batches of at most 100 references and fails open:
 * any refusal, outage or slow answer shows the content instead of failing
 * the screen.
 */
class FilterHiddenContentUseCase(
    private val gateway: CommunitySafetyGateway,
    private val state: CommunityBlockState,
    private val budgetMillis: Long = LOOKUP_BUDGET_MILLIS,
) {
    /** Increases on every block or unblock made in this process. */
    val blockRevisions: StateFlow<Long>
        get() = state.revisions

    suspend operator fun invoke(refs: Collection<SafetyContentRef>): Set<SafetyContentRef> {
        val asked = refs.filter { it.isWellFormed }.distinct()
        if (asked.isEmpty()) return emptySet()
        return withTimeoutOrNull(budgetMillis) {
            if (!state.mayHaveBlocks()) return@withTimeoutOrNull emptySet()
            val hidden = mutableSetOf<SafetyContentRef>()
            for (batch in asked.chunked(MAX_BATCH)) {
                try {
                    hidden += gateway.hiddenContent(batch).filter { it in batch }
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {
                    break
                }
            }
            hidden
        } ?: emptySet()
    }

    companion object {
        /** Contract bound on one hidden-content question. */
        const val MAX_BATCH = 100

        /** A screen never waits longer than this for the answer. */
        const val LOOKUP_BUDGET_MILLIS = 5_000L
    }
}
