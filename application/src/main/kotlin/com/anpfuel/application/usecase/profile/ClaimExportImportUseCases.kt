package com.anpfuel.application.usecase.profile

import com.anpfuel.domain.profile.ClaimDeclaration
import com.anpfuel.domain.profile.ClaimExportDecision
import com.anpfuel.domain.profile.ClaimExportImportRule
import com.anpfuel.domain.profile.ClaimExportRequest
import com.anpfuel.domain.profile.ClaimImportDecision

/**
 * P32-T02 — Account-bound claim export/import orchestration.
 *
 * Validation runs before any I/O: guests, expired declarations, owner
 * mismatches, wrong types, oversize bytes and replays never reach
 * storage or network. The accepted import carries no certification of
 * signature or powers.
 */
interface ClaimDeclarationStore {
    suspend fun export(request: ClaimExportRequest): ClaimDeclaration

    suspend fun markNonceSeen(nonce: String)

    suspend fun isNonceSeen(nonce: String): Boolean
}

sealed interface ClaimImportOutcome {
    data object AcceptedPendingReview : ClaimImportOutcome

    data class Denied(val reason: ClaimImportDecision) : ClaimImportOutcome
}

class RequestClaimExportUseCase(
    private val store: ClaimDeclarationStore,
) {
    suspend operator fun invoke(signedIn: Boolean, request: ClaimExportRequest): ClaimExportDecision {
        val decision = ClaimExportImportRule.canExport(signedIn, request)
        if (decision !is ClaimExportDecision.Ready) return decision
        val issued = store.export(request)
        return ClaimExportDecision.Ready(declaration = issued)
    }
}

class SubmitSignedClaimUseCase(
    private val store: ClaimDeclarationStore,
) {
    suspend operator fun invoke(
        signedIn: Boolean,
        declaration: ClaimDeclaration,
        nowEpochSeconds: Long,
        callerAccountId: String,
        fileMime: String,
        fileBytes: Long,
    ): ClaimImportOutcome {
        val seen = store.isNonceSeen(declaration.nonce)
        val decision = ClaimExportImportRule.canImport(
            signedIn = signedIn,
            declaration = declaration,
            nowEpochSeconds = nowEpochSeconds,
            callerAccountId = callerAccountId,
            fileMime = fileMime,
            fileBytes = fileBytes,
            seenNonces = if (seen) setOf(declaration.nonce) else emptySet(),
        )
        if (decision !is ClaimImportDecision.Accepted) return ClaimImportOutcome.Denied(decision)
        store.markNonceSeen(declaration.nonce)
        return ClaimImportOutcome.AcceptedPendingReview
    }
}
