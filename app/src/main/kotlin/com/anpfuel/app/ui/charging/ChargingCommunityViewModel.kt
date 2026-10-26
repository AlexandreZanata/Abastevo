package com.anpfuel.app.ui.charging

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.charging.ChargingCommunitySnapshot
import com.anpfuel.application.usecase.charging.ChargingCommunityUseCase
import com.anpfuel.application.usecase.charging.ChargingSaveOutcome
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.domain.charging.*
import com.anpfuel.domain.charging.ChargingCondition
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingObservationDraft
import com.anpfuel.domain.charging.ChargingObservationException
import com.anpfuel.domain.charging.ChargingReportReason
import com.anpfuel.domain.charging.ChargingVote
import com.anpfuel.domain.charging.CommunityChargingObservation
import com.anpfuel.domain.charging.OwnedChargingObservation
import com.anpfuel.domain.charging.PendingChargingOperation
import dagger.hilt.android.lifecycle.HiltViewModel
import java.math.BigDecimal
import java.time.LocalDate
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class ChargingCommunityForm(
    val connector: ChargingConnector = ChargingConnector.TYPE2,
    val condition: ChargingCondition = ChargingCondition.STANDARD,
    val day: String = LocalDate.now().toString(),
    val price: String = "",
    val note: String = "",
    val editingId: String? = null,
    val event: ChargingOperationalStatus? = null,
    val scope: ChargingTargetScope = ChargingTargetScope.SITE,
    val groupId: String? = null,
    val eventTime: String = java.time.Instant.now().toString(),
)

data class ChargingCommunityUiState(
    val loading: Boolean = true,
    val groups: List<ChargingConnectionGroup> = emptyList(),
    val saving: Boolean = false,
    val offline: Boolean = false,
    val signedOut: Boolean = false,
    val history: List<CommunityChargingObservation> = emptyList(),
    val mine: List<OwnedChargingObservation> = emptyList(),
    val pending: List<PendingChargingOperation> = emptyList(),
    val form: ChargingCommunityForm = ChargingCommunityForm(),
    val formError: String? = null,
    val notice: String? = null,
    val conflict: Boolean = false,
)

/**
 * EE-22 community electric journeys. The form writes through the durable
 * pending queue before any network call; a lost reply/restart retries the
 * same observation id. Votes/reports require the current owner and never
 * act on the owner's own report. Late account/generation results cannot
 * render another account's private receipts.
 */
