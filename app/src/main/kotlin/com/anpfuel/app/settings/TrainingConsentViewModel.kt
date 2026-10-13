package com.anpfuel.app.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.privacy.TrainingConsentCaller
import com.anpfuel.application.usecase.privacy.TrainingConsentOutcome
import com.anpfuel.application.usecase.privacy.TrainingConsentUseCase
import com.anpfuel.domain.repository.TrainingConsentRejectKind
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * CS11B training-consent settings state (B-BR-CS06, BUC-CS05).
 *
 * Guests see [TrainingConsentUiState.Guest] with the free-login explanation and no
 * network IO. [prepare] syncs any pending offline revocation first
 * and then renders the exact server state; the toggle grants (ON)
 * or revokes (OFF) and reloads the server state so the switch never
 * shows a claim the server does not hold. A second tap while
 * [TrainingConsentUiState.Updating] is ignored (single flight).
 * UI decisions are serialized on Main; IO returns to that dispatcher before
 * generation checks. Account changes/logout and newer refreshes invalidate
 * earlier results, including old update completion/single-flight release.
 * Pending states stay explicit with [retry]; terminal rejection
 * carries a fixed kind label and a sanitized message.
 */
sealed interface TrainingConsentUiState {
    data object Guest : TrainingConsentUiState
    data object Loading : TrainingConsentUiState
    data class Content(
        val granted: Boolean,
        val state: String?,
        val termsVersion: String?,
        val termsDigest: String?,
        val revision: Long,
        val pendingRevocation: Boolean,
        val needsRegrant: Boolean,
    ) : TrainingConsentUiState
    data object Updating : TrainingConsentUiState
    data object Offline : TrainingConsentUiState
    data object Pending : TrainingConsentUiState
    data class Rejected(
        val kind: TrainingConsentRejectKind,
        val message: String,
    ) : TrainingConsentUiState
}

@HiltViewModel
class TrainingConsentViewModel @Inject constructor(
    private val useCase: TrainingConsentUseCase,
) : ViewModel() {

    /** Test seam for the IO context (unit tests inject the test dispatcher). */
    internal var ioDispatcher: kotlinx.coroutines.CoroutineDispatcher =
        kotlinx.coroutines.Dispatchers.IO

    private val _state =
        MutableStateFlow<TrainingConsentUiState>(TrainingConsentUiState.Loading)
    val state: StateFlow<TrainingConsentUiState> = _state.asStateFlow()

    private var caller: TrainingConsentCaller? = null
    private var updating = false
    private var revoking = false
    private var generation = 0L

    fun prepare(accountId: String, scope: com.anpfuel.domain.model.ContributionScope? = null) {
        generation += 1
        updating = false
        revoking = false
        caller = null
        if (accountId.isBlank()) {
            _state.value = TrainingConsentUiState.Guest
            return
        }
        caller = TrainingConsentCaller(accountId, scope)
        _state.value = TrainingConsentUiState.Loading
        refresh()
    }

    fun refresh() {
        val current = caller ?: return
        if (_state.value is TrainingConsentUiState.Updating) return
        val request = ++generation
        _state.value = TrainingConsentUiState.Loading
        viewModelScope.launch {
            val outcome = withContext(ioDispatcher) { useCase.syncPending(current) }
            if (generation != request) return@launch
            _state.value = when (outcome) {
                is TrainingConsentOutcome.Guest ->
                    TrainingConsentUiState.Guest
                is TrainingConsentOutcome.Offline ->
                    TrainingConsentUiState.Offline
                is TrainingConsentOutcome.RevocationPending ->
                    TrainingConsentUiState.Pending
                is TrainingConsentOutcome.Status ->
                    contentOf(outcome.status.granted, outcome.status.state,
                        outcome.status.termsVersion, outcome.status.termsDigest,
                        outcome.status.revision, outcome.pendingRevocation,
                        outcome.status.needsRegrant)
                is TrainingConsentOutcome.Granted ->
                    contentOf(true, outcome.receipt.state, outcome.receipt.termsVersion,
                        null, outcome.receipt.revision, false, false)
                is TrainingConsentOutcome.Revoked ->
                    contentOf(false, outcome.receipt.state, outcome.receipt.termsVersion,
                        null, outcome.receipt.revision, false, false)
                is TrainingConsentOutcome.Rejected ->
                    TrainingConsentUiState.Rejected(outcome.kind, outcome.message)
            }
        }
    }

    fun setEnabled(enabled: Boolean, termsVersion: String, termsDigest: String) {
        val current = caller ?: return
        if (updating && (enabled || revoking)) return
        val request = ++generation
        updating = true
        revoking = !enabled
        _state.value = TrainingConsentUiState.Updating
        viewModelScope.launch {
            try {
                val outcome = withContext(ioDispatcher) {
                    if (enabled) useCase.grant(current, termsVersion, termsDigest)
                    else useCase.revoke(current)
                }
                if (generation != request) return@launch
                val next = when (outcome) {
                    is TrainingConsentOutcome.Guest ->
                        TrainingConsentUiState.Guest
                    is TrainingConsentOutcome.Offline ->
                        TrainingConsentUiState.Offline
                    is TrainingConsentOutcome.RevocationPending ->
                        TrainingConsentUiState.Pending
                    is TrainingConsentOutcome.Granted,
                    is TrainingConsentOutcome.Revoked,
                    is TrainingConsentOutcome.Status,
                    -> withContext(ioDispatcher) { reload(current) }
                    is TrainingConsentOutcome.Rejected ->
                        TrainingConsentUiState.Rejected(outcome.kind, outcome.message)
                }
                if (generation == request) _state.value = next
            } finally {
                if (generation == request) { updating = false; revoking = false }
            }
        }
    }

    fun retry() {
        refresh()
    }

    private suspend fun reload(current: TrainingConsentCaller): TrainingConsentUiState {
        return when (val outcome = useCase.syncPending(current)) {
            is TrainingConsentOutcome.Status ->
                contentOf(outcome.status.granted, outcome.status.state,
                    outcome.status.termsVersion, outcome.status.termsDigest,
                    outcome.status.revision, outcome.pendingRevocation,
                    outcome.status.needsRegrant)
            is TrainingConsentOutcome.Offline ->
                TrainingConsentUiState.Offline
            is TrainingConsentOutcome.RevocationPending ->
                TrainingConsentUiState.Pending
            is TrainingConsentOutcome.Rejected ->
                TrainingConsentUiState.Rejected(outcome.kind, outcome.message)
            else ->
                TrainingConsentUiState.Rejected(
                    TrainingConsentRejectKind.TRANSPORT,
                    "consent update failed",
                )
        }
    }

    private fun contentOf(
        granted: Boolean,
        state: String?,
        termsVersion: String?,
        termsDigest: String?,
        revision: Long,
        pendingRevocation: Boolean,
        needsRegrant: Boolean,
    ) = TrainingConsentUiState.Content(
        granted = granted,
        state = state,
        termsVersion = termsVersion,
        termsDigest = termsDigest,
        revision = revision,
        pendingRevocation = pendingRevocation,
        needsRegrant = needsRegrant,
    )
}
