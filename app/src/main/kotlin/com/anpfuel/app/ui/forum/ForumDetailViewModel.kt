package com.anpfuel.app.ui.forum

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.forum.EditForumCommentUseCase
import com.anpfuel.application.usecase.forum.GetForumCommentsUseCase
import com.anpfuel.application.usecase.forum.GetForumPostDetailUseCase
import com.anpfuel.application.usecase.forum.GetForumRepliesUseCase
import com.anpfuel.application.usecase.forum.RemoveForumCommentUseCase
import com.anpfuel.application.usecase.forum.ReportForumTargetUseCase
import com.anpfuel.application.usecase.forum.SubmitForumCommentUseCase
import com.anpfuel.application.usecase.forum.ValidateForumSafeLinkUseCase
import com.anpfuel.domain.forum.ForumComment
import com.anpfuel.domain.forum.ForumCommentReceipt
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumReportReason
import com.anpfuel.domain.forum.ForumReportReceipt
import dagger.hilt.android.lifecycle.HiltViewModel
import java.net.URI
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

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
    private val getForumComments: GetForumCommentsUseCase,
    private val getForumReplies: GetForumRepliesUseCase,
    private val submitForumComment: SubmitForumCommentUseCase,
    private val editForumComment: EditForumCommentUseCase,
    private val removeForumComment: RemoveForumCommentUseCase,
    private val reportForumTarget: ReportForumTargetUseCase,
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
    }

    fun checkSessionConsistency() {
        val currentSession = sessionStore?.load()
        val currentAccountId = currentSession?.accountId
        val wasAuthed = _state.value.isAuthenticated
        val isAuthed = currentSession != null

        if (currentAccountId != lastKnownAccountId) {
            // Account changed or logged out: cancel in-flight work and clear private state
            commentsJob?.cancel()
            _state.update {
                it.copy(
                    isAuthenticated = isAuthed,
                    editingComment = null,
                    editDraftText = "",
                    commentPendingRemoval = null,
                    reportingTarget = null,
                    isSubmittingComment = false,
                    isSavingEdit = false,
                    isRemovingComment = false,
                    isSubmittingReport = false,
                )
            }
            lastKnownAccountId = currentAccountId
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
                if (_state.value.isSocialEnabled) {
                    loadComments(postId)
                }
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
                if (_state.value.isSocialEnabled) {
                    loadComments(id)
                }
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
                val page = getForumComments(postId)
                _state.update {
                    it.copy(
                        comments = page.items,
                        commentsLoading = false,
                    )
                }
            } catch (e: ForumException.Offline) {
                _state.update { it.copy(commentsLoading = false, isOffline = true) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update { it.copy(commentsLoading = false) }
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
        viewModelScope.launch {
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
        viewModelScope.launch {
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
        viewModelScope.launch {
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
        viewModelScope.launch {
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