@HiltViewModel
class ChargingCommunityViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val community: ChargingCommunityUseCase,
    private val connectivity: ObserveNetworkConnectivityUseCase,
    private val sessions: AuthSessionStore,
) : ViewModel() {
    private val placeId: String = checkNotNull(savedStateHandle["placeId"])
    private val state = MutableStateFlow(ChargingCommunityUiState())
    val uiState = state.asStateFlow()
    private var generation = 0L

    init {
        viewModelScope.launch {
            runCatching { connectivity().collect { connected ->
                state.update { it.copy(offline = !connected) }
                if (connected) { community.retryPending(); refresh() }
            } }
        }
        refresh()
    }

    fun refresh() {
        val owner = sessions.load()?.accountId
        val request = ++generation
        state.update { it.copy(mine = emptyList(), pending = emptyList(), signedOut = owner == null, form = if (owner == null) ChargingCommunityForm() else it.form) }
        state.update { it.copy(loading = true) }
        viewModelScope.launch {
            try {
                val snapshot = community.snapshot(placeId)
                if (request != generation || sessions.load()?.accountId != owner) return@launch
                val groups = try { community.targets(placeId) } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { state.value.groups }
                if (request != generation || sessions.load()?.accountId != owner) return@launch
                state.update { it.copy(groups = groups) }
                apply(snapshot)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (error: Exception) {
                if (request == generation) {
                    state.update { it.copy(loading = false, offline = error is java.io.IOException, notice = "error") }
                }
            }
        }
    }

    private fun apply(snapshot: ChargingCommunitySnapshot) {
        state.update {
            it.copy(
                loading = false,
                history = snapshot.history,
                mine = snapshot.mine,
                pending = snapshot.pending,
                offline = it.offline || snapshot.historyUnavailable,
                signedOut = sessions.load() == null,
            )
        }
    }

    fun setConnector(connector: ChargingConnector) = state.update { it.copy(form = it.form.copy(connector = connector)) }
    fun setCondition(condition: ChargingCondition) = state.update { it.copy(form = it.form.copy(condition = condition)) }
    fun setDay(day: String) = state.update { it.copy(form = it.form.copy(day = day)) }
    fun setPrice(price: String) = state.update { it.copy(form = it.form.copy(price = price)) }
    fun setNote(note: String) = state.update { it.copy(form = it.form.copy(note = note)) }

    fun setEvent(event: ChargingOperationalStatus?) = state.update { it.copy(form = it.form.copy(event = event)) }
    fun setEventTime(value: String) = state.update { it.copy(form = it.form.copy(eventTime = value)) }
    fun setScope(scope: ChargingTargetScope) = state.update { it.copy(form = it.form.copy(scope = scope, groupId = null)) }
    fun setGroup(group: ChargingConnectionGroup) = state.update { it.copy(form = it.form.copy(groupId = group.id, connector = group.connector)) }
    fun discard(id: String) { viewModelScope.launch { community.discard(id); refresh() } }
    fun retry() { viewModelScope.launch { community.retryPending(); refresh() } }

    fun edit(observation: OwnedChargingObservation) {
        state.update {
            it.copy(
                form = ChargingCommunityForm(
                    connector = observation.connector,
                    condition = observation.condition,
                    day = observation.observedDay,
                    price = observation.priceMilliBrlPerKwh?.let { milli -> BigDecimal(milli).movePointLeft(3).stripTrailingZeros().toPlainString() }.orEmpty(),
                    note = observation.note,
                    editingId = observation.id,
                    event = observation.operational?.event,
                    scope = observation.operational?.targetScope ?: ChargingTargetScope.SITE,
                    groupId = observation.operational?.connectionGroupId,
                    eventTime = observation.operational?.observedAt ?: java.time.Instant.now().toString(),
                ),
                formError = null,
            )
        }
    }

    fun cancelEdit() = state.update { it.copy(form = ChargingCommunityForm(), formError = null) }

    fun save() {
        if (state.value.saving) return
        if (state.value.signedOut) {
            state.update { it.copy(notice = "sign-in") }
            return
        }
        val form = state.value.form
        val price = parsePrice(form.price)
        if (price is PriceParse.Invalid) {
            state.update { it.copy(formError = "price") }
            return
        }
        val draft = try {
            ChargingObservationDraft(
                placeId = placeId,
                connector = form.connector,
                priceMilliBrlPerKwh = (price as? PriceParse.Known)?.milli,
                condition = form.condition,
                observedDay = form.day,
                note = form.note,
                operational = form.event?.let { ChargingOperationalEvent(it, form.scope, if (form.scope == ChargingTargetScope.SITE) null else form.groupId, form.eventTime) },
            )
        } catch (_: Exception) {
            state.update { it.copy(formError = "form") }
            return
        }
        val editing = state.value.mine.firstOrNull { it.id == form.editingId }
        state.update { it.copy(saving = true, formError = null, notice = null, conflict = false) }
        viewModelScope.launch {
            try {
                val outcome = community.save(draft, editing)
                if (outcome is ChargingSaveOutcome.SignedOut) {
                    state.update { it.copy(saving = false, signedOut = true, notice = "sign-in") }
                    return@launch
                }
                state.update {
                    it.copy(
                        saving = false,
                        pending = community.pendingForCurrentOwner(),
                        conflict = outcome is ChargingSaveOutcome.Conflict,
                        offline = outcome is ChargingSaveOutcome.Offline,
                        notice = when (outcome) {
                            is ChargingSaveOutcome.Offline -> "offline"
                            is ChargingSaveOutcome.Conflict -> "conflict"
                            is ChargingSaveOutcome.Refused -> if (outcome.message == "queue-full") "queue-full" else "refused"
                            else -> "saved"
                        },
                        form = if (outcome is ChargingSaveOutcome.Saved) ChargingCommunityForm() else it.form,
                    )
                }
                if (outcome is ChargingSaveOutcome.Saved) refresh()
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                state.update { it.copy(saving = false, notice = "offline", offline = true) }
            }
        }
    }

    fun withdraw(observation: OwnedChargingObservation) {
        if (state.value.saving) return
        state.update { it.copy(saving = true, notice = null) }
        viewModelScope.launch {
            try {
                val outcome = community.withdraw(observation)
                state.update {
                    it.copy(
                        saving = false,
                        pending = community.pendingForCurrentOwner(),
                        notice = if (outcome is ChargingSaveOutcome.Saved) "withdrawn" else "pending",
                        conflict = outcome is ChargingSaveOutcome.Conflict,
                    )
                }
                if (outcome is ChargingSaveOutcome.Saved) refresh()
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                state.update { it.copy(saving = false, notice = "offline", offline = true) }
            }
        }
    }

    fun vote(observation: CommunityChargingObservation, choice: ChargingVote) {
        viewModelScope.launch {
            try {
                val tally = community.vote(observation.id, choice)
                state.update { current ->
                    current.copy(
                        history = current.history.map {
                            if (it.id == observation.id) {
                                it.copy(helpful = tally.helpful, notHelpful = tally.notHelpful)
                            } else {
                                it
                            }
                        },
                        notice = "voted",
                    )
                }
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (error: Exception) {
                state.update {
                    it.copy(
                        signedOut = error is ChargingObservationException.SignedOut,
                        notice = if (error is ChargingObservationException.Denied) "self" else "offline",
                    )
                }
            }
        }
    }

    fun report(observation: CommunityChargingObservation, reason: ChargingReportReason) {
        viewModelScope.launch {
            try {
                community.report(observation.id, reason, "")
                state.update { it.copy(notice = "reported") }
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (error: Exception) {
                state.update {
                    it.copy(
                        signedOut = error is ChargingObservationException.SignedOut,
                        notice = if (error is ChargingObservationException.Denied) "self" else "offline",
                    )
                }
            }
        }
    }

    fun dismissNotice() = state.update { it.copy(notice = null, formError = null) }

    private sealed interface PriceParse {
        data class Known(val milli: Long) : PriceParse
        data object Empty : PriceParse
        data object Invalid : PriceParse
    }

    private fun parsePrice(raw: String): PriceParse {
        if (raw.isBlank()) return PriceParse.Empty
        return try {
            val normalized = raw.trim().replace(" ", "").replace(',', '.')
            val value = BigDecimal(normalized)
            if (value < BigDecimal.ZERO || value > BigDecimal("10000")) return PriceParse.Invalid
            PriceParse.Known(value.movePointRight(3).setScale(0).longValueExact())
        } catch (_: Exception) {
            PriceParse.Invalid
        }
    }
}
