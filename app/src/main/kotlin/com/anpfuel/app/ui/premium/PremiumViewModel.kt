package com.anpfuel.app.ui.premium

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.premium.GetPremiumStatusUseCase
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusException
import dagger.hilt.android.lifecycle.HiltViewModel
import java.io.IOException
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Honest Premium entry states; no local entitlement is ever inferred. */
enum class PremiumNotice { OFFLINE, DENIED, ERROR, RESTORE_UNAVAILABLE, STORE_FAILED }

data class PremiumUiState(
    val signedOut: Boolean = true,
    val loading: Boolean = false,
    val status: PremiumStatus? = null,
    val notice: PremiumNotice? = null,
)

/**
 * MEX-16 verified Premium status/manage entry.
 *
 * Reads only the server-derived status for the current owner. Manage
 * opens the provider's subscription page; restore is unavailable in
 * this build because no Play Billing token source is shipped, so it
 * never fabricates a purchase or a local grant. Account changes and
 * late responses are fenced by generation and owner comparison.
 */
@HiltViewModel
class PremiumViewModel @Inject constructor(
    private val status: GetPremiumStatusUseCase,
    private val sessions: AuthSessionStore,
) : ViewModel() {
    private val state = MutableStateFlow(PremiumUiState(signedOut = false, loading = true))
    val uiState = state.asStateFlow()
    private var generation = 0L
    private var statusOwner: String? = null

    fun refresh() {
        val request = ++generation
        val owner = sessions.load()?.accountId
        if (owner == null) {
            statusOwner = null
            state.value = PremiumUiState(signedOut = true)
            return
        }
        state.value = if (statusOwner == owner) {
            state.value.copy(signedOut = false, loading = true, notice = null)
        } else {
            statusOwner = owner
            PremiumUiState(signedOut = false, loading = true)
        }
        viewModelScope.launch {
            try {
                val snapshot = status.invoke()
                if (request != generation) return@launch
                if (sessions.load()?.accountId != owner || snapshot.accountId != owner) {
                    statusOwner = null
                    state.value = PremiumUiState(signedOut = true)
                    return@launch
                }
                state.value = PremiumUiState(signedOut = false, status = snapshot.status)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (error: Exception) {
                if (request != generation) return@launch
                if (sessions.load()?.accountId != owner) {
                    statusOwner = null
                    state.value = PremiumUiState(signedOut = true)
                    return@launch
                }
                state.value = state.value.copy(loading = false, status = null, notice = noticeFor(error))
            }
        }
    }

    fun clearNotice() {
        state.value = state.value.copy(notice = null)
    }

    fun restoreUnavailable() {
        state.value = state.value.copy(notice = PremiumNotice.RESTORE_UNAVAILABLE)
    }

    fun storeOpenFailed() {
        state.value = state.value.copy(notice = PremiumNotice.STORE_FAILED)
    }

    private fun noticeFor(error: Exception): PremiumNotice = when (error) {
        is PremiumStatusException.SignedOut -> PremiumNotice.DENIED
        is PremiumStatusException.Denied -> PremiumNotice.DENIED
        is PremiumStatusException.Unavailable -> PremiumNotice.OFFLINE
        is PremiumStatusException.Invalid -> PremiumNotice.ERROR
        is IOException -> PremiumNotice.OFFLINE
        else -> PremiumNotice.ERROR
    }
}
