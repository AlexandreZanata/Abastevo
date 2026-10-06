package com.anpfuel.domain.profile

/**
 * P32-T02 — Claim declaration and export/import decisions.
 *
 * The declaration binds account/station/operator/scopes with a one-use
 * nonce digest and TTL. Import acceptance never certifies signature or
 * corporate powers: independent review owns approval.
 */
data class ClaimDeclaration(
    val version: Int,
    val accountId: String,
    val stationId: String,
    val operatorName: String,
    val scopes: Set<String>,
    val nonce: String,
    val issuedAtEpochSeconds: Long,
    val ttlSeconds: Long,
)

data class ClaimExportRequest(
    val accountId: String,
    val stationId: String,
    val scopes: Set<String>,
)

sealed interface ClaimExportDecision {
    data object DeniedGuest : ClaimExportDecision

    data object DeniedInvalid : ClaimExportDecision

    data class Ready(val declaration: ClaimDeclaration) : ClaimExportDecision
}

sealed interface ClaimImportDecision {
    data object DeniedGuest : ClaimImportDecision

    data object DeniedExpired : ClaimImportDecision

    data object DeniedOwnerMismatch : ClaimImportDecision

    data object DeniedFileType : ClaimImportDecision

    data object DeniedTooLarge : ClaimImportDecision

    data object DeniedReplay : ClaimImportDecision

    data class Accepted(val certifiesPowers: Boolean = false) : ClaimImportDecision
}

object ClaimExportImportRule {

    const val MAX_SIGNED_BYTES: Long = 5L * 1024L * 1024L

    private val allowedMimes = setOf("application/pdf")

    private val allowedScopes = setOf("edit_hours", "edit_services", "edit_contact", "reply_reviews")

    fun canExport(signedIn: Boolean, request: ClaimExportRequest): ClaimExportDecision {
        if (!signedIn || request.accountId.isBlank()) return ClaimExportDecision.DeniedGuest
        if (request.stationId.isBlank() || request.scopes.isEmpty()) return ClaimExportDecision.DeniedInvalid
        if (!allowedScopes.containsAll(request.scopes)) return ClaimExportDecision.DeniedInvalid
        return ClaimExportDecision.Ready(
            declaration = ClaimDeclaration(
                version = 1,
                accountId = request.accountId,
                stationId = request.stationId,
                operatorName = "",
                scopes = request.scopes,
                nonce = "",
                issuedAtEpochSeconds = 0L,
                ttlSeconds = 900L,
            ),
        )
    }

    fun canImport(
        signedIn: Boolean,
        declaration: ClaimDeclaration,
        nowEpochSeconds: Long,
        callerAccountId: String,
        fileMime: String,
        fileBytes: Long,
        seenNonces: Set<String>,
    ): ClaimImportDecision {
        if (!signedIn || callerAccountId.isBlank()) return ClaimImportDecision.DeniedGuest
        if (nowEpochSeconds > declaration.issuedAtEpochSeconds + declaration.ttlSeconds) {
            return ClaimImportDecision.DeniedExpired
        }
        if (callerAccountId != declaration.accountId) return ClaimImportDecision.DeniedOwnerMismatch
        if (!allowedMimes.contains(fileMime)) return ClaimImportDecision.DeniedFileType
        if (fileBytes > MAX_SIGNED_BYTES) return ClaimImportDecision.DeniedTooLarge
        if (seenNonces.contains(declaration.nonce)) return ClaimImportDecision.DeniedReplay
        return ClaimImportDecision.Accepted(certifiesPowers = false)
    }
}
