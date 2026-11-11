package com.anpfuel.app.ui.forum

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.forum.EditForumCommentUseCase
import com.anpfuel.application.usecase.forum.GetForumPostDetailUseCase
import com.anpfuel.application.usecase.forum.GetForumRepliesUseCase
import com.anpfuel.application.usecase.forum.GetOwnForumCommentIdsUseCase
import com.anpfuel.application.usecase.forum.GetOwnForumReactionsUseCase
import com.anpfuel.application.usecase.forum.GetOwnerTopicUseCase
import com.anpfuel.application.usecase.forum.GetRankedForumCommentsUseCase
import com.anpfuel.application.usecase.forum.ObserveForumConversationUseCase
import com.anpfuel.application.usecase.forum.RemoveForumCommentUseCase
import com.anpfuel.application.usecase.forum.ReportForumTargetUseCase
import com.anpfuel.application.usecase.forum.SetForumReactionUseCase
import com.anpfuel.application.usecase.forum.SubmitForumCommentUseCase
import com.anpfuel.application.usecase.forum.ValidateForumSafeLinkUseCase
import com.anpfuel.domain.forum.ForumComment
import com.anpfuel.domain.forum.ForumCommentReceipt
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumLiveEvent
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumReaction
import com.anpfuel.domain.forum.ForumReactionCounts
import com.anpfuel.domain.forum.ForumReactionTarget
import com.anpfuel.domain.forum.ForumReplySort
import com.anpfuel.domain.forum.ForumReportReason
import com.anpfuel.domain.forum.ForumReportReceipt
import dagger.hilt.android.lifecycle.HiltViewModel
import java.net.URI
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

data class ForumDetailUiState(
    val postId: String = "",
    val post: ForumPostDetail? = null,
    val loading: Boolean = true,
    val refreshing: Boolean = false,
    val error: ForumDetailError? = null,
    val isOffline: Boolean = false,
    val linkConfirmationTarget: String? = null,
    val linkConfirmationHost: String? = null,
    val linkError: Boolean = false,

    // Social discussion additions (FOR-19)
    val isSocialEnabled: Boolean = false,
    val comments: List<ForumComment> = emptyList(),
    val commentsLoading: Boolean = false,
    val expandedReplies: Set<String> = emptySet(),
    val repliesMap: Map<String, List<ForumComment>> = emptyMap(),
    val repliesLoading: Set<String> = emptySet(),
    val replyingTo: ForumComment? = null,
    val composerText: String = "",
    val isSubmittingComment: Boolean = false,
    val submissionReceipt: ForumCommentReceipt? = null,
    val editingComment: ForumComment? = null,
    val editDraftText: String = "",
    val isSavingEdit: Boolean = false,
    val commentPendingRemoval: ForumComment? = null,
    val isRemovingComment: Boolean = false,
    val reportingTarget: Pair<String, String>? = null, // Pair(targetType, targetId)
    val isSubmittingReport: Boolean = false,
    val reportReceipt: ForumReportReceipt? = null,
    val actionError: ForumActionError? = null,
    val isAuthenticated: Boolean = false,
    val requireAuthPrompt: Boolean = false,
    // Ownership confirmed by signed owner reads: edit/remove controls show
    // only on the viewer's own topic and comments.
    val isTopicOwner: Boolean = false,
    val ownCommentIds: Set<String> = emptySet(),

    // X-style conversation (B-BR-FOR-17, B-BR-FOR-18)
    val replySort: ForumReplySort = ForumReplySort.RELEVANT,
    val commentsCursor: String? = null,
    val loadingMoreComments: Boolean = false,
    val postReaction: ForumReaction = ForumReaction.NONE,
    val commentReactions: Map<String, ForumReaction> = emptyMap(),
    /** Replies published while reading, offered behind "new replies". */
    val pendingLiveComments: List<ForumComment> = emptyList(),
    val liveConnected: Boolean = false,
)

sealed interface ForumActionError {
    data object Conflict : ForumActionError
    data class RateLimited(val retryAfterSeconds: Long?) : ForumActionError
    data object TargetMissing : ForumActionError
    data object Offline : ForumActionError
    data class Generic(val message: String?) : ForumActionError
}

