package com.anpfuel.app.ui.enterprise.expense

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.attachment.ReceiptDraft
import com.anpfuel.application.enterprise.expense.ExpenseFlow
import com.anpfuel.application.enterprise.expense.ExpenseForm
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

/** SCR-EXP-01 presentation owner (EMP-EXP-004); gestures carry the epoch and generation they rendered. */
@HiltViewModel
class ExpenseViewModel @Inject constructor(
    private val flow: ExpenseFlow,
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

    /** Reads the protected expenses of the selected organization; never a network call. */
    fun open(context: EnterpriseContextSummary, renderedEpoch: Long) {
        if (closed || renderedEpoch != fence.epoch) return
        viewModelScope.launch { flow.open(context, renderedEpoch) }
    }
    fun reload(renderedEpoch: Long, generation: Long) = action(renderedEpoch, generation) { flow.reload(renderedEpoch, generation) }
    /** Saves offline; the optional receipt follows in its own protected write (EDR-338). */
    fun record(choice: EnterpriseAssetChoice, form: ExpenseForm, receipt: ReceiptDraft?, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.record(choice, form, receipt, renderedEpoch, generation) }
    fun attach(expenseId: String, draft: ReceiptDraft, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.attach(expenseId, draft, renderedEpoch, generation) }
    /** "Enviar agora" exists only with a connection (12-ux-app §6). */
    fun sendNow(renderedEpoch: Long, generation: Long) = action(renderedEpoch, generation, online = true) { flow.sendNow(renderedEpoch, generation) }
    fun openExpense(expenseId: String?, renderedEpoch: Long, generation: Long) {
        if (isCurrent(renderedEpoch, generation)) flow.openExpense(expenseId, renderedEpoch, generation)
    }
    fun withdraw() { if (!closed) flow.withdraw() }
    fun invalidateIfStale() { if (!closed) flow.invalidateIfStale() }

    override fun onCleared() {
        closed = true
        flow.close()
        super.onCleared()
    }
}
