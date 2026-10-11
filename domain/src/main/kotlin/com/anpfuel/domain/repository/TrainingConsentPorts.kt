package com.anpfuel.domain.repository

/**
 * CS11A — Training-consent wire contract (B-BR-CS06).
 *
 * Optional, default OFF, versioned server grant/revoke: the owner
 * reviews the current terms pair and binds each decision to a
 * per-device monotonic revision the server orders. A grant covers
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
) : Exception(message)

interface TrainingConsentGateway {
    suspend fun grantConsent(
        termsVersion: String,
        termsDigest: String,
        revision: Long,
    ): TrainingConsentReceipt

    suspend fun revokeConsent(
        revision: Long,
    ): TrainingConsentReceipt

    suspend fun consentStatus(): TrainingConsentStatus
}
