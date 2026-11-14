package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.enterprise.EnterpriseContextFlow
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.port.NetworkConnectivityGateway
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import javax.inject.Named
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.launch

/** Only source-confirmed presentation metadata; no saved context or private store handle. */
@HiltViewModel
class EnterpriseContextViewModel @Inject constructor(
    private val flow: EnterpriseContextFlow,
    @Named("enterpriseConnectivity") network: NetworkConnectivityGateway,
    fence: EnterpriseSessionFence,
) : ViewModel() {
    val state = flow.state
    val epochs = fence.epochs
    private val mutableConnected = MutableStateFlow(false)
    val connected = mutableConnected.asStateFlow()

    init {
        viewModelScope.launch {
            var initialRequested = false
            network.observeIsConnected().catch { emit(false) }.collect { online ->
                mutableConnected.value = online
                if (!online) flow.unconfirm()
                else if (!initialRequested) {
                    initialRequested = true
                    refresh()
                }
            }
        }
        viewModelScope.launch { fence.epochs.collect { flow.invalidateIfStale() } }
    }

    private fun sourceAction(renderedEpoch: Long, action: suspend () -> Unit) {
        if (connected.value && !state.value.busy && renderedEpoch == epochs.value) viewModelScope.launch {
            if (connected.value && renderedEpoch == epochs.value) action()
        }
    }

    fun refresh(renderedEpoch: Long = epochs.value) = sourceAction(renderedEpoch) { flow.refresh() }
    fun loadMore(renderedEpoch: Long = epochs.value) = sourceAction(renderedEpoch) { flow.loadMore() }
    fun select(context: EnterpriseContextSummary, renderedEpoch: Long = epochs.value, acknowledgeErasedWork: Boolean = false) =
        sourceAction(renderedEpoch) { flow.select(context, acknowledgeErasedWork) }

    fun selectPersonal() {
        // Withdraw handles now, before a queued coroutine can wait on local I/O.
        flow.unconfirm()
        viewModelScope.launch { flow.selectPersonal() }
    }

    fun unconfirm() = flow.unconfirm()
    fun invalidateIfStale() = flow.invalidateIfStale()

    override fun onCleared() {
        flow.close()
        super.onCleared()
    }
}
