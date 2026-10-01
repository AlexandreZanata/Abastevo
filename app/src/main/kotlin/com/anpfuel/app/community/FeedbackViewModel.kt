package com.anpfuel.app.community

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.port.FeedbackFlagProvider
import com.anpfuel.application.usecase.feedback.FeedbackStatsOutcome
import com.anpfuel.application.usecase.feedback.FeedbackWriteOutcome
import com.anpfuel.application.usecase.feedback.FeedbackPageOutcome
import com.anpfuel.application.usecase.feedback.GetFeedbackPageUseCase
import com.anpfuel.application.usecase.feedback.SubmitFeedbackUseCase
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.portable.PortableFeedback
import com.anpfuel.domain.portable.PortableText
import com.anpfuel.domain.repository.FeedbackCommentView
import com.anpfuel.domain.repository.FeedbackException
import com.anpfuel.domain.repository.FeedbackRejectKind
import com.anpfuel.domain.repository.PendingFeedbackOp
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * P17-T02 social-flow state (B-BR-F01…F08, BUC-F01…F05); P22-T02
 * wires the 1–5 personal stars through the published ratings
 * transport plus the public aggregate read.
 *
 * Flag-gated: [FeedbackUiState.Disabled] performs no network IO and
 * renders nothing (rollback is flag OFF). [prepare] fixes the target
 * plus the caller-held account id; a blank account surfaces
 * [FeedbackUiState.SignInRequired] without IO for writes — anonymous
 * browsing stays available through [loadFirstPage] and [loadStats],
 * which need no session. Over-280 text and out-of-range stars are
 * rejected locally ([INVALID], no IO). Terminal rejections carry
 * fixed kind labels for optimistic rollback; outages queue with one
 * stable op id and [retry] reuses it, so retries never amplify the
 * write. Rating aggregates travel as exact count/sum; the mean
 * renders through [FeedbackDisplay.ratingLine], kept separate from
 * price confidence (B-BR-C04).
 */
sealed interface FeedbackUiState {
    data object Disabled : FeedbackUiState
    data object Idle : FeedbackUiState
    data object SignInRequired : FeedbackUiState
    data class Loaded(
        val items: List<FeedbackCommentView>,
        val nextCursor: String?,
        val stale: Boolean,
    ) : FeedbackUiState
    data object Submitting : FeedbackUiState
    data class Saved(val commentId: String, val revision: Int) : FeedbackUiState
    data class Rated(val created: Boolean, val count: Long, val sum: Long) : FeedbackUiState
    data object RatingDeleted : FeedbackUiState
    data class StatsLoaded(val count: Long, val sum: Long) : FeedbackUiState
    data class Voted(val valid: Long, val invalid: Long) : FeedbackUiState
    data object VoteRemoved : FeedbackUiState
    data object Reported : FeedbackUiState
    data object CommentDeleted : FeedbackUiState
    data class Queued(val op: PendingFeedbackOp) : FeedbackUiState
    data class Rejected(val kindLabel: String, val message: String) : FeedbackUiState
}

