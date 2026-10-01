package com.anpfuel.app.community

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.port.CommunityVoteFlagProvider
import com.anpfuel.application.usecase.community.CommunityVoteOutcome
import com.anpfuel.application.usecase.community.SubmitCommunityVoteUseCase
import com.anpfuel.domain.repository.CommunityVoteRejectKind
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * P10-T07 confirm/dispute state (BUC-005, B-BR-005/006/011).
 *
 * Flag-gated: [UiState.Disabled] performs no network IO and renders
 * nothing (rollback is flag OFF). [prepare] fixes the shown snapshot
 * (amount/product/unit/full condition) and mints one stable client
 * submission id; [confirm] and [dispute] reuse that id so retries
 * converge instead of amplifying the vote. A second tap while
 * [UiState.Submitting] is ignored (single flight). [retry] repeats the
 * last action with the same id. Terminal rejection carries a fixed
 * kind label and a sanitized message — private dispute detail is never
 * surfaced. A changed price is disputed with its replacement id while
 * the new observation itself travels via the contribution outbox.
 */
sealed interface CommunityVoteUiState {
    data object Disabled : CommunityVoteUiState
    data object Idle : CommunityVoteUiState
    data class Ready(
        val summary: String,
        val observationId: String,
    ) : CommunityVoteUiState
    data object Submitting : CommunityVoteUiState
    data class Confirmed(
        val voteId: String,
        val replayed: Boolean,
    ) : CommunityVoteUiState
    data class Disputed(
        val voteId: String,
        val state: String,
    ) : CommunityVoteUiState
    data class Rejected(
        val kindLabel: String,
        val message: String,
    ) : CommunityVoteUiState
}

@HiltViewModel
class CommunityVoteViewModel @Inject constructor(
    private val useCase: SubmitCommunityVoteUseCase,
    private val flagProvider: CommunityVoteFlagProvider,
) : ViewModel() {

    private sealed interface LastAction {
        data object Confirm : LastAction
        data class Dispute(
            val reasonWire: String,
            val detail: String?,
            val replacementObservationId: String?,
        ) : LastAction
    }

    private data class PendingTarget(
        val observationId: String,
        val clientSubmissionId: String,
        val shownConditionKind: String,
        val shownUnit: String,
    )

    private val _state =
        MutableStateFlow<CommunityVoteUiState>(initialState())
    val state: StateFlow<CommunityVoteUiState> = _state.asStateFlow()

    private var pending: PendingTarget? = null
    private var lastAction: LastAction? = null

    fun prepare(
        observationId: String,
        productWire: String,
        amountMilliBrl: Long,
        unit: String,
        conditionKind: String,
        qualifierId: String? = null,
    ) {
        if (!flagProvider.isEnabled()) {
            pending = null
            lastAction = null
            _state.value = CommunityVoteUiState.Disabled
            return
        }
        val conditionLabel = CommunityPriceDisplay.formatCondition(conditionKind, qualifierId)
        val summary = CommunityVoteDisplay.confirmSummary(
            productWire = productWire,
            amountMilliBrl = amountMilliBrl,
            unit = unit,
            conditionLabel = conditionLabel,
        )
        pending = PendingTarget(
            observationId = observationId,
            clientSubmissionId = UUID.randomUUID().toString(),
            shownConditionKind = conditionKind,
            shownUnit = unit,
        )
        lastAction = null
        _state.value = CommunityVoteUiState.Ready(summary, observationId)
    }

    fun confirm() {
        val target = pending ?: return
        if (_state.value is CommunityVoteUiState.Submitting) return
        lastAction = LastAction.Confirm
        _state.value = CommunityVoteUiState.Submitting
        viewModelScope.launch {
            val outcome = try {
                useCase.confirm(
                    SubmitCommunityVoteUseCase.ConfirmRequest(
                        observationId = target.observationId,
                        clientSubmissionId = target.clientSubmissionId,
                        shownConditionKind = target.shownConditionKind,
                        shownUnit = target.shownUnit,
                    ),
                )
            } catch (error: Exception) {
                _state.value = CommunityVoteUiState.Rejected(
                    kindLabel = CommunityVoteDisplay.rejectKindLabel(
                        CommunityVoteRejectKind.TRANSPORT,
                    ),
                    message = error.message ?: error.javaClass.simpleName,
                )
                return@launch
            }
            _state.value = when (outcome) {
                is CommunityVoteOutcome.Disabled -> CommunityVoteUiState.Disabled
                is CommunityVoteOutcome.Confirmed ->
                    CommunityVoteUiState.Confirmed(
                        voteId = outcome.receipt.voteId,
                        replayed = outcome.replayed,
                    )
                is CommunityVoteOutcome.Disputed ->
                    CommunityVoteUiState.Disputed(
                        voteId = outcome.receipt.voteId,
                        state = outcome.state,
                    )
                is CommunityVoteOutcome.Rejected ->
                    CommunityVoteUiState.Rejected(
                        kindLabel = CommunityVoteDisplay.rejectKindLabel(outcome.kind),
                        message = outcome.message,
                    )
            }
        }
    }

    fun dispute(reasonWire: String, detail: String?, replacementObservationId: String?) {
        val target = pending ?: return
        if (_state.value is CommunityVoteUiState.Submitting) return
        lastAction = LastAction.Dispute(reasonWire, detail, replacementObservationId)
        _state.value = CommunityVoteUiState.Submitting
        viewModelScope.launch {
            val outcome = try {
                useCase.dispute(
                    SubmitCommunityVoteUseCase.DisputeRequest(
                        targetObservationId = target.observationId,
                        clientSubmissionId = target.clientSubmissionId,
                        reasonWire = reasonWire,
                        detail = detail,
                        replacementObservationId = replacementObservationId,
                        shownConditionKind = target.shownConditionKind,
                    ),
                )
            } catch (error: Exception) {
                _state.value = CommunityVoteUiState.Rejected(
                    kindLabel = CommunityVoteDisplay.rejectKindLabel(
                        CommunityVoteRejectKind.TRANSPORT,
                    ),
                    message = error.message ?: error.javaClass.simpleName,
                )
                return@launch
            }
            _state.value = when (outcome) {
                is CommunityVoteOutcome.Disabled -> CommunityVoteUiState.Disabled
                is CommunityVoteOutcome.Confirmed ->
                    CommunityVoteUiState.Confirmed(
                        voteId = outcome.receipt.voteId,
                        replayed = outcome.replayed,
                    )
                is CommunityVoteOutcome.Disputed ->
                    CommunityVoteUiState.Disputed(
                        voteId = outcome.receipt.voteId,
                        state = outcome.state,
                    )
                is CommunityVoteOutcome.Rejected ->
                    CommunityVoteUiState.Rejected(
                        kindLabel = CommunityVoteDisplay.rejectKindLabel(outcome.kind),
                        message = outcome.message,
                    )
            }
        }
    }

    /** Repeats the last action with the same stable submission id. */
    fun retry() {
        when (val action = lastAction) {
            null -> return
            is LastAction.Confirm -> confirm()
            is LastAction.Dispute -> dispute(
                action.reasonWire,
                action.detail,
                action.replacementObservationId,
            )
        }
    }

    private fun initialState(): CommunityVoteUiState =
        if (!flagProvider.isEnabled()) CommunityVoteUiState.Disabled
        else CommunityVoteUiState.Idle
}
