package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.enterprise.EnterpriseInvitationFlow
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.port.NetworkConnectivityGateway
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import javax.inject.Named
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.launch

/** Presentation contains no invitation code, credential or saved state. */
@HiltViewModel
class EnterpriseInvitationViewModel @Inject constructor(
    private val flow: EnterpriseInvitationFlow,
    @Named("enterpriseConnectivity") network: NetworkConnectivityGateway,
    fence: EnterpriseSessionFence,
) : ViewModel() {
    val state = flow.state
    val epochs = fence.epochs
    private val _connected = MutableStateFlow(false)
    val connected = _connected.asStateFlow()

    init {
        viewModelScope.launch {
            network.observeIsConnected().catch { emit(false) }.collect {
                _connected.value = it
                if (!it) flow.restart()
            }
        }
        viewModelScope.launch { fence.epochs.collect { flow.invalidateIfStale() } }
    }

    fun preview(code: String, inputEpoch: Long = epochs.value) {
        if (_connected.value && !state.value.busy && inputEpoch == epochs.value) viewModelScope.launch {
            // A queued UI action must not adopt a later account/family ceremony.
            if (inputEpoch == epochs.value) flow.preview(code)
        }
    }

    fun accept(consented: Boolean) {
        if (_connected.value && consented && !state.value.busy) viewModelScope.launch { flow.accept(consented) }
    }

    fun decline() {
        if (_connected.value && !state.value.busy) viewModelScope.launch { flow.decline() }
    }

    fun restart() = flow.restart()
    fun invalidateIfStale() = flow.invalidateIfStale()

    override fun onCleared() {
        flow.restart()
        super.onCleared()
    }
}
