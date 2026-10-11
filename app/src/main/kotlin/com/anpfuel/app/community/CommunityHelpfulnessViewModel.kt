package com.anpfuel.app.community

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.port.CommunityHelpfulnessFlagProvider
import com.anpfuel.application.usecase.community.HelpfulnessVoteOutcome
import com.anpfuel.application.usecase.community.SubmitHelpfulnessVoteUseCase
import com.anpfuel.domain.repository.CommunityHelpfulnessRejectKind
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * CS08B helpful/unhelpful state (B-BR-CS01, BUC-CS02).
 *
 * Flag-gated: [UiState.Disabled] performs no network IO and renders
 * nothing (rollback is flag OFF). Guests see [UiState.Guest] with the
 * free-login explanation and no network IO. [prepare] fixes the target
 * (observation + immutable sequence) and mints one stable client
 * submission id; [vote] and [remove] reuse that id so retries converge
 * instead of amplifying the vote. A second tap while
 * [UiState.Submitting] is ignored (single flight). Counts render from
 * exact tallies with the owner's choice and the frozen review notice;
 * terminal rejection carries a fixed kind label and a sanitized
 * message. Reporting reuses the existing dispute flow through the
 * host-provided callback — this ViewModel never reports by itself.
 */
sealed interface CommunityHelpfulnessUiState {
    data object Disabled : CommunityHelpfulnessUiState
    data object Guest : CommunityHelpfulnessUiState
    data object Idle : CommunityHelpfulnessUiState
    data object Loading : CommunityHelpfulnessUiState
    data class Content(
        val observationId: String,
        val decisionSequence: Long,
        val submissionId: String,
        val helpful: Long,
        val notHelpful: Long,
        val total: Long,
        val helpfulBasisPoints: Long?,
        val myChoice: String?,
        val underReview: Boolean,
    ) : CommunityHelpfulnessUiState
    data object Submitting : CommunityHelpfulnessUiState
    data class Rejected(
        val kind: CommunityHelpfulnessRejectKind,
        val message: String,
    ) : CommunityHelpfulnessUiState
}

