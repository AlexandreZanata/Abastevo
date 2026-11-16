package com.anpfuel.app.ui.enterprise.trip

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.app.ui.enterprise.tracking.TrackingServiceLauncher
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripFlow
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.meter.ReadingFlag
import com.anpfuel.domain.enterprise.tracking.TrackingDevice
import com.anpfuel.domain.enterprise.tracking.TrackingSessionState
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import com.anpfuel.domain.enterprise.trip.TripLogbookType
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import javax.inject.Named
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.launch

/** SCR-TRIP-01 presentation owner (EMP-TRIP-005); gestures carry the epoch and generation they rendered. */
@HiltViewModel
class TripViewModel @Inject constructor(
    private val flow: TripFlow,
    @Named("enterpriseConnectivity") private val network: NetworkConnectivityGateway,
    private val fence: EnterpriseSessionFence,
    activity: EnterpriseDeliveryActivity,
    private val tracking: TrackingServiceLauncher,
) : ViewModel() {
    /** Without the tracking service (screens built outside the Hilt graph): tracking writes still work, nothing listens. */
    constructor(flow: TripFlow, network: NetworkConnectivityGateway, fence: EnterpriseSessionFence, activity: EnterpriseDeliveryActivity) :
        this(flow, network, fence, activity, TrackingServiceLauncher.NONE)

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
        // A finished delivery may have stored new receipts: re-read the protected book (local only).
        viewModelScope.launch {
            activity.changes.drop(1).collect {
                val current = state.value
                if (!closed && current.context != null && !current.busy && current.epoch == fence.epoch) flow.reload(fence.epoch, current.generation)
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

    /** Reads the protected trip book of the selected organization; never a network call. */
    fun open(context: EnterpriseContextSummary, renderedEpoch: Long) {
        if (closed || renderedEpoch != fence.epoch) return
        viewModelScope.launch { flow.open(context, renderedEpoch) }
    }
    fun reload(renderedEpoch: Long, generation: Long) = action(renderedEpoch, generation) { flow.reload(renderedEpoch, generation) }
    fun start(choice: EnterpriseAssetChoice, label: TripAssetLabel, departure: TripCapture, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.start(choice, label, departure, renderedEpoch, generation) }
    fun log(tripId: String, type: TripLogbookType, text: String?, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.log(tripId, type, text, renderedEpoch, generation) }
    fun finish(tripId: String, final: TripCapture, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) {
            flow.finish(tripId, final, renderedEpoch, generation)
            // The session ended with the finish (B-BR-EMP-499): release the listener now instead of at the next watchdog tick.
            state.value.tracking?.session(tripId)?.takeIf { it.state == TrackingSessionState.ENDED }?.let { tracking.release(it.sessionId) }
        }
    fun markReady(tripId: String, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) { flow.markReady(tripId, renderedEpoch, generation) }
    /** "Enviar agora" exists only with a connection (12-ux-app §6). */
    fun sendNow(renderedEpoch: Long, generation: Long) = action(renderedEpoch, generation, online = true) { flow.sendNow(renderedEpoch, generation) }
    /**
     * EMP-GPS-002 (B-BR-EMP-496): the driver's tap with the permission and notification state observed on this visible
     * screen; only a session that is now recording arms the one-time grant and starts the service.
     */
    fun startTracking(tripId: String, device: TrackingDevice, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) {
            flow.startTracking(tripId, device, renderedEpoch, generation)
            val current = state.value
            val session = current.tracking?.session(tripId)
            val context = current.context
            if (!closed && context != null && session != null && session.state == TrackingSessionState.RECORDING && !session.refused) {
                tracking.launch(context.partition, session.sessionId, context.organizationName)
            }
        }
    /** "Parar rastreamento" on the trip screen: the interruption is written first, then the listener is released. */
    fun stopTracking(tripId: String, renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation) {
            val session = state.value.tracking?.session(tripId)?.sessionId
            flow.stopTracking(tripId, renderedEpoch, generation)
            session?.let(tracking::release)
        }
    /** Online only: the policy in force for this member (B-BR-EMP-495). */
    fun refreshTrackingPolicy(renderedEpoch: Long, generation: Long) =
        action(renderedEpoch, generation, online = true) { flow.refreshTrackingPolicy(renderedEpoch, generation) }
    fun openTrip(tripId: String?, renderedEpoch: Long, generation: Long) {
        if (isCurrent(renderedEpoch, generation)) flow.openTrip(tripId, renderedEpoch, generation)
    }
    fun finishWarnings(tripId: String, final: TripCapture): List<ReadingFlag> = if (closed) emptyList() else flow.finishWarnings(tripId, final)
    fun withdraw() { if (!closed) flow.withdraw() }
    fun invalidateIfStale() { if (!closed) flow.invalidateIfStale() }

    override fun onCleared() {
        closed = true
        flow.close()
        super.onCleared()
    }
}
