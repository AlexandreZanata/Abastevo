package com.anpfuel.app.ui.contributors

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.contributor.OpenClaimUseCase
import com.anpfuel.application.usecase.contributor.ReadOwnClaimsUseCase
import com.anpfuel.application.usecase.contributor.SubmitClaimProofUseCase
import com.anpfuel.application.usecase.contributor.WithdrawClaimUseCase
import com.anpfuel.domain.contributor.Claim
import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.IssuedClaim
import dagger.hilt.android.lifecycle.HiltViewModel
import java.io.IOException
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Honest claim flow states; the server owns quotas and decisions. */
enum class ClaimNotice { OFFLINE, DENIED, ERROR, QUOTA, CONFLICT, INVALID }

data class ClaimUiState(
    val loading: Boolean = true,
    val claims: List<Claim> = emptyList(),
    val issued: IssuedClaim? = null,
    val notice: ClaimNotice? = null,
    val working: Boolean = false,
)

/**
 * B-BR-CTB claim self-service (CTB-02/03/04), signed-in only.
 *
 * Late responses are fenced by generation. The issued one-time code stays
 * in state until dismissed; it is never logged or persisted by the client.
 */
@HiltViewModel
class ClaimViewModel @Inject constructor(
    private val openClaim: OpenClaimUseCase,
    private val readClaims: ReadOwnClaimsUseCase,
    private val submitProof: SubmitClaimProofUseCase,
    private val withdrawClaim: WithdrawClaimUseCase,
) : ViewModel() {
    private val state = MutableStateFlow(ClaimUiState())
    val uiState = state.asStateFlow()
    private var generation = 0L

    fun refresh() {
        val request = ++generation
        state.value = ClaimUiState(loading = true, issued = state.value.issued)
        viewModelScope.launch {
            try {
                val claims = readClaims()
                if (request != generation) return@launch
                state.value = ClaimUiState(loading = false, claims = claims, issued = state.value.issued)
            } catch (e: Exception) {
                if (request != generation) return@launch
                state.value = ClaimUiState(loading = false, notice = noticeOf(e))
            }
        }
    }

    fun open(login: String) {
        val request = ++generation
        state.value = state.value.copy(working = true, notice = null)
        viewModelScope.launch {
            try {
                val issued = openClaim(login.trim())
                if (request != generation) return@launch
                val claims = readClaims()
                if (request != generation) return@launch
                state.value = ClaimUiState(loading = false, claims = claims, issued = issued)
            } catch (e: Exception) {
                if (request != generation) return@launch
                state.value = state.value.copy(working = false, notice = noticeOf(e))
            }
        }
    }

    fun prove(claimId: String, gistId: String) {
        mutate(claimId) { submitProof(claimId, gistId.trim()) }
    }

    fun withdraw(claimId: String) {
        mutate(claimId) { withdrawClaim(claimId) }
    }

    private fun mutate(claimId: String, block: suspend () -> Claim) {
        val request = ++generation
        state.value = state.value.copy(working = true, notice = null)
        viewModelScope.launch {
            try {
                block()
                if (request != generation) return@launch
                val claims = readClaims()
                if (request != generation) return@launch
                state.value = state.value.copy(working = false, claims = claims)
            } catch (e: Exception) {
                if (request != generation) return@launch
                state.value = state.value.copy(working = false, notice = noticeOf(e))
            }
        }
    }

    fun dismissIssued() {
        state.value = state.value.copy(issued = null)
    }

    private fun noticeOf(e: Exception): ClaimNotice = when (e) {
        is ContributorException.Offline, is IOException -> ClaimNotice.OFFLINE
        is ContributorException.Denied -> ClaimNotice.DENIED
        is ContributorException.Quota -> ClaimNotice.QUOTA
        is ContributorException.Conflict -> ClaimNotice.CONFLICT
        is ContributorException.Invalid -> ClaimNotice.INVALID
        else -> ClaimNotice.ERROR
    }
}