@HiltViewModel
class CommunityHelpfulnessViewModel @Inject constructor(
    private val useCase: SubmitHelpfulnessVoteUseCase,
    private val flagProvider: CommunityHelpfulnessFlagProvider,
) : ViewModel() {

    /** Test seam for the IO context (unit tests inject the test dispatcher). */
    internal var ioDispatcher: kotlinx.coroutines.CoroutineDispatcher =
        kotlinx.coroutines.Dispatchers.IO

    private data class Target(
        val observationId: String,
        val decisionSequence: Long,
        val clientSubmissionId: String,
    )

    private val _state =
        MutableStateFlow<CommunityHelpfulnessUiState>(initialState())
    val state: StateFlow<CommunityHelpfulnessUiState> = _state.asStateFlow()

    private var target: Target? = null
    private var submitting = false

    private fun initialState(): CommunityHelpfulnessUiState =
        if (flagProvider.isEnabled()) {
            CommunityHelpfulnessUiState.Idle
        } else {
            CommunityHelpfulnessUiState.Disabled
        }

    fun prepare(observationId: String, decisionSequence: Long, accountId: String) {
        if (!flagProvider.isEnabled()) {
            _state.value = CommunityHelpfulnessUiState.Disabled
            return
        }
        if (accountId.isBlank()) {
            _state.value = CommunityHelpfulnessUiState.Guest
            return
        }
        target = Target(observationId, decisionSequence, UUID.randomUUID().toString())
        refresh()
    }

    fun refresh() {
        val current = target ?: return
        if (_state.value is CommunityHelpfulnessUiState.Submitting) return
        _state.value = CommunityHelpfulnessUiState.Loading
        viewModelScope.launch(ioDispatcher) {
            when (
                val outcome =
                    useCase.tally(
                        SubmitHelpfulnessVoteUseCase.TallyRequest(
                            current.observationId,
                            current.decisionSequence,
                        ),
                    )
            ) {
                is HelpfulnessVoteOutcome.Disabled ->
                    _state.value = CommunityHelpfulnessUiState.Disabled
                is HelpfulnessVoteOutcome.Tallied ->
                    _state.value = contentOf(current, outcome.tally.helpful, outcome.tally.notHelpful,
                        outcome.tally.total, outcome.tally.helpfulBasisPoints,
                        outcome.tally.myChoice, outcome.tally.underReview)
                is HelpfulnessVoteOutcome.Rejected ->
                    _state.value = CommunityHelpfulnessUiState.Rejected(outcome.kind, outcome.message)
                else ->
                    _state.value = CommunityHelpfulnessUiState.Rejected(
                        CommunityHelpfulnessRejectKind.TRANSPORT,
                        "helpfulness read failed",
                    )
            }
        }
    }

    fun vote(choiceWire: String) {
        val current = target ?: return
        if (_state.value is CommunityHelpfulnessUiState.Submitting) return
        if (submitting) return
        submitting = true
        _state.value = CommunityHelpfulnessUiState.Submitting
        viewModelScope.launch(ioDispatcher) {
            try {
                when (
                    val outcome =
                        useCase.cast(
                            SubmitHelpfulnessVoteUseCase.CastRequest(
                                current.observationId,
                                current.clientSubmissionId,
                                choiceWire,
                                current.decisionSequence,
                            ),
                        )
                ) {
                    is HelpfulnessVoteOutcome.Disabled ->
                        _state.value = CommunityHelpfulnessUiState.Disabled
                    is HelpfulnessVoteOutcome.Voted ->
                        _state.value = contentOf(current, outcome.receipt.helpful, outcome.receipt.notHelpful,
                            outcome.receipt.total, outcome.receipt.helpfulBasisPoints,
                            outcome.receipt.myChoice, underReview = false)
                    is HelpfulnessVoteOutcome.Rejected ->
                        _state.value = CommunityHelpfulnessUiState.Rejected(outcome.kind, outcome.message)
                    else ->
                        _state.value = CommunityHelpfulnessUiState.Rejected(
                            CommunityHelpfulnessRejectKind.TRANSPORT,
                            "helpfulness vote failed",
                        )
                }
            } finally {
                submitting = false
            }
        }
    }

    fun remove() {
        val current = target ?: return
        if (_state.value is CommunityHelpfulnessUiState.Submitting) return
        if (submitting) return
        submitting = true
        _state.value = CommunityHelpfulnessUiState.Submitting
        viewModelScope.launch(ioDispatcher) {
            try {
                when (
                    val outcome =
                        useCase.remove(
                            SubmitHelpfulnessVoteUseCase.RemoveRequest(
                                current.observationId,
                                current.decisionSequence,
                            ),
                        )
                ) {
                    is HelpfulnessVoteOutcome.Disabled ->
                        _state.value = CommunityHelpfulnessUiState.Disabled
                    is HelpfulnessVoteOutcome.Removed ->
                        _state.value = contentOf(current, outcome.tally.helpful, outcome.tally.notHelpful,
                            outcome.tally.total, outcome.tally.helpfulBasisPoints,
                            outcome.tally.myChoice, outcome.tally.underReview)
                    is HelpfulnessVoteOutcome.Rejected ->
                        _state.value = CommunityHelpfulnessUiState.Rejected(outcome.kind, outcome.message)
                    else ->
                        _state.value = CommunityHelpfulnessUiState.Rejected(
                            CommunityHelpfulnessRejectKind.TRANSPORT,
                            "helpfulness removal failed",
                        )
                }
            } finally {
                submitting = false
            }
        }
    }

    fun retry() {
        refresh()
    }

    private fun contentOf(
        current: Target,
        helpful: Long,
        notHelpful: Long,
        total: Long,
        basisPoints: Long?,
        myChoice: String?,
        underReview: Boolean,
    ) = CommunityHelpfulnessUiState.Content(
        observationId = current.observationId,
        decisionSequence = current.decisionSequence,
        submissionId = current.clientSubmissionId,
        helpful = helpful,
        notHelpful = notHelpful,
        total = total,
        helpfulBasisPoints = basisPoints,
        myChoice = myChoice,
        underReview = underReview,
    )
}
