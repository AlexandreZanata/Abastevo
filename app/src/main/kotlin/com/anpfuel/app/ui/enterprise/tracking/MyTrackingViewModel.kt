package com.anpfuel.app.ui.enterprise.tracking

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.tracking.MyTrackingFlow
import com.anpfuel.application.enterprise.tracking.TrackingActivity
import com.anpfuel.application.enterprise.tracking.TrackingRuntime
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/**
 * EMP-GPS-015 presentation owner (B-BR-EMP-548..550): the indicator of every enterprise screen, from the process's running
 * organization collector only, and "Meu rastreamento" (SCR-GPS-04) over the selected partition's sealed records.
 */
@HiltViewModel
class MyTrackingViewModel @Inject constructor(
    private val flow: MyTrackingFlow,
    private val fence: EnterpriseSessionFence,
    runtime: TrackingRuntime,
) : ViewModel() {
    val state = flow.state
    /** Never the personal recorder and never a stored session alone: only a listener of this process (EDR-357). */
    val activity: StateFlow<TrackingActivity?> = runtime.activity.stateIn(viewModelScope, SharingStarted.Eagerly, null)
    @Volatile private var closed = false

    init { viewModelScope.launch { fence.epochs.collect { flow.invalidateIfStale() } } }

    /** Reads the sealed tracking records of the selected organization; never a network call. */
    fun open(context: EnterpriseContextSummary, renderedEpoch: Long) {
        if (closed || renderedEpoch != fence.epoch) return
        viewModelScope.launch { flow.open(context, renderedEpoch) }
    }
    fun reload(renderedEpoch: Long, generation: Long) {
        if (closed || renderedEpoch != fence.epoch) return
        viewModelScope.launch { flow.reload(renderedEpoch, generation) }
    }
    fun withdraw() { if (!closed) flow.withdraw() }

    override fun onCleared() {
        closed = true
        flow.close()
        super.onCleared()
    }
}
