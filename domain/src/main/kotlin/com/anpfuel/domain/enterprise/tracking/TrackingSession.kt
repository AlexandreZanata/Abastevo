package com.anpfuel.domain.enterprise.tracking

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.trip.MembershipRef
import com.anpfuel.domain.enterprise.trip.TripRef
import com.anpfuel.domain.enterprise.trip.TripState
import java.time.Instant

private fun failure(kind: TrackingFailure): TrackingDomainException = TrackingDomainException(kind)

/** A malformed organization is a scope failure; a malformed identity is invalid. */
private fun requireScope(raw: String): String =
    try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw failure(TrackingFailure.SCOPE) }

/** An absent reference is invalid; a constructed one must belong to the scope. */
private fun <T : Any> owned(scope: String, reference: T?, organizationId: (T) -> String): T {
    if (reference == null) throw invalid()
    if (organizationId(reference) != scope) throw failure(TrackingFailure.SCOPE)
    return reference
}

/** The unset instant (Go's zero time) and anything outside years 1..9999 are refused. */
private val unset: Instant = Instant.parse("0001-01-01T00:00:00Z")
private val maximum: Instant = Instant.parse("9999-12-31T23:59:59.999999999Z")
private fun requireInstant(at: Instant?): Instant {
    if (at == null || at <= unset || at > maximum) throw invalid()
    return at
}

/** A scoped session identity; its shape proves no existence. */
class TrackingSessionRef(val organizationId: String, val id: String) {
    init {
        requireScope(organizationId)
        try { EnterprisePartition.requireId(id) } catch (_: IllegalArgumentException) { throw invalid() }
    }
    override fun equals(other: Any?): Boolean = other is TrackingSessionRef && organizationId == other.organizationId && id == other.id
    override fun hashCode(): Int = 31 * organizationId.hashCode() + id.hashCode()
    override fun toString(): String = "EnterpriseTrackingRef(redacted)"
}

/** The bound trip's current facts, supplied with every command; the session stores no trip state. */
data class TrackingTripFacts(val reference: TripRef? = null, val driver: MembershipRef? = null, val state: TripState? = null) {
    override fun toString(): String = "EnterpriseTrackingTripFacts(redacted)"

    internal fun check(scope: String): Triple<TripRef, MembershipRef, TripState> =
        Triple(owned(scope, reference) { it.organizationId }, owned(scope, driver) { it.organizationId }, state ?: throw invalid())
}

/** What the device reports at a start or resume. */
data class TrackingDevice(val permission: LocationPermission? = null, val notificationVisible: Boolean = false) {
    override fun toString(): String = "EnterpriseTrackingDevice(redacted)"
}

/** Opens the session of a trip when the driver starts it. */
data class TrackingOpenInput(
    val session: TrackingSessionRef? = null,
    val trip: TrackingTripFacts = TrackingTripFacts(),
    val author: MembershipRef? = null,
    val origin: TrackingOrigin? = null,
    val policy: TrackingPolicy? = null,
    val at: Instant? = null,
    /** The trip's current session, if it already has one. */
    val existing: TrackingSession? = null,
) {
    override fun toString(): String = "EnterpriseTrackingOpenInput(redacted)"
}

/** The owner's start or resume on the visible trip screen. */
data class TrackingDriverAction(
    val trip: TrackingTripFacts = TrackingTripFacts(),
    val author: MembershipRef? = null,
    val origin: TrackingOrigin? = null,
    val device: TrackingDevice = TrackingDevice(),
    val at: Instant? = null,
) {
    override fun toString(): String = "EnterpriseTrackingDriverAction(redacted)"
}

/** A suspension, interruption, reduction or end with its typed cause; the author is consulted only for STOPPED_BY_DRIVER. */
data class TrackingEvent(
    val trip: TrackingTripFacts = TrackingTripFacts(),
    val cause: TrackingCause? = null,
    val author: MembershipRef? = null,
    val at: Instant? = null,
) {
    override fun toString(): String = "EnterpriseTrackingEvent(redacted)"
}

/** One recorded history entry. */
class TrackingTransition internal constructor(
    val state: TrackingSessionState,
    val cause: TrackingCause?,
    val actor: TrackingActor,
    val permission: LocationPermission?,
    val at: Instant,
) {
    override fun toString(): String = "EnterpriseTrackingTransition(redacted)"
}

