package com.anpfuel.application.usecase.privacy

import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.application.port.TrainingConsentLocalStore
import com.anpfuel.domain.repository.TrainingConsentException
import com.anpfuel.domain.repository.TrainingConsentGateway
import com.anpfuel.domain.repository.TrainingConsentReceipt
import com.anpfuel.domain.repository.TrainingConsentRejectKind
import com.anpfuel.domain.repository.TrainingConsentStatus
import kotlinx.coroutines.flow.first

/**
 * CS11A — Training-consent decisions (B-BR-CS06, BUC-CS05).
 *
 * Guest callers ([TrainingConsentCaller] null) perform no IO and
 * claim nothing server-side. Grants refuse offline without queueing:
 * an ON decision needs the reviewed current terms and a live
 * receipt. Revokes queue offline as a pending revocation the UI
 * surfaces until [syncPending] uploads it. Every decision burns one
 * persisted per-device revision for server-side ordering; identical
 * retries converge server-side (`replayed = true`). Rejections carry
 * fixed kinds with sanitized messages.
 */
data class TrainingConsentCaller(val accountId: String)

sealed interface TrainingConsentOutcome {
    data object Guest : TrainingConsentOutcome
    data object Offline : TrainingConsentOutcome
    data object RevocationPending : TrainingConsentOutcome
    data class Status(
        val status: TrainingConsentStatus,
        val pendingRevocation: Boolean,
    ) : TrainingConsentOutcome
    data class Granted(val receipt: TrainingConsentReceipt) : TrainingConsentOutcome
    data class Revoked(val receipt: TrainingConsentReceipt) : TrainingConsentOutcome
    data class Rejected(
        val kind: TrainingConsentRejectKind,
        val message: String,
    ) : TrainingConsentOutcome
}

class TrainingConsentUseCase(
    private val local: TrainingConsentLocalStore,
    private val gateway: TrainingConsentGateway,
    private val network: NetworkConnectivityGateway,
) {

    suspend fun status(caller: TrainingConsentCaller?): TrainingConsentOutcome {
        if (caller == null) {
            return TrainingConsentOutcome.Guest
        }
        if (!network.observeIsConnected().first()) {
            return TrainingConsentOutcome.Offline
        }
        return try {
            TrainingConsentOutcome.Status(gateway.consentStatus(), local.hasPendingRevocation())
        } catch (error: TrainingConsentException) {
            TrainingConsentOutcome.Rejected(error.kind, error.message.orEmpty())
        }
    }

    suspend fun grant(
        caller: TrainingConsentCaller?,
        termsVersion: String,
        termsDigest: String,
    ): TrainingConsentOutcome {
        if (caller == null) {
            return TrainingConsentOutcome.Guest
        }
        require(termsVersion.isNotBlank()) { "terms_version is blank" }
        require(termsDigest.isNotBlank()) { "terms_digest is blank" }
        if (!network.observeIsConnected().first()) {
            return TrainingConsentOutcome.Offline
        }
        return try {
            val receipt = gateway.grantConsent(termsVersion, termsDigest, local.nextRevision())
            local.setToggleOn(true)
            TrainingConsentOutcome.Granted(receipt)
        } catch (error: TrainingConsentException) {
            TrainingConsentOutcome.Rejected(error.kind, error.message.orEmpty())
        }
    }

    suspend fun revoke(caller: TrainingConsentCaller?): TrainingConsentOutcome {
        if (caller == null) {
            return TrainingConsentOutcome.Guest
        }
        if (!network.observeIsConnected().first()) {
            local.setToggleOn(false)
            local.setPendingRevocation(true)
            return TrainingConsentOutcome.RevocationPending
        }
        return try {
            val receipt = gateway.revokeConsent(local.nextRevision())
            local.setToggleOn(false)
            local.setPendingRevocation(false)
            TrainingConsentOutcome.Revoked(receipt)
        } catch (error: TrainingConsentException) {
            TrainingConsentOutcome.Rejected(error.kind, error.message.orEmpty())
        }
    }

    suspend fun syncPending(caller: TrainingConsentCaller?): TrainingConsentOutcome {
        if (caller == null) {
            return TrainingConsentOutcome.Guest
        }
        if (!local.hasPendingRevocation()) {
            return status(caller)
        }
        if (!network.observeIsConnected().first()) {
            return TrainingConsentOutcome.RevocationPending
        }
        return try {
            val receipt = gateway.revokeConsent(local.nextRevision())
            local.setToggleOn(false)
            local.setPendingRevocation(false)
            TrainingConsentOutcome.Revoked(receipt)
        } catch (error: TrainingConsentException) {
            TrainingConsentOutcome.Rejected(error.kind, error.message.orEmpty())
        }
    }
}
