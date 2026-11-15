package com.anpfuel.app.ui.enterprise.fleet

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.fleet.MyAssetsFlow
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import javax.inject.Named
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.launch

/** "Meus ativos" presentation owner (EMP-FLEET-013); gestures carry the epoch and generation they rendered. */
@HiltViewModel
class MyAssetsViewModel @Inject constructor(
    private val flow: MyAssetsFlow,
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
    }

    fun isCurrent(renderedEpoch: Long, generation: Long): Boolean {
        val current = state.value
        return !closed && renderedEpoch == fence.epoch && generation == current.generation &&
            (current.epoch == null || current.epoch == renderedEpoch)
    }

    private fun action(renderedEpoch: Long, generation: Long, online: Boolean = false, work: suspend () -> Unit) {
        if (!isCurrent(renderedEpoch, generation) || state.value.busy || online && !connected.value) return
        viewModelScope.launch {
            if (isCurrent(renderedEpoch, generation) && (!online || connected.value)) work()
        }
    }

    /** Reads the protected list at once; pulls only when connected (B-BR-EMP-351). */
    fun open(context: EnterpriseContextSummary, renderedEpoch: Long) {
        if (closed || renderedEpoch != fence.epoch) return
        val online = connected.value
        viewModelScope.launch { flow.open(context, renderedEpoch, online) }
    }
    fun refresh(renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation, online = true) { flow.refresh(renderedEpoch, generation) }
    fun detail(assetId: String, renderedEpoch: Long, generation: Long) {
        if (isCurrent(renderedEpoch, generation)) flow.detail(assetId, renderedEpoch, generation)
    }
    fun backToList(renderedEpoch: Long, generation: Long) {
        if (isCurrent(renderedEpoch, generation)) flow.backToList(renderedEpoch, generation)
    }
    fun represent(renderedEpoch: Long, generation: Long) {
        if (isCurrent(renderedEpoch, generation)) flow.represent(renderedEpoch, generation)
    }
    /** No network: the calling screen receives the hint synchronously (B-BR-EMP-352). */
    fun choose(assetId: String, warningAccepted: Boolean, renderedEpoch: Long, generation: Long): EnterpriseAssetChoice? =
        if (isCurrent(renderedEpoch, generation)) flow.choose(assetId, warningAccepted, renderedEpoch, generation) else null
    fun withdraw() { if (!closed) flow.withdraw() }
    fun invalidateIfStale() { if (!closed) flow.invalidateIfStale() }

    override fun onCleared() {
        closed = true
        flow.close()
        super.onCleared()
    }
}