/** One half-open recording period with its precision; [end] is null while it is open. */
class TrackingPeriod internal constructor(val start: Instant, val end: Instant?, val precision: LocationPermission) {
    override fun toString(): String = "EnterpriseTrackingPeriod(redacted)"
}

private enum class EventCommand { SUSPEND, INTERRUPT, REDUCE, END }

/** The closed meaning of one event cause. */
private class CauseRule(val command: EventCommand, val to: TrackingSessionState, val actor: TrackingActor, val trip: Set<TripState>)

private val inProgress = setOf(TripState.IN_PROGRESS)
private val started = setOf(TripState.IN_PROGRESS, TripState.PAUSED, TripState.COMPLETED, TripState.CANCELLED)
private val rules = mapOf(
    TrackingCause.TRIP_PAUSED to CauseRule(EventCommand.SUSPEND, TrackingSessionState.SUSPENDED, TrackingActor.TRIP, setOf(TripState.PAUSED)),
    TrackingCause.LONG_STATIONARY to CauseRule(EventCommand.SUSPEND, TrackingSessionState.SUSPENDED, TrackingActor.DEVICE, inProgress),
    TrackingCause.PERMISSION_REVOKED to CauseRule(EventCommand.INTERRUPT, TrackingSessionState.INTERRUPTED, TrackingActor.DEVICE, inProgress),
    TrackingCause.NOTIFICATION_BLOCKED to CauseRule(EventCommand.INTERRUPT, TrackingSessionState.INTERRUPTED, TrackingActor.DEVICE, inProgress),
    TrackingCause.PROCESS_STOPPED to CauseRule(EventCommand.INTERRUPT, TrackingSessionState.INTERRUPTED, TrackingActor.DEVICE, inProgress),
    TrackingCause.DEVICE_RESTARTED to CauseRule(EventCommand.INTERRUPT, TrackingSessionState.INTERRUPTED, TrackingActor.DEVICE, inProgress),
    TrackingCause.STOPPED_BY_DRIVER to CauseRule(EventCommand.INTERRUPT, TrackingSessionState.INTERRUPTED, TrackingActor.DRIVER, inProgress),
    TrackingCause.OWNER_CHANGED to CauseRule(EventCommand.INTERRUPT, TrackingSessionState.INTERRUPTED, TrackingActor.DEVICE, inProgress),
    TrackingCause.PRECISION_REDUCED to CauseRule(EventCommand.REDUCE, TrackingSessionState.RECORDING, TrackingActor.DEVICE, inProgress),
    TrackingCause.TRIP_COMPLETED to CauseRule(EventCommand.END, TrackingSessionState.ENDED, TrackingActor.TRIP, setOf(TripState.COMPLETED)),
    TrackingCause.TRIP_CANCELLED to CauseRule(EventCommand.END, TrackingSessionState.ENDED, TrackingActor.TRIP, setOf(TripState.CANCELLED)),
    TrackingCause.MEMBERSHIP_ENDED to CauseRule(EventCommand.END, TrackingSessionState.ENDED, TrackingActor.SERVER, started),
    TrackingCause.DEVICE_REVOKED to CauseRule(EventCommand.END, TrackingSessionState.ENDED, TrackingActor.SERVER, started),
)

/** Only the visible enterprise trip screen opens, starts or resumes (BR-GPS-002). */
private fun requireExplicit(origin: TrackingOrigin) {
    when (origin) {
        TrackingOrigin.TRIP_SCREEN -> Unit
        TrackingOrigin.PERSONAL -> throw failure(TrackingFailure.PERSONAL_TRIP)
        else -> throw failure(TrackingFailure.IMPLICIT_START)
    }
}

