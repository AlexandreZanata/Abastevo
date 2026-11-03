package com.anpfuel.app.ui.suggest

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.intake.CancelOwnedSuggestionUseCase
import com.anpfuel.application.usecase.intake.CancelSuggestionOutcome
import com.anpfuel.application.usecase.intake.GetOwnedSuggestionsUseCase
import com.anpfuel.application.usecase.intake.GetSuggestionStatusUseCase
import com.anpfuel.application.usecase.intake.OwnedSuggestionsOutcome
import com.anpfuel.application.usecase.intake.SubmitStationSuggestionUseCase
import com.anpfuel.application.usecase.intake.SubmitSuggestionOutcome
import com.anpfuel.application.usecase.intake.SuggestionStatusOutcome
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.repository.IntakeSummary
import com.anpfuel.domain.repository.ServerStationCache
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import org.json.JSONObject

/**
 * P27-T04 suggest/correct/status journey (free accounts).
 *
 * Search-before-submit warns when the proposal CNPJ already sits in
 * the cached catalog (review still dedups; corrections stay allowed).
 * The stable client submission id survives offline retries so no
 * duplicate visible station ever appears. Guest (no session) renders
 * sign-in gating; revoked sessions surface explicitly. No background
 * permission is demanded and no photo is required.
 */
data class SuggestUiState(
    val displayName: String = "",
    val municipalityCode: String = "",
    val state: String = "",
    val cnpj: String = "",
    val chargingSite: Boolean = false,
    val publicAddress: String = "",
    val duplicateHint: String? = null,
    val submit: SubmitSuggestionOutcome? = null,
    val isSending: Boolean = false,
    val ownItems: List<IntakeSummary> = emptyList(),
    val selectedStatus: SuggestionStatusOutcome? = null,
)

@HiltViewModel
class SuggestStationViewModel @Inject constructor(
    private val submit: SubmitStationSuggestionUseCase,
    private val status: GetSuggestionStatusUseCase,
    private val cancel: CancelOwnedSuggestionUseCase,
    private val mine: GetOwnedSuggestionsUseCase,
    private val sessionStore: AuthSessionStore? = null,
    private val catalogCache: ServerStationCache? = null,
    saved: SavedStateHandle = SavedStateHandle(),
) : ViewModel() {

    private val chargingSite = saved.get<Boolean>("chargingSite") == true
    private val _uiState = MutableStateFlow(SuggestUiState(chargingSite = chargingSite))
    internal var ioDispatcher: CoroutineDispatcher = Dispatchers.IO
    val uiState: StateFlow<SuggestUiState> = _uiState.asStateFlow()

    private var clientSubmissionId: String = UUID.randomUUID().toString()

    fun onField(displayName: String, municipalityCode: String, state: String, cnpj: String) {
        _uiState.update {
            it.copy(
                displayName = displayName,
                municipalityCode = municipalityCode,
                state = state,
                cnpj = cnpj,
                duplicateHint = null,
                submit = null,
            )
        }
    }

    fun onPublicAddress(address: String) {
        _uiState.update { it.copy(publicAddress = address, submit = null) }
    }

    fun newForm() {
        if (_uiState.value.isSending) return
        clientSubmissionId = UUID.randomUUID().toString()
        _uiState.update { SuggestUiState(chargingSite = chargingSite, ownItems = it.ownItems) }
    }

    fun submitProposal() {
        if (_uiState.value.isSending) return
        val session = sessionStore?.load()
        if (session == null || session.familyId.isBlank() || session.accessToken.isBlank()) {
            _uiState.update { it.copy(submit = SubmitSuggestionOutcome.SessionInvalid) }
            return
        }
        val current = _uiState.value
        if (chargingSite && current.displayName.isBlank() && current.publicAddress.isBlank()) {
            _uiState.update { it.copy(submit = SubmitSuggestionOutcome.Invalid) }
            return
        }
        val submissionId = clientSubmissionId
        _uiState.update { it.copy(isSending = true, submit = null, duplicateHint = null) }
        viewModelScope.launch {
            val hint = if (chargingSite) null else withContext(ioDispatcher) { findDuplicateHint(current.cnpj) }
            val proposal = JSONObject()
                .put("display_name", current.displayName)
                .put("municipality_code", current.municipalityCode)
                .put("state", current.state)
                .apply {
                    if (chargingSite) {
                        put("kind", "charging_site")
                        put("public_address", current.publicAddress)
                    } else {
                        put("cnpj_normalized", current.cnpj)
                    }
                }
            val outcome = withContext(ioDispatcher) {
                submit.invoke(session.familyId, session.accessToken, submissionId, proposal.toString())
            }
            if (sessionStore?.load()?.accountId != session.accountId) {
                _uiState.update { it.copy(isSending = false, ownItems = emptyList(), selectedStatus = null) }
                return@launch
            }
            _uiState.update { it.copy(isSending = false, submit = outcome, duplicateHint = hint) }
            if (outcome is SubmitSuggestionOutcome.Submitted) {
                refreshMine(session)
            }
        }
    }

    fun refreshMine() {
        _uiState.update { it.copy(ownItems = emptyList(), selectedStatus = null) }
        val session = sessionStore?.load() ?: return
        viewModelScope.launch {
            refreshMine(session)
        }
    }

    fun loadStatus(id: String) {
        val session = sessionStore?.load() ?: return
        viewModelScope.launch {
            val result = withContext(ioDispatcher) { status.invoke(session.familyId, session.accessToken, id) }
            if (sessionStore?.load()?.accountId == session.accountId) {
                _uiState.update { it.copy(selectedStatus = result) }
            }
        }
    }

    fun cancelSuggestion(id: String) {
        val session = sessionStore?.load() ?: return
        viewModelScope.launch {
            val result = withContext(ioDispatcher) { cancel.invoke(session.familyId, session.accessToken, id) }
            if (sessionStore?.load()?.accountId != session.accountId) return@launch
            when (result) {
                is CancelSuggestionOutcome.Cancelled,
                is CancelSuggestionOutcome.Closed,
                is CancelSuggestionOutcome.NotFound,
                -> {
                    _uiState.update { it.copy(selectedStatus = null) }
                    refreshMine(session)
                }
                else -> Unit
            }
        }
    }

    private suspend fun refreshMine(session: PortableAuth.Session) {
        val outcome = withContext(ioDispatcher) { mine.invoke(session.familyId, session.accessToken) }
        if (sessionStore?.load()?.accountId == session.accountId && outcome is OwnedSuggestionsOutcome.Found) {
            _uiState.update { it.copy(ownItems = outcome.items) }
        }
    }

    private suspend fun findDuplicateHint(cnpj: String): String? {
        val normalized = cnpj.trim()
        if (normalized.isEmpty()) return null
        val page = runCatching { catalogCache?.loadPage() }.getOrNull() ?: return null
        val match = page.items.firstOrNull {
            it.cnpjNormalized.equals(normalized, ignoreCase = true)
        } ?: return null
        return match.displayName
    }
}
