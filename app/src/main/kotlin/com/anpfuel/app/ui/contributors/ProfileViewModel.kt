package com.anpfuel.app.ui.contributors

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.contributor.ReadOwnProfileUseCase
import com.anpfuel.application.usecase.contributor.UpsertProfileUseCase
import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.ProfileEdit
import com.anpfuel.domain.contributor.ProfileRead
import dagger.hilt.android.lifecycle.HiltViewModel
import java.io.IOException
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Honest profile states; 403 without the capability is its own message. */
enum class ProfileNotice { OFFLINE, DENIED, ERROR, QUOTA, CONFLICT, FORBIDDEN }

data class ProfileUiState(
    val loading: Boolean = true,
    val read: ProfileRead? = null,
    val notice: ProfileNotice? = null,
    val saving: Boolean = false,
    val saved: Boolean = false,
)

/**
 * B-BR-CTB profile self-service (CTB-08/12), signed-in only.
 *
 * Client validation mirrors the server bounds; the server stays
 * authoritative and its codes surface through [notice].
 */
@HiltViewModel
class ProfileViewModel @Inject constructor(
    private val readProfile: ReadOwnProfileUseCase,
    private val upsertProfile: UpsertProfileUseCase,
) : ViewModel() {
    private val state = MutableStateFlow(ProfileUiState())
    val uiState = state.asStateFlow()
    private var generation = 0L

    fun refresh() {
        val request = ++generation
        state.value = ProfileUiState(loading = true)
        viewModelScope.launch {
            try {
                val read = readProfile()
                if (request != generation) return@launch
                state.value = ProfileUiState(loading = false, read = read)
            } catch (e: Exception) {
                if (request != generation) return@launch
                state.value = ProfileUiState(loading = false, notice = noticeOf(e))
            }
        }
    }

    fun save(edit: ProfileEdit) {
        val request = ++generation
        state.value = state.value.copy(saving = true, notice = null, saved = false)
        viewModelScope.launch {
            try {
                val read = upsertProfile(edit)
                if (request != generation) return@launch
                state.value = state.value.copy(saving = false, read = read, saved = true)
            } catch (e: Exception) {
                if (request != generation) return@launch
                state.value = state.value.copy(saving = false, notice = noticeOf(e))
            }
        }
    }

    private fun noticeOf(e: Exception): ProfileNotice = when (e) {
        is ContributorException.Offline, is IOException -> ProfileNotice.OFFLINE
        is ContributorException.Denied -> ProfileNotice.DENIED
        is ContributorException.Quota -> ProfileNotice.QUOTA
        is ContributorException.Conflict -> ProfileNotice.CONFLICT
        is ContributorException.Forbidden -> ProfileNotice.FORBIDDEN
        else -> ProfileNotice.ERROR
    }
}
