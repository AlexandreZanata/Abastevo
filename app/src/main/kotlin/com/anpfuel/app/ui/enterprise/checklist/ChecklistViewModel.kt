package com.anpfuel.app.ui.enterprise.checklist

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.checklist.ChecklistFlow
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistContext
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import javax.inject.Named
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.launch

/**
 * The checklist execution owner (EMP-CHECK-002, B-BR-EMP-443); gestures carry the epoch and generation they rendered.
 * Answers and completions are local writes; the company is read only with a connection.
 */
@HiltViewModel
class ChecklistViewModel @Inject constructor(
    private val flow: ChecklistFlow,
    @Named("enterpriseConnectivity") private val network: NetworkConnectivityGateway,
    private val fence: EnterpriseSessionFence,
    activity: EnterpriseDeliveryActivity,
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
        // A finished delivery may have stored new receipts: re-read the book and, when connected, the company's result.
        viewModelScope.launch {
            activity.changes.drop(1).collect {
                val current = state.value
                if (!closed && current.context != null && !current.busy && current.epoch == fence.epoch) {
                    if (connected.value) flow.refresh(fence.epoch, current.generation) else flow.reload(fence.epoch, current.generation)
                }
            }
        }
    }

    fun isCurrent(renderedEpoch: Long, generation: Long): Boolean {
        val current = state.value
        return !closed && renderedEpoch == fence.epoch && generation == current.generation &&
            (current.epoch == null || current.epoch == renderedEpoch)
    }

    private fun action(renderedEpoch: Long, generation: Long, online: Boolean = false, work: suspend () -> Unit) {
        if (!isCurrent(renderedEpoch, generation) || state.value.busy || online && !connected.value) return
        viewModelScope.launch { if (isCurrent(renderedEpoch, generation) && (!online || connected.value)) work() }
    }

    /** Reads the protected book for the asset and moment; with a connection the cached template is then confirmed again. */
    fun open(context: EnterpriseContextSummary, assetId: String, checklistContext: ChecklistContext, renderedEpoch: Long) {
        if (closed || renderedEpoch != fence.epoch) return
        viewModelScope.launch {
            flow.open(context, assetId, checklistContext, renderedEpoch)
            val current = state.value
            if (!closed && connected.value && !current.busy && current.epoch == renderedEpoch && fence.epoch == renderedEpoch &&
                current.assetId == assetId && current.checklistContext == checklistContext) flow.refresh(renderedEpoch, current.generation)
        }
    }
    fun refresh(renderedEpoch: Long, generation: Long) = action(renderedEpoch, generation, online = true) { flow.refresh(renderedEpoch, generation) }
    fun start(renderedEpoch: Long, generation: Long) = action(renderedEpoch, generation) { flow.start(renderedEpoch, generation) }
    fun answer(runId: String, position: Int, answer: String, note: String?, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.answer(runId, position, answer, note, renderedEpoch, generation) }
    fun complete(runId: String, acknowledged: Boolean, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.complete(runId, acknowledged, renderedEpoch, generation) }
    /** "Enviar agora" exists only with a connection (12-ux-app §6). */
    fun sendNow(renderedEpoch: Long, generation: Long) = action(renderedEpoch, generation, online = true) { flow.sendNow(renderedEpoch, generation) }
    fun withdraw() { if (!closed) flow.withdraw() }
    fun invalidateIfStale() { if (!closed) flow.invalidateIfStale() }

    override fun onCleared() {
        closed = true
        flow.close()
        super.onCleared()
    }
}