enum class ForumDetailError {
    NOT_FOUND,
    DISABLED,
    OFFLINE,
    GENERIC,
}

@HiltViewModel
class ForumDetailViewModel @Inject constructor(
    private val getForumPostDetail: GetForumPostDetailUseCase,
    private val validateForumSafeLink: ValidateForumSafeLinkUseCase,
    private val getForumReplies: GetForumRepliesUseCase,
    private val submitForumComment: SubmitForumCommentUseCase,
    private val editForumComment: EditForumCommentUseCase,
    private val removeForumComment: RemoveForumCommentUseCase,
    private val reportForumTarget: ReportForumTargetUseCase,
    private val getOwnerTopic: GetOwnerTopicUseCase,
    private val getOwnForumCommentIds: GetOwnForumCommentIdsUseCase,
    private val getRankedComments: GetRankedForumCommentsUseCase,
    private val setForumReaction: SetForumReactionUseCase,
    private val getOwnForumReactions: GetOwnForumReactionsUseCase,
    private val observeConversation: ObserveForumConversationUseCase,
    private val sessionStore: AuthSessionStore? = null,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val postIdArg: String = savedStateHandle.get<String>("postId").orEmpty()
    private val _state = MutableStateFlow(
        ForumDetailUiState(
            postId = postIdArg,
            isAuthenticated = sessionStore?.load() != null,
        )
    )
    val state: StateFlow<ForumDetailUiState> = _state.asStateFlow()

    private var loadJob: Job? = null
    private var commentsJob: Job? = null
    private var ownershipJob: Job? = null
    private var writeJob: Job? = null
    private var liveJob: Job? = null
    private var reactionsJob: Job? = null

    // Reaction writes go out in tap order; only the newest answer per target
    // reconciles the optimistic state.
    private val reactionMutex = Mutex()
    private val reactionVersions = mutableMapOf<String, Int>()
    private var lastKnownAccountId: String? = sessionStore?.load()?.accountId

    // A retry of the same unchanged write reuses its operation id so a
    // request that timed out after the server committed replays instead of
    // creating a duplicate; any change to the payload starts a new operation.
    private var pendingWrite: Pair<String, String>? = null

    private fun operationIdFor(payloadKey: String): String {
        val pending = pendingWrite
        if (pending != null && pending.first == payloadKey) return pending.second
        return UUID.randomUUID().toString().also { pendingWrite = payloadKey to it }
    }

    init {
        if (postIdArg.isNotBlank()) {
            load(postIdArg)
        }
    }

    fun setSocialEnabled(enabled: Boolean) {
        _state.update { it.copy(isSocialEnabled = enabled) }
        if (enabled && _state.value.comments.isEmpty() && !_state.value.commentsLoading) {
            val id = _state.value.postId
            if (id.isNotBlank()) {
                loadComments(id)
            }
        }
        _state.value.post?.let { refreshOwnership(it) }
    }

    /**
     * Confirms through signed owner reads whether the viewer owns the topic
     * and which comments are theirs. Failures leave controls hidden.
     */
    private fun refreshOwnership(post: ForumPostDetail) {
        ownershipJob?.cancel()
        if (sessionStore?.load() == null) {
            _state.update { it.copy(isTopicOwner = false, ownCommentIds = emptySet()) }
            return
        }
        ownershipJob = viewModelScope.launch {
            val ownsTopic = post.kind == ForumPostKind.TOPIC && try {
                getOwnerTopic(post.id)
                true
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                false
            }
            val ownComments = if (_state.value.isSocialEnabled) {
                try {
                    getOwnForumCommentIds(post.id)
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    _state.value.ownCommentIds
                }
            } else {
                emptySet()
            }
            _state.update { it.copy(isTopicOwner = ownsTopic, ownCommentIds = ownComments) }
        }
    }

    fun checkSessionConsistency() {
        val currentSession = sessionStore?.load()
        val currentAccountId = currentSession?.accountId
        val wasAuthed = _state.value.isAuthenticated
        val isAuthed = currentSession != null

        if (currentAccountId != lastKnownAccountId) {
            // Account changed or logged out: cancel in-flight work and clear private state
            commentsJob?.cancel()
            writeJob?.cancel()
            ownershipJob?.cancel()
            pendingWrite = null
            _state.update {
                it.copy(
                    isAuthenticated = isAuthed,
                    isTopicOwner = false,
                    ownCommentIds = emptySet(),
                    editingComment = null,
                    editDraftText = "",
                    commentPendingRemoval = null,
                    reportingTarget = null,
                    isSubmittingComment = false,
                    isSavingEdit = false,
                    isRemovingComment = false,
                    isSubmittingReport = false,
                    postReaction = ForumReaction.NONE,
                    commentReactions = emptyMap(),
                )
            }
            lastKnownAccountId = currentAccountId
            if (isAuthed) {
                _state.value.post?.let {
                    refreshOwnership(it)
                    refreshOwnReactions(it.id)
                }
            }
        } else if (wasAuthed != isAuthed) {
            _state.update { it.copy(isAuthenticated = isAuthed) }
        }
    }

    fun load(postId: String) {
        checkSessionConsistency()
        loadJob?.cancel()
        _state.update { it.copy(postId = postId, loading = true, error = null) }
        loadJob = viewModelScope.launch {
            try {
                val detail = getForumPostDetail(postId)
                _state.update {
                    it.copy(
                        post = detail,
                        loading = false,
                        refreshing = false,
                        error = null,
                        isOffline = false,
                    )
                }
                // B-BR-FOR-18: the post first, then its replies.
                if (!_state.value.isSocialEnabled) {
                    setSocialEnabled(true)
                } else {
                    loadComments(postId)
                }
                refreshOwnership(detail)
            } catch (e: ForumException.NotFound) {
                _state.update { it.copy(loading = false, refreshing = false, error = ForumDetailError.NOT_FOUND, post = null) }
            } catch (e: ForumException.Disabled) {
                _state.update { it.copy(loading = false, refreshing = false, error = ForumDetailError.DISABLED) }
            } catch (e: ForumException.Offline) {
                _state.update {
                    it.copy(
                        loading = false,
                        refreshing = false,
                        isOffline = true,
                        error = if (it.post == null) ForumDetailError.OFFLINE else null,
                    )
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update {
                    it.copy(
                        loading = false,
                        refreshing = false,
                        error = if (it.post == null) ForumDetailError.GENERIC else null,
                    )
                }
            }
        }
    }

    fun refresh() {
        val id = _state.value.postId
        if (id.isBlank()) return
        checkSessionConsistency()
        loadJob?.cancel()
        _state.update { it.copy(refreshing = true, error = null) }
        loadJob = viewModelScope.launch {
            try {
                val detail = getForumPostDetail(id)
                _state.update {
                    it.copy(
                        post = detail,
                        loading = false,
                        refreshing = false,
                        error = null,
                        isOffline = false,
                    )
                }
                // B-BR-FOR-18: the post first, then its replies.
                if (!_state.value.isSocialEnabled) {
                    setSocialEnabled(true)
                } else {
                    loadComments(id)
                }
                refreshOwnership(detail)
            } catch (e: ForumException.NotFound) {
                _state.update { it.copy(loading = false, refreshing = false, error = ForumDetailError.NOT_FOUND, post = null) }
            } catch (e: ForumException.Disabled) {
                _state.update { it.copy(loading = false, refreshing = false, error = ForumDetailError.DISABLED) }
            } catch (e: ForumException.Offline) {
                _state.update {
                    it.copy(
                        loading = false,
                        refreshing = false,
                        isOffline = true,
                        error = if (it.post == null) ForumDetailError.OFFLINE else null,
                    )
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update {
                    it.copy(
                        loading = false,
                        refreshing = false,
                        error = if (it.post == null) ForumDetailError.GENERIC else null,
                    )
                }
            }
        }
    }

    fun loadComments(postId: String) {
        commentsJob?.cancel()
        _state.update { it.copy(commentsLoading = true) }
        commentsJob = viewModelScope.launch {
            try {
                val page = getRankedComments(postId, _state.value.replySort)
                _state.update {
                    it.copy(
                        comments = page.items,
                        commentsCursor = page.nextCursor,
                        commentsLoading = false,
                        // Everything new is now in the list itself.
                        pendingLiveComments = emptyList(),
                    )
                }
                refreshOwnReactions(postId)
            } catch (e: ForumException.Offline) {
                _state.update { it.copy(commentsLoading = false, isOffline = true) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update { it.copy(commentsLoading = false) }
            }
        }
    }

    /** Reorders the replies; each order has its own keyset pages. */
    fun setReplySort(sort: ForumReplySort) {
        if (sort == _state.value.replySort) return
        _state.update { it.copy(replySort = sort, comments = emptyList(), commentsCursor = null) }
        val id = _state.value.postId
        if (id.isNotBlank()) loadComments(id)
    }

    fun loadMoreComments() {
        val cur = _state.value
        val cursor = cur.commentsCursor ?: return
        if (cur.loadingMoreComments || cur.commentsLoading) return
        _state.update { it.copy(loadingMoreComments = true) }
        viewModelScope.launch {
            try {
                val page = getRankedComments(cur.postId, cur.replySort, cursor = cursor)
                _state.update { state ->
                    val known = state.comments.map { it.id }.toSet()
                    state.copy(
                        comments = state.comments + page.items.filterNot { it.id in known },
                        commentsCursor = page.nextCursor,
                        loadingMoreComments = false,
                    )
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update { it.copy(loadingMoreComments = false) }
            }
        }
    }

    /** Brings live replies into the list, at the top, when the reader asks. */
    fun showNewReplies() {
        _state.update { state ->
            val fresh = state.pendingLiveComments.filterNot { p -> state.comments.any { it.id == p.id } }
            state.copy(comments = fresh + state.comments, pendingLiveComments = emptyList())
        }
    }

    private fun refreshOwnReactions(postId: String) {
        reactionsJob?.cancel()
        if (sessionStore?.load() == null) {
            _state.update { it.copy(postReaction = ForumReaction.NONE, commentReactions = emptyMap()) }
            return
        }
        reactionsJob = viewModelScope.launch {
            try {
                val own = getOwnForumReactions(postId)
                _state.update { it.copy(postReaction = own.post, commentReactions = own.comments) }
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // Counts stay public; only the highlight of the caller's own votes is missing.
            }
        }
    }

    fun reactToPost(tapped: ForumReaction) {
        val post = _state.value.post ?: return
        val from = _state.value.postReaction
        val to = from.toggle(tapped)
        sendReaction(ForumReactionTarget.POST, post.id, from, to)
    }

    fun reactToComment(comment: ForumComment, tapped: ForumReaction) {
        val from = _state.value.commentReactions[comment.id] ?: ForumReaction.NONE
        val to = from.toggle(tapped)
        sendReaction(ForumReactionTarget.COMMENT, comment.id, from, to)
    }

    /**
     * Optimistic reaction: the count and highlight change at once, the
     * signed write follows in tap order, and a failure puts both back.
     */
    private fun sendReaction(target: ForumReactionTarget, targetId: String, from: ForumReaction, to: ForumReaction) {
        checkSessionConsistency()
        if (sessionStore?.load() == null) {
            _state.update { it.copy(requireAuthPrompt = true, isAuthenticated = false) }
            return
        }
        applyLocalReaction(target, targetId, to) { it.applying(from, to) }
        val version = (reactionVersions[targetId] ?: 0) + 1
        reactionVersions[targetId] = version
        viewModelScope.launch {
            try {
                val result = reactionMutex.withLock {
                    setForumReaction(target, targetId, to, UUID.randomUUID().toString())
                }
                if (reactionVersions[targetId] == version) {
                    applyLocalReaction(target, targetId, result.reaction) { result.counts }
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (reactionVersions[targetId] == version) {
                    applyLocalReaction(target, targetId, from) { it.applying(to, from) }
                }
                val error = when (e) {
                    is ForumException.AuthRequired -> {
                        _state.update { it.copy(requireAuthPrompt = true, isAuthenticated = false) }
                        null
                    }
                    is ForumException.RateLimited -> ForumActionError.RateLimited(e.retryAfterSeconds)
                    is ForumException.NotFound -> ForumActionError.TargetMissing
                    is ForumException.Offline -> ForumActionError.Offline
                    else -> ForumActionError.Generic(e.message)
                }
                if (error != null) _state.update { it.copy(actionError = error) }
            }
        }
    }

    private fun applyLocalReaction(
        target: ForumReactionTarget,
        targetId: String,
        reaction: ForumReaction,
        counts: (ForumReactionCounts) -> ForumReactionCounts,
    ) {
        _state.update { state ->
            when (target) {
                ForumReactionTarget.POST -> state.copy(
                    postReaction = reaction,
                    post = state.post?.let { it.copy(reactions = counts(it.reactions)) },
                )
                ForumReactionTarget.COMMENT -> state.updatingComment(targetId) { it.copy(reactions = counts(it.reactions)) }
                    .copy(commentReactions = state.commentReactions + (targetId to reaction))
            }
        }
    }

    /** Opens the live stream while the conversation is on screen. */
    fun startLive() {
        if (liveJob?.isActive == true) return
        val postId = _state.value.postId
        if (postId.isBlank()) return
        liveJob = viewModelScope.launch {
            var backoffMs = LIVE_MIN_BACKOFF_MS
            while (isActive) {
                try {
                    observeConversation(postId).collect { event ->
                        backoffMs = LIVE_MIN_BACKOFF_MS
                        onLiveEvent(event)
                    }
                    // The server ended the stream on purpose; reconnect soon.
                    delay(LIVE_MIN_BACKOFF_MS)
                } catch (e: CancellationException) {
                    throw e
                } catch (e: ForumException.NotFound) {
                    break
                } catch (_: Exception) {
                    _state.update { it.copy(liveConnected = false) }
                    delay(backoffMs)
                    backoffMs = (backoffMs * 2).coerceAtMost(LIVE_MAX_BACKOFF_MS)
                }
            }
            _state.update { it.copy(liveConnected = false) }
        }
    }

    fun stopLive() {
        liveJob?.cancel()
        liveJob = null
        _state.update { it.copy(liveConnected = false) }
    }

    internal fun onLiveEvent(event: ForumLiveEvent) {
        when (event) {
            ForumLiveEvent.Ready -> _state.update { it.copy(liveConnected = true) }
            is ForumLiveEvent.CommentPublished -> onLiveComment(event.comment)
            is ForumLiveEvent.CommentReactions -> _state.update { state ->
                state.updatingComment(event.commentId) { it.copy(reactions = event.counts) }
            }
            is ForumLiveEvent.CommentGone -> _state.update { state ->
                state.removingComment(event.commentId, event.parentId)
            }
            is ForumLiveEvent.PostReactions -> _state.update { state ->
                state.copy(post = state.post?.copy(reactions = event.counts))
            }
            ForumLiveEvent.PostGone -> {
                _state.update { it.copy(post = null, error = ForumDetailError.NOT_FOUND, liveConnected = false) }
                liveJob?.cancel()
            }
            ForumLiveEvent.Resync -> {
                _state.update { it.copy(liveConnected = false) }
                val id = _state.value.postId
                if (id.isNotBlank()) loadComments(id)
            }
            ForumLiveEvent.Reconnect -> _state.update { it.copy(liveConnected = false) }
        }
    }

    private fun onLiveComment(comment: ForumComment) {
        _state.update { state ->
            val parentId = comment.parentId
            when {
                parentId == null && state.comments.any { it.id == comment.id } ->
                    state.updatingComment(comment.id) { comment.copy(replyCount = it.replyCount, topReply = it.topReply) }
                parentId == null ->
                    if (state.pendingLiveComments.any { it.id == comment.id }) state
                    else state.copy(pendingLiveComments = listOf(comment) + state.pendingLiveComments)
                else -> {
                    val loaded = state.repliesMap[parentId]
                    val withReply = if (loaded != null && loaded.none { it.id == comment.id }) {
                        state.copy(repliesMap = state.repliesMap + (parentId to loaded + comment))
                    } else {
                        state
                    }
                    if (loaded?.any { it.id == comment.id } == true) withReply
                    else withReply.copy(
                        comments = withReply.comments.map {
                            if (it.id == parentId) it.copy(replyCount = it.replyCount + 1, topReply = it.topReply ?: comment) else it
                        },
                    )
                }
            }
        }
    }

    fun toggleReplies(commentId: String) {
        val isExpanded = _state.value.expandedReplies.contains(commentId)
        if (isExpanded) {
            _state.update { it.copy(expandedReplies = it.expandedReplies - commentId) }
        } else {
            _state.update { it.copy(expandedReplies = it.expandedReplies + commentId) }
            if (!_state.value.repliesMap.containsKey(commentId)) {
                loadReplies(commentId)
            }
        }
    }

    private fun loadReplies(commentId: String) {
        _state.update { it.copy(repliesLoading = it.repliesLoading + commentId) }
        viewModelScope.launch {
            try {
                val replies = getForumReplies(commentId)
                _state.update {
                    it.copy(
                        repliesMap = it.repliesMap + (commentId to replies.items),
                        repliesLoading = it.repliesLoading - commentId,
                    )
                }
            } catch (e: ForumException.Offline) {
                _state.update {
                    it.copy(
                        repliesLoading = it.repliesLoading - commentId,
                        isOffline = true,
                    )
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update { it.copy(repliesLoading = it.repliesLoading - commentId) }
            }
        }
    }

    fun startReply(comment: ForumComment) {
        _state.update { it.copy(replyingTo = comment) }
    }

    fun cancelReply() {
        _state.update { it.copy(replyingTo = null) }
    }

    fun updateComposerText(text: String) {
        val clamped = if (text.length > 280) text.take(280) else text
        _state.update { it.copy(composerText = clamped) }
    }

    fun submitComment() {
        val cur = _state.value
        if (cur.isSubmittingComment || cur.composerText.isBlank()) return
        checkSessionConsistency()
        if (sessionStore?.load() == null) {
            _state.update { it.copy(requireAuthPrompt = true, isAuthenticated = false) }
            return
        }

        val bodyText = cur.composerText.trim()
        val parentId = cur.replyingTo?.id
        val operationId = operationIdFor("create|${cur.postId}|$parentId|$bodyText")
        _state.update { it.copy(isSubmittingComment = true, actionError = null) }
        writeJob = viewModelScope.launch {
            try {
                val receipt = submitForumComment(
                    postId = cur.postId,
                    bodyText = bodyText,
                    parentId = parentId,
                    operationId = operationId,
                )
                pendingWrite = null
                _state.update {
                    it.copy(
                        isSubmittingComment = false,
                        composerText = "",
                        replyingTo = null,
                        submissionReceipt = receipt,
                        ownCommentIds = it.ownCommentIds + receipt.id,
                    )
                }
            } catch (e: ForumException.AuthRequired) {
                _state.update { it.copy(isSubmittingComment = false, requireAuthPrompt = true, isAuthenticated = false) }
            } catch (e: ForumException.RateLimited) {
                _state.update { it.copy(isSubmittingComment = false, actionError = ForumActionError.RateLimited(e.retryAfterSeconds)) }
            } catch (e: ForumException.Conflict) {
                _state.update { it.copy(isSubmittingComment = false, actionError = ForumActionError.Conflict) }
            } catch (e: ForumException.NotFound) {
                _state.update { it.copy(isSubmittingComment = false, actionError = ForumActionError.TargetMissing) }
            } catch (e: ForumException.Offline) {
                _state.update { it.copy(isSubmittingComment = false, isOffline = true, actionError = ForumActionError.Offline) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update { it.copy(isSubmittingComment = false, actionError = ForumActionError.Generic(e.message)) }
            }
        }
    }

    fun startEdit(comment: ForumComment) {
        _state.update { it.copy(editingComment = comment, editDraftText = comment.bodyText) }
    }

    fun updateEditDraftText(text: String) {
        val clamped = if (text.length > 280) text.take(280) else text
        _state.update { it.copy(editDraftText = clamped) }
    }

    fun cancelEdit() {
        _state.update { it.copy(editingComment = null, editDraftText = "") }
    }

    fun saveEdit() {
        val cur = _state.value
        val comment = cur.editingComment ?: return
        if (cur.isSavingEdit || cur.editDraftText.isBlank()) return
        checkSessionConsistency()
        if (sessionStore?.load() == null) {
            _state.update { it.copy(requireAuthPrompt = true, isAuthenticated = false) }
            return
        }

        val bodyText = cur.editDraftText.trim()
        val operationId = operationIdFor("edit|${comment.id}|${comment.revision}|$bodyText")
        _state.update { it.copy(isSavingEdit = true, actionError = null) }
        writeJob = viewModelScope.launch {
            try {
                val receipt = editForumComment(
                    commentId = comment.id,
                    expectedRevision = comment.revision,
                    bodyText = bodyText,
                    operationId = operationId,
                )
                pendingWrite = null
                _state.update {
                    it.copy(
                        isSavingEdit = false,
                        editingComment = null,
                        editDraftText = "",
                        submissionReceipt = receipt,
                    )
                }
                loadComments(cur.postId)
            } catch (e: ForumException.AuthRequired) {
                _state.update { it.copy(isSavingEdit = false, requireAuthPrompt = true, isAuthenticated = false) }
            } catch (e: ForumException.RateLimited) {
                _state.update { it.copy(isSavingEdit = false, actionError = ForumActionError.RateLimited(e.retryAfterSeconds)) }
            } catch (e: ForumException.Conflict) {
                // Draft remains preserved in editDraftText for owner to recover
                _state.update { it.copy(isSavingEdit = false, actionError = ForumActionError.Conflict) }
            } catch (e: ForumException.NotFound) {
                _state.update { it.copy(isSavingEdit = false, editingComment = null, editDraftText = "", actionError = ForumActionError.TargetMissing) }
                loadComments(cur.postId)
            } catch (e: ForumException.Offline) {
                _state.update { it.copy(isSavingEdit = false, isOffline = true, actionError = ForumActionError.Offline) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update { it.copy(isSavingEdit = false, actionError = ForumActionError.Generic(e.message)) }
            }
        }
    }

    fun confirmRemove(comment: ForumComment) {
        _state.update { it.copy(commentPendingRemoval = comment) }
    }

    fun cancelRemove() {
        _state.update { it.copy(commentPendingRemoval = null) }
    }

    fun executeRemove() {
        val cur = _state.value
        val comment = cur.commentPendingRemoval ?: return
        if (cur.isRemovingComment) return
        checkSessionConsistency()
        if (sessionStore?.load() == null) {
            _state.update { it.copy(requireAuthPrompt = true, isAuthenticated = false) }
            return
        }

        _state.update { it.copy(isRemovingComment = true, actionError = null) }
        writeJob = viewModelScope.launch {
            try {
                removeForumComment(
                    commentId = comment.id,
                    expectedRevision = comment.revision,
                )
                _state.update { it.copy(isRemovingComment = false, commentPendingRemoval = null) }
                loadComments(cur.postId)
            } catch (e: ForumException.AuthRequired) {
                _state.update { it.copy(isRemovingComment = false, requireAuthPrompt = true, isAuthenticated = false) }
            } catch (e: ForumException.NotFound) {
                _state.update { it.copy(isRemovingComment = false, commentPendingRemoval = null, actionError = ForumActionError.TargetMissing) }
                loadComments(cur.postId)
            } catch (e: ForumException.Conflict) {
                _state.update { it.copy(isRemovingComment = false, actionError = ForumActionError.Conflict) }
                loadComments(cur.postId)
            } catch (e: ForumException.RateLimited) {
                _state.update { it.copy(isRemovingComment = false, actionError = ForumActionError.RateLimited(e.retryAfterSeconds)) }
            } catch (e: ForumException.Offline) {
                _state.update { it.copy(isRemovingComment = false, isOffline = true, actionError = ForumActionError.Offline) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update { it.copy(isRemovingComment = false, actionError = ForumActionError.Generic(e.message)) }
            }
        }
    }

    fun openReport(targetType: String, targetId: String) {
        checkSessionConsistency()
        if (sessionStore?.load() == null) {
            _state.update { it.copy(requireAuthPrompt = true, isAuthenticated = false) }
            return
        }
        _state.update { it.copy(reportingTarget = Pair(targetType, targetId)) }
    }

    fun cancelReport() {
        _state.update { it.copy(reportingTarget = null) }
    }

    fun submitReport(reason: ForumReportReason, detail: String? = null) {
        val cur = _state.value
        val target = cur.reportingTarget ?: return
        if (cur.isSubmittingReport) return

        _state.update { it.copy(isSubmittingReport = true, actionError = null) }
        writeJob = viewModelScope.launch {
            try {
                val receipt = reportForumTarget(
                    targetType = target.first,
                    targetId = target.second,
                    reason = reason,
                    detail = detail,
                )
                _state.update {
                    it.copy(
                        isSubmittingReport = false,
                        reportingTarget = null,
                        reportReceipt = receipt,
                    )
                }
            } catch (e: ForumException.AuthRequired) {
                _state.update { it.copy(isSubmittingReport = false, requireAuthPrompt = true, isAuthenticated = false) }
            } catch (e: ForumException.RateLimited) {
                _state.update { it.copy(isSubmittingReport = false, actionError = ForumActionError.RateLimited(e.retryAfterSeconds)) }
            } catch (e: ForumException.NotFound) {
                _state.update { it.copy(isSubmittingReport = false, reportingTarget = null, actionError = ForumActionError.TargetMissing) }
            } catch (e: ForumException.Offline) {
                _state.update { it.copy(isSubmittingReport = false, isOffline = true, actionError = ForumActionError.Offline) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update { it.copy(isSubmittingReport = false, actionError = ForumActionError.Generic(e.message)) }
            }
        }
    }

    fun dismissReportReceipt() {
        _state.update { it.copy(reportReceipt = null) }
    }

    fun dismissSubmissionReceipt() {
        _state.update { it.copy(submissionReceipt = null) }
    }

    fun dismissActionError() {
        _state.update { it.copy(actionError = null) }
    }

    fun dismissAuthPrompt() {
        _state.update { it.copy(requireAuthPrompt = false) }
    }

    fun onExternalLinkClick(url: String) {
        val isValid = try {
            validateForumSafeLink(url)
        } catch (_: Exception) {
            false
        }
        if (isValid) {
            val host = try {
                URI(url).host ?: url
            } catch (_: Exception) {
                url
            }
            _state.update {
                it.copy(
                    linkConfirmationTarget = url,
                    linkConfirmationHost = host,
                    linkError = false,
                )
            }
        } else {
            _state.update { it.copy(linkError = true) }
        }
    }

    fun dismissLinkDialog() {
        _state.update {
            it.copy(
                linkConfirmationTarget = null,
                linkConfirmationHost = null,
                linkError = false,
            )
        }
    }
}

private const val LIVE_MIN_BACKOFF_MS = 1_000L
private const val LIVE_MAX_BACKOFF_MS = 30_000L

/** Applies [change] to the comment wherever it shows: list, previews, threads, pending. */
internal fun ForumDetailUiState.updatingComment(
    commentId: String,
    change: (ForumComment) -> ForumComment,
): ForumDetailUiState {
    fun ForumComment.updated(): ForumComment {
        val self = if (id == commentId) change(this) else this
        val preview = self.topReply?.let { if (it.id == commentId) change(it) else it }
        return if (preview === self.topReply) self else self.copy(topReply = preview)
    }
    return copy(
        comments = comments.map { it.updated() },
        pendingLiveComments = pendingLiveComments.map { it.updated() },
        repliesMap = repliesMap.mapValues { (_, replies) -> replies.map { if (it.id == commentId) change(it) else it } },
    )
}

/** Drops a comment that left public reads and keeps reply counts honest. */
internal fun ForumDetailUiState.removingComment(commentId: String, parentId: String?): ForumDetailUiState {
    val wasReplyListed = parentId != null && repliesMap[parentId]?.any { it.id == commentId } == true
    return copy(
        comments = comments.filterNot { it.id == commentId }.map { c ->
            if (c.id != parentId) return@map c
            val preview = c.topReply?.takeIf { it.id != commentId }
            c.copy(replyCount = (c.replyCount - 1).coerceAtLeast(0), topReply = preview)
        },
        pendingLiveComments = pendingLiveComments.filterNot { it.id == commentId },
        repliesMap = if (wasReplyListed) repliesMap + (parentId!! to repliesMap.getValue(parentId).filterNot { it.id == commentId }) else repliesMap,
    )
}
