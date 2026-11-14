package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.sync.EnterpriseSyncFlow
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.enterprise.sync.EnterpriseOperationIdentity
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncMode
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import javax.inject.Named
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.launch

/** Presentation ownership never acquires a LocalContext selection or stores a route selector. */
@HiltViewModel
class EnterpriseSyncViewModel @Inject constructor(
    private val flow: EnterpriseSyncFlow,
    @Named("enterpriseConnectivity") private val network: NetworkConnectivityGateway,
    private val fence: EnterpriseSessionFence,
) : ViewModel() {
    val state = flow.state
    val epochs = fence.epochs
    private val mutableConnected = MutableStateFlow(false)
    val connected = mutableConnected.asStateFlow()
    @Volatile private var closed = false

    init {
        viewModelScope.launch {
            network.observeIsConnected().catch { emit(false) }.collect { mutableConnected.value = it }
        }
        viewModelScope.launch { fence.epochs.collect { flow.invalidateIfStale() } }
        viewModelScope.launch { flow.activityChanges.collect { flow.onActivityChanged() } }
    }

    fun isCurrent(renderedEpoch: Long, generation: Long, allowBusy: Boolean = false): Boolean {
        val current = state.value
        return !closed && renderedEpoch == fence.epoch && generation == current.generation &&
            (current.epoch == null || current.epoch == renderedEpoch) && (allowBusy || !current.busy)
    }

    private fun action(renderedEpoch: Long, generation: Long, online: Boolean = false, work: suspend () -> Unit) {
        if (!isCurrent(renderedEpoch, generation) || online && !connected.value) return
        viewModelScope.launch {
            if (isCurrent(renderedEpoch, generation) && (!online || connected.value)) work()
        }
    }

    fun open(context: EnterpriseContextSummary, renderedEpoch: Long, generation: Long,
        mode: EnterpriseSyncMode = EnterpriseSyncMode.PENDING): Boolean {
        if (!isCurrent(renderedEpoch, generation)) return false
        action(renderedEpoch, generation) { flow.open(context, renderedEpoch, mode) }
        return true
    }
    fun refresh(renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.refresh(renderedEpoch, generation) }
    fun loadMore(renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.more(renderedEpoch, generation) }
    fun detail(identity: EnterpriseOperationIdentity, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.showDetail(identity.operationId, identity.payloadDigest, renderedEpoch, generation) }
    fun dismiss(identity: EnterpriseOperationIdentity, receiptDigest: String, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.dismiss(identity.operationId, identity.payloadDigest, receiptDigest, renderedEpoch, generation) }
    fun manualCheck(renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation, online = true) { flow.manualCheck(connected.value, renderedEpoch, generation) }
    fun changeMode(mode: EnterpriseSyncMode, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.changeMode(mode, renderedEpoch, generation) }
    fun backToList(renderedEpoch: Long, generation: Long) {
        if (isCurrent(renderedEpoch, generation, allowBusy = true)) flow.backToList(renderedEpoch, generation)
    }
    fun withdraw() { if (!closed) flow.withdraw() }
    fun invalidateIfStale() { if (!closed) flow.invalidateIfStale() }

    override fun onCleared() {
        closed = true
        flow.close()
        super.onCleared()
    }
}
