package com.anpfuel.app.ui.forum

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.forum.GetForumPostDetailUseCase
import com.anpfuel.application.usecase.forum.ValidateForumSafeLinkUseCase
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumPostDetail
import dagger.hilt.android.lifecycle.HiltViewModel
import java.net.URI
import javax.inject.Inject
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
)

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
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val postIdArg: String = savedStateHandle.get<String>("postId").orEmpty()
    private val _state = MutableStateFlow(ForumDetailUiState(postId = postIdArg))
    val state: StateFlow<ForumDetailUiState> = _state.asStateFlow()

    private var loadJob: Job? = null

    init {
        if (postIdArg.isNotBlank()) {
            load(postIdArg)
        }
    }

    fun load(postId: String) {
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
