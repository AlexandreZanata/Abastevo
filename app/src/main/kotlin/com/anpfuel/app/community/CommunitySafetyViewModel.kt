package com.anpfuel.app.community

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.safety.BlockCommunityUserUseCase
import com.anpfuel.application.usecase.safety.FilterHiddenContentUseCase
import com.anpfuel.application.usecase.safety.ReportCommunityUserUseCase
import com.anpfuel.domain.safety.CommunitySafetyException
import com.anpfuel.domain.safety.SafetyContentRef
import com.anpfuel.domain.safety.UserReportReason
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** What a list of user content may show and which safety dialog is open. */
data class CommunitySafetyUiState(
    /** References already asked about; only these are shown, so blocked content never flashes. */
    val checked: Set<SafetyContentRef> = emptySet(),
    /** Content written by accounts the viewer blocked (B-BR-SAFE-06). */
    val hidden: Set<SafetyContentRef> = emptySet(),
    val userReportTarget: SafetyContentRef? = null,
    val submittingReport: Boolean = false,
    val blockTarget: SafetyContentRef? = null,
    val blocking: Boolean = false,
    val notice: CommunitySafetyNotice? = null,
    val error: CommunitySafetyError? = null,
    /** The session is gone: the screen sends the user to sign in (B-BR-SAFE-03). */
    val requireSignIn: Boolean = false,
) {
    fun isVisible(ref: SafetyContentRef): Boolean = ref in checked && ref !in hidden
}

enum class CommunitySafetyNotice { REPORTED, REPORT_REPLAYED, BLOCKED }

sealed interface CommunitySafetyError {
    data object SelfTarget : CommunitySafetyError
    data object BlockLimit : CommunitySafetyError
    data class RateLimited(val retryAfterSeconds: Long?) : CommunitySafetyError
    data object NotFound : CommunitySafetyError
    data object Offline : CommunitySafetyError
    data object Generic : CommunitySafetyError
}

/**
 * Report-user, block-user and hidden-content filtering for a list of
 * public user content outside the forum, such as station comments
 * (BUC-SAFE-02, BUC-SAFE-03, B-BR-SAFE-06). The screen decides who is
 * signed in; this model never runs an action implicitly after sign-in.
 */
@HiltViewModel
class CommunitySafetyViewModel @Inject constructor(
    private val filterHiddenContent: FilterHiddenContentUseCase,
    private val reportCommunityUser: ReportCommunityUserUseCase,
    private val blockCommunityUser: BlockCommunityUserUseCase,
) : ViewModel() {

    private val _state = MutableStateFlow(CommunitySafetyUiState())
    val state: StateFlow<CommunitySafetyUiState> = _state.asStateFlow()

    private var shown: List<SafetyContentRef> = emptyList()
    private var filterJob: Job? = null
    private var actionJob: Job? = null
    private var recheckPending = false

    /** Blocks made here stay hidden even when a later lookup fails open. */
    private val blockedHere = mutableSetOf<SafetyContentRef>()

    init {
        // A block or unblock made on another screen applies here too.
        viewModelScope.launch {
            filterHiddenContent.blockRevisions.drop(1).collect { runLookup(recheck = true) }
        }
    }

    /**
     * Asks which of the listed references are hidden; references already
     * asked about are not asked again. The lookup fails open: on any error
     * the content shows.
     */
    fun show(refs: List<SafetyContentRef>) {
        shown = refs.distinct()
        runLookup(recheck = false)
    }

    /** One lookup at a time; a cancelled recheck is carried into the next one. */
    private fun runLookup(recheck: Boolean) {
        if (recheck) recheckPending = true
        filterJob?.cancel()
        filterJob = viewModelScope.launch {
            val all = recheckPending
            lookup(shown, recheck = all)
            if (all) recheckPending = false
        }
    }

    private suspend fun lookup(refs: List<SafetyContentRef>, recheck: Boolean) {
        val ask = if (recheck) refs else refs.filterNot { it in _state.value.checked }
        if (ask.isEmpty()) return
        val found = try {
            filterHiddenContent(ask)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            emptySet()
        }
        _state.update {
            val base = if (recheck) it.hidden - ask.toSet() else it.hidden
            it.copy(checked = it.checked + ask, hidden = base + found + blockedHere)
        }
    }

    fun openReportUser(ref: SafetyContentRef) {
        _state.update { it.copy(userReportTarget = ref, notice = null, error = null) }
    }

    fun cancelReportUser() {
        _state.update { it.copy(userReportTarget = null) }
    }

    fun submitReportUser(reason: UserReportReason, detail: String?) {
        val current = _state.value
        val target = current.userReportTarget ?: return
        if (current.submittingReport) return
        _state.update { it.copy(submittingReport = true, error = null) }
        actionJob = viewModelScope.launch {
            try {
                val receipt = reportCommunityUser(target, reason, detail)
                val notice = if (receipt.replayed) CommunitySafetyNotice.REPORT_REPLAYED else CommunitySafetyNotice.REPORTED
                _state.update { it.copy(submittingReport = false, userReportTarget = null, notice = notice) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                fail(e)
                _state.update { it.copy(submittingReport = false, userReportTarget = null) }
            }
        }
    }

    fun openBlockUser(ref: SafetyContentRef) {
        _state.update { it.copy(blockTarget = ref, notice = null, error = null) }
    }

    fun cancelBlockUser() {
        _state.update { it.copy(blockTarget = null) }
    }

    /** Blocks the author, hides the source at once and filters the list again (B-BR-SAFE-05). */
    fun confirmBlockUser() {
        val current = _state.value
        val target = current.blockTarget ?: return
        if (current.blocking) return
        _state.update { it.copy(blocking = true, error = null) }
        actionJob = viewModelScope.launch {
            try {
                blockCommunityUser(target)
                blockedHere += target
                _state.update {
                    it.copy(blocking = false, blockTarget = null, hidden = it.hidden + target, notice = CommunitySafetyNotice.BLOCKED)
                }
                runLookup(recheck = true)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                fail(e)
                _state.update { it.copy(blocking = false, blockTarget = null) }
            }
        }
    }

    fun dismissMessages() {
        _state.update { it.copy(notice = null, error = null) }
    }

    fun consumeSignIn() {
        _state.update { it.copy(requireSignIn = false) }
    }

    private fun fail(e: Exception) {
        val error = when (e) {
            is CommunitySafetyException.AuthRequired -> {
                _state.update { it.copy(requireSignIn = true) }
                null
            }
            is CommunitySafetyException.SelfTarget -> CommunitySafetyError.SelfTarget
            is CommunitySafetyException.BlockLimit -> CommunitySafetyError.BlockLimit
            is CommunitySafetyException.RateLimited -> CommunitySafetyError.RateLimited(e.retryAfterSeconds)
            is CommunitySafetyException.NotFound -> CommunitySafetyError.NotFound
            is CommunitySafetyException.Offline -> CommunitySafetyError.Offline
            else -> CommunitySafetyError.Generic
        }
        _state.update { it.copy(error = error) }
    }
}
