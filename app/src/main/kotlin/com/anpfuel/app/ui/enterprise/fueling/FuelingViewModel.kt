package com.anpfuel.app.ui.enterprise.fueling

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.attachment.ReceiptDraft
import com.anpfuel.application.enterprise.fueling.FuelingFlow
import com.anpfuel.application.enterprise.fueling.FuelingForm
import com.anpfuel.application.enterprise.fueling.FuelingWarnings
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import javax.inject.Named
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.launch

/** SCR-FUEL-01/02 presentation owner (EMP-FUEL-011); gestures carry the epoch and generation they rendered. */
@HiltViewModel
class FuelingViewModel @Inject constructor(
    private val flow: FuelingFlow,
    @Named("enterpriseConnectivity") private val network: NetworkConnectivityGateway,
    private val fence: EnterpriseSessionFence,
    activity: EnterpriseDeliveryActivity,
) : ViewModel() {
    val state = flow.state
    private val mutableConnected = MutableStateFlow(false)
    val connected = mutableConnected.asStateFlow()
    @Volatile private var closed = false

    init {
        viewModelScope.launch { network.observeIsConnected().catch { emit(false) }.collect { mutableConnected.value = it } }
        viewModelScope.launch { fence.epochs.collect { flow.invalidateIfStale() } }
        // A finished delivery may have stored new receipts: re-read the protected queue (local only).
        viewModelScope.launch {
            activity.changes.drop(1).collect {
                val current = state.value
                if (!closed && current.context != null && !current.busy && current.epoch == fence.epoch) flow.reload(fence.epoch, current.generation)
            }
        }
    }

    fun isCurrent(renderedEpoch: Long, generation: Long): Boolean {
        val current = state.value
        return !closed && renderedEpoch == fence.epoch && generation == current.generation && (current.epoch == null || current.epoch == renderedEpoch)
    }

    private fun action(renderedEpoch: Long, generation: Long, online: Boolean = false, work: suspend () -> Unit) {
        if (!isCurrent(renderedEpoch, generation) || state.value.busy || online && !connected.value) return
        viewModelScope.launch { if (isCurrent(renderedEpoch, generation) && (!online || connected.value)) work() }
    }

    /** Reads the protected fuelings of the selected organization; never a network call. */
    fun open(context: EnterpriseContextSummary, renderedEpoch: Long) {
        if (closed || renderedEpoch != fence.epoch) return
        viewModelScope.launch { flow.open(context, renderedEpoch) }
    }
    fun reload(renderedEpoch: Long, generation: Long) = action(renderedEpoch, generation) { flow.reload(renderedEpoch, generation) }
    /** Saves offline; the optional receipt follows in its own protected write (EDR-297). */
    fun record(choice: EnterpriseAssetChoice, form: FuelingForm, receipt: ReceiptDraft?, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.record(choice, form, receipt, renderedEpoch, generation) }
    fun attach(fuelingId: String, draft: ReceiptDraft, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.attach(fuelingId, draft, renderedEpoch, generation) }
    /** "Enviar agora" exists only with a connection (12-ux-app §6). */
    fun sendNow(renderedEpoch: Long, generation: Long) = action(renderedEpoch, generation, online = true) { flow.sendNow(renderedEpoch, generation) }
    fun openFueling(fuelingId: String?, renderedEpoch: Long, generation: Long) {
        if (isCurrent(renderedEpoch, generation)) flow.openFueling(fuelingId, renderedEpoch, generation)
    }
    fun warnings(form: FuelingForm): FuelingWarnings = if (closed) FuelingWarnings(false, emptyList()) else flow.warnings(form)
    fun withdraw() { if (!closed) flow.withdraw() }
    fun invalidateIfStale() { if (!closed) flow.invalidateIfStale() }

    override fun onCleared() {
        closed = true
        flow.close()
        super.onCleared()
    }
}