/** Immutable tracking session; only [open] and its commands build one. */
class TrackingSession private constructor(
    val reference: TrackingSessionRef,
    val trip: TripRef,
    val owner: MembershipRef,
    val policy: TrackingPolicy,
    history: List<TrackingTransition>,
) {
    val history: List<TrackingTransition> = history.toList()
    val organizationId: String get() = reference.organizationId

    /** The state of the last history entry. */
    val state: TrackingSessionState get() = history.last().state

    /** The cause of the last history entry; null after a successful start or resume. */
    val cause: TrackingCause? get() = history.last().cause

    /** The precision of the current recording period; null when not recording. */
    val precision: LocationPermission? get() = if (state == TrackingSessionState.RECORDING) recordingPeriods.last().precision else null

    /** Whether any recording period was approximate. */
    val coarse: Boolean get() = recordingPeriods.any { it.precision == LocationPermission.APPROXIMATE }

    /** Half-open recording periods derived from the history; a reduction closes the precise period and opens an approximate one. */
    val recordingPeriods: List<TrackingPeriod> get() {
        val out = mutableListOf<TrackingPeriod>()
        history.forEachIndexed { i, h ->
            val wasRecording = i > 0 && history[i - 1].state == TrackingSessionState.RECORDING
            val reduction = h.cause == TrackingCause.PRECISION_REDUCED
            if (wasRecording && (h.state != TrackingSessionState.RECORDING || reduction)) {
                out[out.size - 1] = TrackingPeriod(out.last().start, h.at, out.last().precision)
            }
            if (h.state == TrackingSessionState.RECORDING && (!wasRecording || reduction)) {
                out += TrackingPeriod(h.at, null, requireNotNull(h.permission))
            }
        }
        return out.toList()
    }

    /** Validates the scope, the receiver and the bound trip's facts. */
    private fun check(scope: String, facts: TrackingTripFacts): Triple<TripRef, MembershipRef, TripState> {
        requireScope(scope)
        if (organizationId != scope) throw failure(TrackingFailure.SCOPE)
        val checked = facts.check(scope)
        if (checked.first != trip) throw failure(TrackingFailure.SCOPE)
        return checked
    }

    /** Appends one entry to a copy of the history; instants never go back. */
    private fun record(transition: TrackingTransition): TrackingSession {
        if (transition.at < history.last().at) throw failure(TrackingFailure.OUT_OF_ORDER)
        return TrackingSession(reference, trip, owner, policy, history + transition)
    }

    /** Begins recording from NOT_STARTED (B-BR-EMP-422). */
    fun start(scope: String, action: TrackingDriverAction): TrackingSession =
        driverAction(scope, action, setOf(TrackingSessionState.NOT_STARTED))

    /** Records again from SUSPENDED or INTERRUPTED (B-BR-EMP-422). */
    fun resume(scope: String, action: TrackingDriverAction): TrackingSession =
        driverAction(scope, action, setOf(TrackingSessionState.SUSPENDED, TrackingSessionState.INTERRUPTED))

    /**
     * Scope, structure, the explicit origin, the owner, the from-state, the trip state and the instant order; a refused
     * permission or a hidden notification is recorded as the attempt's cause and keeps the state.
     */
    private fun driverAction(scope: String, action: TrackingDriverAction, from: Set<TrackingSessionState>): TrackingSession {
        val (_, driver, tripState) = check(scope, action.trip)
        val author = owned(scope, action.author) { it.organizationId }
        val origin = action.origin ?: throw invalid()
        val permission = action.device.permission ?: throw invalid()
        val at = requireInstant(action.at)
        requireExplicit(origin)
        if (author != owner || driver != owner) throw failure(TrackingFailure.FORBIDDEN)
        if (state !in from) throw failure(TrackingFailure.INVALID_TRANSITION)
        if (tripState != TripState.IN_PROGRESS) throw failure(TrackingFailure.TRIP_STATE)
        val transition = when {
            permission == LocationPermission.NONE -> TrackingTransition(state, TrackingCause.PERMISSION_DENIED, TrackingActor.DRIVER, permission, at)
            !action.device.notificationVisible -> TrackingTransition(state, TrackingCause.NOTIFICATION_BLOCKED, TrackingActor.DRIVER, permission, at)
            else -> TrackingTransition(TrackingSessionState.RECORDING, null, TrackingActor.DRIVER, permission, at)
        }
        return record(transition)
    }

    /** Moves RECORDING to SUSPENDED (B-BR-EMP-423). */
    fun suspend(scope: String, event: TrackingEvent): TrackingSession = event(scope, event, EventCommand.SUSPEND)

    /** Moves RECORDING to INTERRUPTED (B-BR-EMP-423). */
    fun interrupt(scope: String, event: TrackingEvent): TrackingSession = event(scope, event, EventCommand.INTERRUPT)

    /** Keeps RECORDING with approximate precision (B-BR-EMP-423). */
    fun reducePrecision(scope: String, event: TrackingEvent): TrackingSession = event(scope, event, EventCommand.REDUCE)

    /** Moves any open state to ENDED (B-BR-EMP-424). */
    fun end(scope: String, event: TrackingEvent): TrackingSession = event(scope, event, EventCommand.END)

    /** Scope, structure, the cause of this command, the driver's own stop, the from-state, the trip state and the instant order. */
    private fun event(scope: String, event: TrackingEvent, command: EventCommand): TrackingSession {
        val (_, _, tripState) = check(scope, event.trip)
        val cause = event.cause ?: throw invalid()
        val at = requireInstant(event.at)
        val rule = rules[cause]?.takeIf { it.command == command } ?: throw invalid()
        if (rule.actor == TrackingActor.DRIVER && owned(scope, event.author) { it.organizationId } != owner) {
            throw failure(TrackingFailure.FORBIDDEN)
        }
        val allowed = when (command) {
            EventCommand.END -> state != TrackingSessionState.ENDED
            EventCommand.REDUCE -> precision == LocationPermission.PRECISE
            else -> state == TrackingSessionState.RECORDING
        }
        if (!allowed) throw failure(TrackingFailure.INVALID_TRANSITION)
        if (tripState !in rule.trip) throw failure(TrackingFailure.TRIP_STATE)
        return apply(cause, rule, at)
    }

    private fun apply(cause: TrackingCause, rule: CauseRule, at: Instant): TrackingSession = record(
        TrackingTransition(rule.to, cause, rule.actor, if (rule.command == EventCommand.REDUCE) LocationPermission.APPROXIMATE else null, at),
    )

    /**
     * Maps the trip's current state onto the session (B-BR-EMP-425) and never refuses with INVALID_TRANSITION: PAUSED
     * suspends a recording session, COMPLETED and CANCELLED end any open session, IN_PROGRESS changes nothing.
     */
    fun follow(scope: String, trip: TrackingTripFacts, at: Instant?): TrackingSession {
        val (_, _, tripState) = check(scope, trip)
        val instant = requireInstant(at)
        val cause = when (tripState) {
            TripState.IN_PROGRESS -> return this
            TripState.PAUSED -> if (state != TrackingSessionState.RECORDING) return this else TrackingCause.TRIP_PAUSED
            TripState.COMPLETED, TripState.CANCELLED -> when {
                state == TrackingSessionState.ENDED -> return this
                tripState == TripState.CANCELLED -> TrackingCause.TRIP_CANCELLED
                else -> TrackingCause.TRIP_COMPLETED
            }
            else -> throw failure(TrackingFailure.TRIP_STATE)
        }
        return apply(cause, rules.getValue(cause), instant)
    }

    /**
     * The precision of a fix that may be collected now, or null when nothing may be collected (B-BR-EMP-425,
     * BR-GPS-001/007): only for the bound trip IN_PROGRESS, still run by the owner, while the session is RECORDING.
     */
    fun collecting(scope: String, trip: TrackingTripFacts): LocationPermission? {
        val (_, driver, tripState) = check(scope, trip)
        if (tripState != TripState.IN_PROGRESS || driver != owner || state != TrackingSessionState.RECORDING) return null
        return precision
    }

    override fun toString(): String = "EnterpriseTrackingSession(redacted)"

    companion object {
        /**
         * Creates the NOT_STARTED session of an IN_PROGRESS trip (B-BR-EMP-421): scope, structure and ownership first,
         * then the existing session's binding, then origin, driver, policy, trip state and the one-session rule.
         */
        fun open(scope: String, input: TrackingOpenInput): TrackingSession {
            requireScope(scope)
            val session = owned(scope, input.session) { it.organizationId }
            val (tripRef, driver, tripState) = input.trip.check(scope)
            val author = owned(scope, input.author) { it.organizationId }
            val origin = input.origin ?: throw invalid()
            val policy = input.policy ?: throw invalid()
            val at = requireInstant(input.at)
            input.existing?.let { if (owned(scope, it) { e -> e.organizationId }.trip != tripRef) throw failure(TrackingFailure.SCOPE) }
            requireExplicit(origin)
            when {
                author != driver -> throw failure(TrackingFailure.FORBIDDEN)
                policy == TrackingPolicy.OFF -> throw failure(TrackingFailure.TRACKING_OFF)
                tripState != TripState.IN_PROGRESS -> throw failure(TrackingFailure.TRIP_STATE)
                input.existing != null -> throw failure(TrackingFailure.SESSION_EXISTS)
            }
            return TrackingSession(session, tripRef, driver, policy,
                listOf(TrackingTransition(TrackingSessionState.NOT_STARTED, TrackingCause.NOT_STARTED, TrackingActor.DRIVER, null, at)))
        }
    }
}
