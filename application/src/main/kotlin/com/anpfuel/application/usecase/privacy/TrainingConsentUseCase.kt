package com.anpfuel.application.usecase.privacy

import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.application.port.TrainingConsentLocalStore
import com.anpfuel.domain.repository.TrainingConsentException
import com.anpfuel.domain.repository.TrainingConsentGateway
import com.anpfuel.domain.repository.TrainingConsentReceipt
import com.anpfuel.domain.repository.TrainingConsentRejectKind
import com.anpfuel.domain.repository.TrainingConsentStatus
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.sync.withLock

/** Consent decisions remain bound to the prepared account; the transport additionally binds key and origin. */
data class TrainingConsentCaller(val accountId: String, val scope: com.anpfuel.domain.model.ContributionScope? = null)

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

/** OFF commits before IO; no background grant and no reattribution of retained decisions. */
class TrainingConsentUseCase(
    private val local: TrainingConsentLocalStore,
    private val gateway: TrainingConsentGateway,
    private val network: NetworkConnectivityGateway,
    private val scopes: com.anpfuel.application.port.ContributionScopeProvider,
    private val nowMillis: () -> Long = System::currentTimeMillis,
    private val newCommandId: () -> String = { java.util.UUID.randomUUID().toString() },
) {
    private val transport = kotlinx.coroutines.sync.Mutex()

    suspend fun status(caller: TrainingConsentCaller?): TrainingConsentOutcome = owned(caller) { scope ->
        if (local.load(scope).pending?.grant == false) return@owned TrainingConsentOutcome.RevocationPending
        if (!online()) return@owned TrainingConsentOutcome.Offline
        transport.withLock {
            checkScope(scope)
            val status = gateway.consentStatus(scope)
            checkScope(scope)
            TrainingConsentOutcome.Status(status, false)
        }
    }

    suspend fun grant(caller: TrainingConsentCaller?, termsVersion: String, termsDigest: String): TrainingConsentOutcome = owned(caller) { scope ->
        require(termsVersion.isNotBlank() && termsDigest.isNotBlank())
        if (local.load(scope).pending?.grant == false) return@owned TrainingConsentOutcome.RevocationPending
        if (!online()) return@owned TrainingConsentOutcome.Offline
        transport.withLock {
            checkScope(scope)
            if (local.load(scope).retryAtMillis > nowMillis()) return@withLock TrainingConsentOutcome.Rejected(TrainingConsentRejectKind.TRANSPORT, "privacy retry pending")
            val server = gateway.consentStatus(scope)
            checkScope(scope)
            val command = local.update(scope) { it.grant(newCommandId(), termsVersion, termsDigest, server.revision) }.pending!!
            try {
                checkScope(scope)
                val receipt = gateway.grantConsent(scope, command.termsVersion, command.termsDigest, command.revision!!, command.id)
                checkScope(scope)
                val state = local.update(scope) { it.acknowledge(command, receipt) }
                if (state.pending != null || !state.granted) TrainingConsentOutcome.RevocationPending
                else TrainingConsentOutcome.Granted(receipt)
            } catch (error: TrainingConsentException) {
                local.update(scope) { it.failed(command, nowMillis(), error.retryAfterSeconds) }
                throw error
            }
        }
    }

    suspend fun revoke(caller: TrainingConsentCaller?): TrainingConsentOutcome = owned(caller) { scope ->
        local.update(scope) { it.revoke(newCommandId()) }
        sendRevocation(scope)
    }

    suspend fun syncPending(caller: TrainingConsentCaller?): TrainingConsentOutcome = owned(caller) { scope ->
        if (local.load(scope).pending?.grant == false) sendRevocation(scope)
        else {
            // A pending ON is never resumed automatically after process death.
            if (!online()) TrainingConsentOutcome.Offline
            else transport.withLock {
                checkScope(scope)
                val status = gateway.consentStatus(scope)
                checkScope(scope)
                TrainingConsentOutcome.Status(status, false)
            }
        }
    }

    private suspend fun sendRevocation(scope: com.anpfuel.domain.model.ContributionScope): TrainingConsentOutcome {
        if (!online()) return TrainingConsentOutcome.RevocationPending
        return transport.withLock {
            var command = local.load(scope).pending ?: return@withLock TrainingConsentOutcome.RevocationPending
            if (command.grant || local.load(scope).retryAtMillis > nowMillis()) return@withLock TrainingConsentOutcome.RevocationPending
            try {
                checkScope(scope)
                if (command.revision == null) {
                    val server = gateway.consentStatus(scope)
                    checkScope(scope)
                    command = local.update(scope) { it.bindRevocation(command.id, server.revision) }.pending!!
                }
                val receipt = try {
                    checkScope(scope)
                    gateway.revokeConsent(scope, command.revision!!, command.id)
                } catch (error: TrainingConsentException) {
                    if (error.kind != TrainingConsentRejectKind.STALE_REVISION && error.kind != TrainingConsentRejectKind.CONFLICT) throw error
                    // A confirmed 409 performed no mutation. OFF alone may rebind once.
                    checkScope(scope)
                    val server = gateway.consentStatus(scope)
                    checkScope(scope)
                    command = local.update(scope) { it.bindRevocation(command.id, server.revision, confirmedStale = true) }.pending!!
                    checkScope(scope)
                    gateway.revokeConsent(scope, command.revision!!, command.id)
                }
                checkScope(scope)
                val state = local.update(scope) { it.acknowledge(command, receipt) }
                if (state.pending != null) TrainingConsentOutcome.RevocationPending else TrainingConsentOutcome.Revoked(receipt)
            } catch (error: TrainingConsentException) {
                local.update(scope) { it.failed(command, nowMillis(), error.retryAfterSeconds) }
                TrainingConsentOutcome.RevocationPending
            }
        }
    }

    private suspend fun online(): Boolean = network.observeIsConnected().first()

    private suspend fun checkScope(expected: com.anpfuel.domain.model.ContributionScope) {
        if (scopes.currentScope() != expected) throw TrainingConsentException(TrainingConsentRejectKind.AUTH_REQUIRED, "privacy owner changed")
    }

    private suspend fun owned(caller: TrainingConsentCaller?, block: suspend (com.anpfuel.domain.model.ContributionScope) -> TrainingConsentOutcome): TrainingConsentOutcome {
        if (caller == null || caller.accountId.isBlank()) return TrainingConsentOutcome.Guest
        return try {
            val scope = scopes.currentScope()
            if (!scope.ownerScope.startsWith("account:${caller.accountId}:") || (caller.scope != null && caller.scope != scope)) {
                throw TrainingConsentException(TrainingConsentRejectKind.AUTH_REQUIRED, "privacy owner unavailable")
            }
            block(scope)
        } catch (error: kotlinx.coroutines.CancellationException) {
            throw error
        } catch (error: TrainingConsentException) {
            TrainingConsentOutcome.Rejected(error.kind, error.message.orEmpty())
        } catch (_: Exception) {
            TrainingConsentOutcome.Rejected(TrainingConsentRejectKind.TRANSPORT, "privacy operation unavailable")
        }
    }
}
