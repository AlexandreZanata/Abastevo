package com.anpfuel.domain.repository

import com.anpfuel.domain.model.ContributionScope

/**
 * CS11A — Training-consent wire contract (B-BR-CS06).
 *
 * Optional, default OFF, versioned server grant/revoke: the owner
 * reviews the current terms pair and binds each decision to a
 * server-seeded monotonic revision in an owner/key/origin journal. A grant covers
 * only future per-image selections; revocation stops selection at
 * receipt and starts owned training-asset deletion. Guest claims
 * nothing server-side.
 */
enum class TrainingConsentRejectKind {
    AUTH_REQUIRED,
    TERMS_OUTDATED,
    STALE_REVISION,
    CONFLICT,
    INVALID,
    TRANSPORT,
    OFFLINE,
}

data class TrainingConsentReceipt(
    val state: String,
    val revision: Long,
    val termsVersion: String,
    val replayed: Boolean,
    val deleteOwnedTrainingAssets: Boolean = false,
)

data class TrainingConsentStatus(
    val granted: Boolean,
    val state: String?,
    val termsVersion: String?,
    val termsDigest: String?,
    val revision: Long,
    val needsRegrant: Boolean,
)

class TrainingConsentException(
    val kind: TrainingConsentRejectKind,
    message: String?,
    val retryAfterSeconds: Long? = null,
) : Exception(message)

interface TrainingConsentGateway {
    suspend fun grantConsent(
        scope: ContributionScope,
        termsVersion: String,
        termsDigest: String,
        revision: Long,
        commandId: String,
    ): TrainingConsentReceipt

    suspend fun revokeConsent(
        scope: ContributionScope,
        revision: Long,
        commandId: String,
    ): TrainingConsentReceipt

    suspend fun consentStatus(scope: ContributionScope): TrainingConsentStatus
}
