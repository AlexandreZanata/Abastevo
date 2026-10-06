package com.anpfuel.app.ui.suggest

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
import com.anpfuel.domain.repository.IntakeSummary
import com.anpfuel.domain.repository.ServerStationCache
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.UUID
import javax.inject.Inject
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
) : ViewModel() {

    private val _uiState = MutableStateFlow(SuggestUiState())
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

    fun newForm() {
        clientSubmissionId = UUID.randomUUID().toString()
        _uiState.update { SuggestUiState(ownItems = it.ownItems) }
    }

    fun submitProposal() {
        val session = sessionStore?.load()
        if (session == null || session.familyId.isBlank() || session.accessToken.isBlank()) {
            _uiState.update { it.copy(submit = SubmitSuggestionOutcome.SessionInvalid) }
            return
        }
        val current = _uiState.value
        viewModelScope.launch {
            _uiState.update { it.copy(isSending = true, submit = null, duplicateHint = null) }
            val hint = findDuplicateHint(current.cnpj)
            val proposal = JSONObject()
                .put("display_name", current.displayName)
                .put("municipality_code", current.municipalityCode)
                .put("state", current.state)
                .put("cnpj_normalized", current.cnpj)
                .toString()
            val outcome = submit.invoke(
                session.familyId, session.accessToken,
                clientSubmissionId, proposal,
            )
            _uiState.update { it.copy(isSending = false, submit = outcome, duplicateHint = hint) }
            if (outcome is SubmitSuggestionOutcome.Submitted) {
                refreshMine(session.familyId, session.accessToken)
            }
        }
    }

    fun refreshMine() {
        val session = sessionStore?.load() ?: return
        viewModelScope.launch {
            refreshMine(session.familyId, session.accessToken)
        }
    }

    fun loadStatus(id: String) {
        val session = sessionStore?.load() ?: return
        viewModelScope.launch {
            _uiState.update {
                it.copy(selectedStatus = status.invoke(session.familyId, session.accessToken, id))
            }
        }
    }

    fun cancelSuggestion(id: String) {
        val session = sessionStore?.load() ?: return
        viewModelScope.launch {
            when (cancel.invoke(session.familyId, session.accessToken, id)) {
                is CancelSuggestionOutcome.Cancelled,
                is CancelSuggestionOutcome.Closed,
                is CancelSuggestionOutcome.NotFound,
                -> {
                    _uiState.update { it.copy(selectedStatus = null) }
                    refreshMine(session.familyId, session.accessToken)
                }
                else -> Unit
            }
        }
    }

    private suspend fun refreshMine(familyId: String, accessToken: String) {
        when (val outcome = mine.invoke(familyId, accessToken)) {
            is OwnedSuggestionsOutcome.Found ->
                _uiState.update { it.copy(ownItems = outcome.items) }
            else -> Unit
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
