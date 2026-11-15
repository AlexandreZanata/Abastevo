package com.anpfuel.app.ui.safety

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.app.community.CommunitySafetyError
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.safety.ListBlockedUsersUseCase
import com.anpfuel.application.usecase.safety.UnblockCommunityUserUseCase
import com.anpfuel.domain.safety.BlockedUser
import com.anpfuel.domain.safety.CommunitySafetyException
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class BlockedUsersUiState(
    val signedIn: Boolean = true,
    val loading: Boolean = true,
    val loadFailed: Boolean = false,
    val items: List<BlockedUser> = emptyList(),
    val pendingUnblock: BlockedUser? = null,
    val unblocking: Boolean = false,
    val unblockedNotice: Boolean = false,
    val error: CommunitySafetyError? = null,
    /** The session is gone: the screen sends the user to sign in (B-BR-SAFE-03). */
    val requireSignIn: Boolean = false,
) {
    val isEmpty: Boolean
        get() = signedIn && !loading && !loadFailed && items.isEmpty()
}

/**
 * The account's own block list (B-BR-SAFE-07, BUC-SAFE-04): label, source
 * kind and date of each block, newest first, and unblock with
 * confirmation. Unblocking takes effect at once; the author's content
 * shows again on the next load.
 */
@HiltViewModel
class BlockedUsersViewModel @Inject constructor(
    private val listBlockedUsers: ListBlockedUsersUseCase,
    private val unblockCommunityUser: UnblockCommunityUserUseCase,
    private val sessionStore: AuthSessionStore,
) : ViewModel() {

    private val _state = MutableStateFlow(BlockedUsersUiState())
    val state: StateFlow<BlockedUsersUiState> = _state.asStateFlow()

    private var loadJob: Job? = null
    private var unblockJob: Job? = null

    init {
        load()
    }

    fun load() {
        if (sessionStore.load() == null) {
            loadJob?.cancel()
            _state.value = BlockedUsersUiState(signedIn = false, loading = false)
            return
        }
        loadJob?.cancel()
        _state.update { it.copy(signedIn = true, loading = true, loadFailed = false) }
        loadJob = viewModelScope.launch {
            try {
                val items = listBlockedUsers()
                _state.update { it.copy(loading = false, loadFailed = false, items = items) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: CommunitySafetyException.AuthRequired) {
                _state.update { it.copy(loading = false, signedIn = false, items = emptyList()) }
            } catch (e: Exception) {
                _state.update { it.copy(loading = false, loadFailed = true) }
            }
        }
    }

    fun askUnblock(user: BlockedUser) {
        _state.update { it.copy(pendingUnblock = user, unblockedNotice = false, error = null) }
    }

    fun cancelUnblock() {
        _state.update { it.copy(pendingUnblock = null) }
    }

    fun confirmUnblock() {
        val current = _state.value
        val target = current.pendingUnblock ?: return
        if (current.unblocking) return
        _state.update { it.copy(unblocking = true, error = null) }
        unblockJob = viewModelScope.launch {
            try {
                unblockCommunityUser(target.blockId)
                _state.update { state ->
                    state.copy(
                        unblocking = false,
                        pendingUnblock = null,
                        unblockedNotice = true,
                        items = state.items.filterNot { it.blockId == target.blockId },
                    )
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: CommunitySafetyException.NotFound) {
                // Already gone (another device, account change): show the server's list.
                _state.update { state ->
                    state.copy(
                        unblocking = false,
                        pendingUnblock = null,
                        items = state.items.filterNot { it.blockId == target.blockId },
                    )
                }
                load()
            } catch (e: Exception) {
                val error = when (e) {
                    is CommunitySafetyException.AuthRequired -> {
                        _state.update { it.copy(requireSignIn = true) }
                        null
                    }
                    is CommunitySafetyException.RateLimited -> CommunitySafetyError.RateLimited(e.retryAfterSeconds)
                    is CommunitySafetyException.Offline -> CommunitySafetyError.Offline
                    else -> CommunitySafetyError.Generic
                }
                _state.update { it.copy(unblocking = false, pendingUnblock = null, error = error) }
            }
        }
    }

    fun dismissMessages() {
        _state.update { it.copy(unblockedNotice = false, error = null) }
    }

    fun consumeSignIn() {
        _state.update { it.copy(requireSignIn = false) }
    }
}