@HiltViewModel
class FeedbackViewModel @Inject constructor(
    private val writes: SubmitFeedbackUseCase,
    private val reads: GetFeedbackPageUseCase,
    private val flagProvider: FeedbackFlagProvider,
) : ViewModel() {

    private sealed interface LastWrite {
        data class Rating(val stars: Int, val opId: String) : LastWrite
        data class DeleteRating(val opId: String) : LastWrite
        data class Comment(val text: String, val opId: String) : LastWrite
        data class Reply(val parentId: String, val text: String, val opId: String) : LastWrite
        data class Edit(val commentId: String, val text: String, val revision: Int, val opId: String) : LastWrite
        data class Vote(val commentId: String, val choice: String, val opId: String) : LastWrite
        data class RemoveVote(val commentId: String, val opId: String) : LastWrite
        data class Report(val commentId: String, val reason: String, val opId: String) : LastWrite
    }

    private data class Target(
        val stationId: String,
        val product: String,
        val accountId: String,
    )

    private val _state =
        MutableStateFlow<FeedbackUiState>(initialState())
    val state: StateFlow<FeedbackUiState> = _state.asStateFlow()

    private var target: Target? = null
    private var cursor: String = ""
    private var lastWrite: LastWrite? = null

    fun prepare(stationId: String, product: String, accountId: String) {
        if (!flagProvider.isEnabled()) {
            target = null
            lastWrite = null
            _state.value = FeedbackUiState.Disabled
            return
        }
        target = Target(stationId, product, accountId)
        cursor = ""
        lastWrite = null
        _state.value = FeedbackUiState.Idle
    }

    fun loadFirstPage() {
        val current = target ?: return
        if (!flagProvider.isEnabled()) {
            _state.value = FeedbackUiState.Disabled
            return
        }
        if (_state.value is FeedbackUiState.Submitting) return
        _state.value = FeedbackUiState.Submitting
        viewModelScope.launch {
            _state.value = when (val outcome = runReads { reads.comments(current.stationId, current.product, "", FIRST_PAGE_LIMIT) }) {
                is FeedbackPageOutcome.Disabled -> FeedbackUiState.Disabled
                is FeedbackPageOutcome.Fresh -> loaded(outcome.page.items, outcome.page.nextCursor, stale = false)
                is FeedbackPageOutcome.StaleCache -> loaded(outcome.page.items, outcome.page.nextCursor, stale = true)
                is FeedbackPageOutcome.Unavailable ->
                    FeedbackUiState.Rejected(FeedbackDisplay.rejectKindLabel(FeedbackRejectKind.TRANSPORT), "feedback unavailable")
            }
        }
    }

    fun loadStats() {
        val current = target ?: return
        if (!flagProvider.isEnabled()) {
            _state.value = FeedbackUiState.Disabled
            return
        }
        if (_state.value is FeedbackUiState.Submitting) return
        _state.value = FeedbackUiState.Submitting
        viewModelScope.launch {
            _state.value = when (val outcome = runReads { reads.stats(current.stationId, current.product) }) {
                is FeedbackStatsOutcome.Disabled -> FeedbackUiState.Disabled
                is FeedbackStatsOutcome.Fresh ->
                    FeedbackUiState.StatsLoaded(outcome.stats.count, outcome.stats.sum)
                is FeedbackStatsOutcome.Unavailable ->
                    FeedbackUiState.Rejected(FeedbackDisplay.rejectKindLabel(FeedbackRejectKind.TRANSPORT), "ratings unavailable")
            }
        }
    }

    fun rate(stars: Int) {
        val current = target ?: return
        if (guardCommon(current.accountId)) return
        if (!PortableFeedback.isValidRating(stars)) {
            _state.value = FeedbackUiState.Rejected("INVALID", "rating outside 1-5")
            return
        }
        val op = LastWrite.Rating(stars, UUID.randomUUID().toString())
        lastWrite = op
        submitting()
        viewModelScope.launch {
            _state.value = writeState(
                runWrites { writes.rate(current.accountId, current.stationId, current.product, stars, op.opId) },
            )
        }
    }

    fun deleteRating() {
        val current = target ?: return
        if (guardCommon(current.accountId)) return
        val op = LastWrite.DeleteRating(UUID.randomUUID().toString())
        lastWrite = op
        submitting()
        viewModelScope.launch {
            _state.value = writeState(
                runWrites { writes.deleteRating(current.accountId, current.stationId, current.product, op.opId) },
            )
        }
    }

    fun submitComment(text: String) {
        val current = target ?: return
        if (guardCommon(current.accountId)) return
        if (guardText(text)) return
        val op = LastWrite.Comment(text, UUID.randomUUID().toString())
        lastWrite = op
        submitting()
        viewModelScope.launch {
            _state.value = writeState(
                runWrites { writes.comment(current.accountId, current.stationId, current.product, text, op.opId) },
            )
        }
    }

    fun reply(parentId: String, text: String) {
        val current = target ?: return
        if (guardCommon(current.accountId)) return
        if (parentId.isBlank() || guardText(text)) {
            if (parentId.isBlank()) {
                _state.value = FeedbackUiState.Rejected("INVALID", "reply target is blank")
            }
            return
        }
        val op = LastWrite.Reply(parentId, text, UUID.randomUUID().toString())
        lastWrite = op
        submitting()
        viewModelScope.launch {
            _state.value = writeState(
                runWrites {
                    writes.reply(current.accountId, current.stationId, current.product, parentId, text, op.opId)
                },
            )
        }
    }

    fun editComment(commentId: String, text: String, expectedRevision: Int) {
        val current = target ?: return
        if (guardCommon(current.accountId)) return
        if (commentId.isBlank() || expectedRevision <= 0 || guardText(text)) {
            if (commentId.isBlank() || expectedRevision <= 0) {
                _state.value = FeedbackUiState.Rejected("INVALID", "comment revision is invalid")
            }
            return
        }
        val op = LastWrite.Edit(commentId, text, expectedRevision, UUID.randomUUID().toString())
        lastWrite = op
        submitting()
        viewModelScope.launch {
            _state.value = writeState(
                runWrites { writes.edit(current.accountId, commentId, text, expectedRevision, op.opId) },
            )
        }
    }

    fun submitVote(commentId: String, choice: String) {
        val current = target ?: return
        if (guardCommon(current.accountId)) return
        if (commentId.isBlank() || (choice != SubmitFeedbackUseCase.VOTE_VALID && choice != SubmitFeedbackUseCase.VOTE_INVALID)) {
            _state.value = FeedbackUiState.Rejected("INVALID", "vote is invalid")
            return
        }
        val op = LastWrite.Vote(commentId, choice, UUID.randomUUID().toString())
        lastWrite = op
        submitting()
        viewModelScope.launch {
            _state.value = writeState(
                runWrites { writes.vote(current.accountId, commentId, choice, op.opId) },
            )
        }
    }

    fun removeVote(commentId: String) {
        val current = target ?: return
        if (guardCommon(current.accountId)) return
        if (commentId.isBlank()) {
            _state.value = FeedbackUiState.Rejected("INVALID", "comment is blank")
            return
        }
        val op = LastWrite.RemoveVote(commentId, UUID.randomUUID().toString())
        lastWrite = op
        submitting()
        viewModelScope.launch {
            _state.value = writeState(
                runWrites { writes.removeVote(current.accountId, commentId, op.opId) },
            )
        }
    }

    fun report(commentId: String, reason: String) {
        val current = target ?: return
        if (guardCommon(current.accountId)) return
        if (commentId.isBlank() || reason.trim().isEmpty()) {
            _state.value = FeedbackUiState.Rejected("INVALID", "report is invalid")
            return
        }
        val op = LastWrite.Report(commentId, reason, UUID.randomUUID().toString())
        lastWrite = op
        submitting()
        viewModelScope.launch {
            _state.value = writeState(
                runWrites { writes.report(current.accountId, commentId, reason, op.opId) },
            )
        }
    }

    /** Repeats the last write with the same stable op id. */
    fun retry() {
        val current = target ?: return
        when (val action = lastWrite) {
            null -> return
            is LastWrite.Rating -> submitWithId(action) { id ->
                writes.rate(current.accountId, current.stationId, current.product, action.stars, id)
            }
            is LastWrite.DeleteRating -> submitWithId(action) { id ->
                writes.deleteRating(current.accountId, current.stationId, current.product, id)
            }
            is LastWrite.Comment -> submitWithId(action) { id ->
                writes.comment(current.accountId, current.stationId, current.product, action.text, id)
            }
            is LastWrite.Reply -> submitWithId(action) { id ->
                writes.reply(current.accountId, current.stationId, current.product, action.parentId, action.text, id)
            }
            is LastWrite.Edit -> submitWithId(action) { id ->
                writes.edit(current.accountId, action.commentId, action.text, action.revision, id)
            }
            is LastWrite.Vote -> submitWithId(action) { id ->
                writes.vote(current.accountId, action.commentId, action.choice, id)
            }
            is LastWrite.RemoveVote -> submitWithId(action) { id ->
                writes.removeVote(current.accountId, action.commentId, id)
            }
            is LastWrite.Report -> submitWithId(action) { id ->
                writes.report(current.accountId, action.commentId, action.reason, id)
            }
        }
    }

    private fun submitWithId(
        action: LastWrite,
        call: suspend (String) -> FeedbackWriteOutcome,
    ) {
        if (_state.value is FeedbackUiState.Submitting) return
        val opId = when (action) {
            is LastWrite.Rating -> action.opId
            is LastWrite.DeleteRating -> action.opId
            is LastWrite.Comment -> action.opId
            is LastWrite.Reply -> action.opId
            is LastWrite.Edit -> action.opId
            is LastWrite.Vote -> action.opId
            is LastWrite.RemoveVote -> action.opId
            is LastWrite.Report -> action.opId
        }
        _state.value = FeedbackUiState.Submitting
        viewModelScope.launch {
            _state.value = writeState(runWrites { call(opId) })
        }
    }

    /** Flag + session guard shared by every write; true when handled. */
    private fun guardCommon(accountId: String): Boolean {
        if (!flagProvider.isEnabled()) {
            _state.value = FeedbackUiState.Disabled
            return true
        }
        if (_state.value is FeedbackUiState.Submitting) return true
        if (accountId.isBlank()) {
            _state.value = FeedbackUiState.SignInRequired
            return true
        }
        return false
    }

    /** 280-scalar guard; true when rejected locally without IO. */
    private fun guardText(text: String): Boolean {
        if (FeedbackDisplay.charsRemaining(text) < 0 || PortableText.normalize(text).isEmpty()) {
            _state.value = FeedbackUiState.Rejected("INVALID", "comment text exceeds 280 characters")
            return true
        }
        return false
    }

    private fun submitting() {
        _state.value = FeedbackUiState.Submitting
    }

    private fun loaded(items: List<FeedbackCommentView>, nextCursor: String?, stale: Boolean): FeedbackUiState {
        cursor = nextCursor.orEmpty()
        return FeedbackUiState.Loaded(items, nextCursor, stale)
    }

    private suspend fun runWrites(call: suspend () -> FeedbackWriteOutcome): FeedbackWriteOutcome {
        return try {
            call()
        } catch (invalid: DomainException) {
            FeedbackWriteOutcome.Rejected(FeedbackRejectKind.TARGET_INVALID, invalid.message ?: "invalid")
        } catch (refused: FeedbackException) {
            FeedbackWriteOutcome.Rejected(refused.kind, refused.message ?: refused.kind.name)
        } catch (error: Exception) {
            FeedbackWriteOutcome.Rejected(FeedbackRejectKind.TRANSPORT, error.message ?: "transport")
        }
    }

    private suspend fun <T> runReads(call: suspend () -> T): T = call()

    private fun writeState(outcome: FeedbackWriteOutcome): FeedbackUiState {
        return when (outcome) {
            is FeedbackWriteOutcome.Disabled -> FeedbackUiState.Disabled
            is FeedbackWriteOutcome.LoginRequired -> FeedbackUiState.SignInRequired
            is FeedbackWriteOutcome.Rated ->
                FeedbackUiState.Rated(outcome.receipt.created, outcome.stats.count, outcome.stats.sum)
            is FeedbackWriteOutcome.RatingDeleted -> FeedbackUiState.RatingDeleted
            is FeedbackWriteOutcome.Commented ->
                FeedbackUiState.Saved(outcome.receipt.commentId, outcome.receipt.revision)
            is FeedbackWriteOutcome.Replied ->
                FeedbackUiState.Saved(outcome.receipt.commentId, outcome.receipt.revision)
            is FeedbackWriteOutcome.Edited ->
                FeedbackUiState.Saved(outcome.receipt.commentId, outcome.receipt.revision)
            is FeedbackWriteOutcome.CommentDeleted -> FeedbackUiState.CommentDeleted
            is FeedbackWriteOutcome.Voted ->
                FeedbackUiState.Voted(outcome.tally.valid, outcome.tally.invalid)
            is FeedbackWriteOutcome.VoteRemoved -> FeedbackUiState.VoteRemoved
            is FeedbackWriteOutcome.Reported -> FeedbackUiState.Reported
            is FeedbackWriteOutcome.Queued -> FeedbackUiState.Queued(outcome.op)
            is FeedbackWriteOutcome.Rejected ->
                FeedbackUiState.Rejected(
                    FeedbackDisplay.rejectKindLabel(outcome.kind),
                    outcome.message,
                )
        }
    }

    private fun initialState(): FeedbackUiState =
        if (!flagProvider.isEnabled()) FeedbackUiState.Disabled
        else FeedbackUiState.Idle

    companion object {
        const val FIRST_PAGE_LIMIT = 20
    }
}
