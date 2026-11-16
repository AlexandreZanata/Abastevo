package com.anpfuel.domain.enterprise.tracking

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.SyncJson
import com.anpfuel.domain.enterprise.trip.MembershipRef
import com.anpfuel.domain.enterprise.trip.TripRef
import com.anpfuel.domain.enterprise.trip.TripState
import java.time.Instant

/*
 * The device's local tracking index (EMP-GPS-002, B-BR-EMP-495..499, EDR-321..324): one sealed `tracking-book` record per
 * partition, written together with every session operation it indexes, holding the last policy read and each session's
 * stored transitions and checkpoint. It decides nothing on the server; every read replays it through the session domain.
 */

private val NOTICE = Regex("^[a-z0-9][a-z0-9._-]{0,63}$")
private val BOOT = Regex("^[A-Za-z0-9._-]{1,64}$")

/** The last read of the organization's policy in force (B-BR-EMP-495); a display hint, never authority. */
class TrackingPolicyRead(val policy: TrackingPolicy, val revision: Long, val noticeVersion: String?, val noticeInForceAt: Instant?,
    val readAt: Instant) {
    init {
        require(revision in 0..Long.MAX_VALUE / 2 && (noticeVersion == null) == (noticeInForceAt == null) &&
            (policy == TrackingPolicy.OFF) == (noticeVersion == null) && (policy == TrackingPolicy.OFF || revision >= 1) &&
            (noticeVersion == null || NOTICE.matches(noticeVersion)))
    }
    override fun toString(): String = "EnterpriseTrackingPolicyRead(redacted)"
}

/** One stored history entry with the operation that recorded it. */
class StoredTrackingTransition(val state: TrackingSessionState, val cause: TrackingCause?, val actor: TrackingActor,
    val permission: LocationPermission?, val at: Instant, val operationId: String) {
    init { EnterprisePartition.requireId(operationId) }
    override fun toString(): String = "EnterpriseStoredTrackingTransition(redacted)"
}

/** The last proof that this device was recording (BR-GPS-012): its boot identity and the instant. */
class TrackingCheckpoint(val bootId: String, val at: Instant) {
    init { require(BOOT.matches(bootId)) }
    override fun equals(other: Any?): Boolean = other is TrackingCheckpoint && bootId == other.bootId && at == other.at
    override fun hashCode(): Int = 31 * bootId.hashCode() + at.hashCode()
    override fun toString(): String = "EnterpriseTrackingCheckpoint(redacted)"
}

/** One session of the book; [refusedOperationId] names a final server refusal that stopped it on this device. */
class LocalTrackingSession(val sessionId: String, val tripId: String, val policy: TrackingPolicy, transitions: List<StoredTrackingTransition>,
    val checkpoint: TrackingCheckpoint? = null, val refusedOperationId: String? = null) {
    private val values = transitions.toList()
    val transitions: List<StoredTrackingTransition> get() = values.toList()
    init {
        EnterprisePartition.requireId(sessionId); EnterprisePartition.requireId(tripId)
        refusedOperationId?.let(EnterprisePartition::requireId)
        require(policy != TrackingPolicy.OFF && values.size in 2..TrackingBook.MAX_TRANSITIONS)
        require(values.zipWithNext().all { (a, b) -> !b.at.isBefore(a.at) })
        require(refusedOperationId == null || refusedOperationId in operationIds)
    }
    val state: TrackingSessionState get() = values.last().state
    val lastAt: Instant get() = values.last().at
    /** Operation identifiers in recording order, each once. */
    val operationIds: List<String> get() = values.map { it.operationId }.distinct()
    val lastOperationId: String get() = values.last().operationId
    val refused: Boolean get() = refusedOperationId != null
    internal fun copy(transitions: List<StoredTrackingTransition> = values, checkpoint: TrackingCheckpoint? = this.checkpoint,
        refusedOperationId: String? = this.refusedOperationId) =
        LocalTrackingSession(sessionId, tripId, policy, transitions, checkpoint, refusedOperationId)
    override fun toString(): String = "EnterpriseLocalTrackingSession(redacted)"
}

/** The versioned `tracking-book` index of one partition (EDR-322). */
class TrackingBook private constructor(val membershipId: String, val enrollmentId: String, val policy: TrackingPolicyRead?,
    sessions: List<LocalTrackingSession>) {
    private val values = sessions.toList()
    val sessions: List<LocalTrackingSession> get() = values.toList()
    init {
        EnterprisePartition.requireId(membershipId); EnterprisePartition.requireId(enrollmentId)
        require(values.size <= MAX_SESSIONS && values.map { it.sessionId }.distinct().size == values.size &&
            values.map { it.tripId }.distinct().size == values.size)
        val ids = values.flatMap { it.operationIds }
        require(ids.distinct().size == ids.size)
    }
    /** The one session of a trip (BR-GPS-008). */
    fun session(tripId: String): LocalTrackingSession? = values.firstOrNull { it.tripId == tripId }
    fun sessionById(sessionId: String): LocalTrackingSession? = values.firstOrNull { it.sessionId == sessionId }

    fun withPolicy(read: TrackingPolicyRead): TrackingBook = TrackingBook(membershipId, enrollmentId, read, values)
    /** Adds or replaces the session with its new stored transitions. */
    fun withSession(session: LocalTrackingSession): TrackingBook {
        val existing = sessionById(session.sessionId)
        if (existing == null) {
            require(session(session.tripId) == null && values.size < MAX_SESSIONS)
            return TrackingBook(membershipId, enrollmentId, policy, values + session)
        }
        require(existing.tripId == session.tripId && existing.policy == session.policy)
        return TrackingBook(membershipId, enrollmentId, policy, values.map { if (it.sessionId == session.sessionId) session else it })
    }
    fun withCheckpoint(sessionId: String, checkpoint: TrackingCheckpoint): TrackingBook {
        val session = requireNotNull(sessionById(sessionId))
        return withSession(session.copy(checkpoint = checkpoint))
    }
    /** A final refusal of one of the session's operations stops it on this device (B-BR-EMP-497); idempotent. */
    fun withRefusal(sessionId: String, operationId: String): TrackingBook {
        val session = sessionById(sessionId) ?: return this
        if (session.refused || operationId !in session.operationIds) return this
        return withSession(session.copy(refusedOperationId = operationId))
    }
    fun requireBinding(membershipId: String, enrollmentId: String): TrackingBook {
        require(membershipId == this.membershipId && enrollmentId == this.enrollmentId) { "Foreign tracking book" }
        return this
    }

    val canonicalBytes: ByteArray get() = CanonicalJson.encode(SyncJson.Object(mapOf(
        "format" to SyncJson.Text(FORMAT), "version" to SyncJson.Integer(VERSION.toString()),
        "membership_id" to SyncJson.Text(membershipId), "enrollment_id" to SyncJson.Text(enrollmentId),
        "policy" to (policy?.let(::policyJson) ?: SyncJson.Null),
        "sessions" to SyncJson.Array(values.map(::sessionJson)),
    )))
    override fun toString(): String = "EnterpriseTrackingBook(redacted)"

    companion object {
        const val FORMAT = "tracking-book"
        const val VERSION = 1L
        const val MAX_SESSIONS = 1000
        const val MAX_TRANSITIONS = 2000
        const val MAX_BYTES = 4 * 1024 * 1024
        private val SESSION_KEYS = setOf("session_id", "trip_id", "policy", "transitions", "checkpoint", "refused_operation_id")
        private val TRANSITION_KEYS = setOf("state", "cause", "actor", "permission", "at", "operation_id")
        private val POLICY_KEYS = setOf("policy", "revision", "notice_version", "notice_in_force_at", "read_at")

        fun empty(membershipId: String, enrollmentId: String): TrackingBook = TrackingBook(membershipId, enrollmentId, null, emptyList())

        /** Strict decode; an unknown format, version or spelling is refused, never guessed. */
        fun decode(raw: ByteArray): TrackingBook {
            val root = obj(CanonicalJson.parseLarge(raw, MAX_BYTES), setOf("format", "version", "membership_id", "enrollment_id", "policy", "sessions"))
            require(text(root, "format") == FORMAT && (root.fields["version"] as? SyncJson.Integer)?.lexeme == VERSION.toString())
            val policy = root.fields["policy"].let { if (it === SyncJson.Null) null else obj(it, POLICY_KEYS) }?.let {
                TrackingPolicyRead(TrackingPolicy.parse(text(it, "policy")), integer(it, "revision"), nullable(it, "notice_version"),
                    nullable(it, "notice_in_force_at")?.let(::instant), instant(text(it, "read_at")))
            }
            val sessions = array(root, "sessions").map { item ->
                val s = obj(item, SESSION_KEYS)
                LocalTrackingSession(text(s, "session_id"), text(s, "trip_id"), TrackingPolicy.parse(text(s, "policy")),
                    array(s, "transitions").map { t ->
                        val v = obj(t, TRANSITION_KEYS)
                        StoredTrackingTransition(TrackingSessionState.parse(text(v, "state")), nullable(v, "cause")?.let(TrackingCause::parse),
                            TrackingActor.parse(text(v, "actor")), nullable(v, "permission")?.let(LocationPermission::parse),
                            instant(text(v, "at")), text(v, "operation_id"))
                    },
                    s.fields["checkpoint"].let { if (it === SyncJson.Null) null else obj(it, setOf("boot_id", "at")) }?.let {
                        TrackingCheckpoint(text(it, "boot_id"), instant(text(it, "at")))
                    },
                    nullable(s, "refused_operation_id"))
            }
            val book = TrackingBook(text(root, "membership_id"), text(root, "enrollment_id"), policy, sessions)
            require(book.canonicalBytes.contentEquals(raw))
            return book
        }

        private fun obj(value: SyncJson?, keys: Set<String>): SyncJson.Object {
            val result = value as? SyncJson.Object ?: throw IllegalArgumentException("Invalid tracking book")
            require(result.fields.keys == keys) { "Invalid tracking book" }
            return result
        }
        private fun array(value: SyncJson.Object, key: String): List<SyncJson> =
            (value.fields[key] as? SyncJson.Array ?: throw IllegalArgumentException("Invalid tracking book")).values
        private fun text(value: SyncJson.Object, key: String): String = (value.fields[key] as? SyncJson.Text)?.value ?: throw IllegalArgumentException()
        private fun nullable(value: SyncJson.Object, key: String): String? = value.fields[key].let { if (it === SyncJson.Null) null else text(value, key) }
        private fun integer(value: SyncJson.Object, key: String): Long = (value.fields[key] as? SyncJson.Integer)?.lexeme?.toLongOrNull()
            ?: throw IllegalArgumentException()
        private fun instant(raw: String): Instant {
            val at = EnterpriseTime.parseInstant(raw)
            require(EnterpriseTime.formatInstant(at) == raw)
            return at
        }
        private fun optional(value: String?): SyncJson = value?.let { SyncJson.Text(it) } ?: SyncJson.Null
        private fun policyJson(p: TrackingPolicyRead): SyncJson = SyncJson.Object(mapOf("policy" to SyncJson.Text(p.policy.name),
            "revision" to SyncJson.Integer(p.revision.toString()), "notice_version" to optional(p.noticeVersion),
            "notice_in_force_at" to optional(p.noticeInForceAt?.let(EnterpriseTime::formatInstant)),
            "read_at" to SyncJson.Text(EnterpriseTime.formatInstant(p.readAt))))
        private fun sessionJson(s: LocalTrackingSession): SyncJson = SyncJson.Object(mapOf(
            "session_id" to SyncJson.Text(s.sessionId), "trip_id" to SyncJson.Text(s.tripId), "policy" to SyncJson.Text(s.policy.name),
            "transitions" to SyncJson.Array(s.transitions.map {
                SyncJson.Object(mapOf("state" to SyncJson.Text(it.state.name), "cause" to optional(it.cause?.name),
                    "actor" to SyncJson.Text(it.actor.name), "permission" to optional(it.permission?.name),
                    "at" to SyncJson.Text(EnterpriseTime.formatInstant(it.at)), "operation_id" to SyncJson.Text(it.operationId)))
            }),
            "checkpoint" to (s.checkpoint?.let { SyncJson.Object(mapOf("boot_id" to SyncJson.Text(it.bootId),
                "at" to SyncJson.Text(EnterpriseTime.formatInstant(it.at)))) } ?: SyncJson.Null),
            "refused_operation_id" to optional(s.refusedOperationId),
        ))
    }
}

/** The trip facts that the device knows, from its own trip book. */
/** EMP-TRIP-021 adds the device's paused and cancelled trips (EMP-TRIP-011 lifecycle commands). */
enum class TrackingLocalTrip { IN_PROGRESS, FINISHED, START_REFUSED, PAUSED, CANCELLED }

/**
 * The device's decision rules (EDR-322): a mirror of the intake's `Rebuild` and `Decide` over the EMP-GPS-001 session
 * domain, used by the service before a write and again by the producer inside the protected write.
 */
object TrackingDecisions {
    /** The trip state that the session domain sees for a device trip; another responsible member is a different driver. */
    fun tripState(local: TrackingLocalTrip): TripState = when (local) {
        TrackingLocalTrip.IN_PROGRESS -> TripState.IN_PROGRESS
        TrackingLocalTrip.FINISHED -> TripState.COMPLETED
        TrackingLocalTrip.START_REFUSED -> TripState.CANCELLED
        TrackingLocalTrip.PAUSED -> TripState.PAUSED
        TrackingLocalTrip.CANCELLED -> TripState.CANCELLED
    }

    fun facts(organizationId: String, tripId: String, driverMembershipId: String, state: TripState): TrackingTripFacts =
        TrackingTripFacts(TripRef(organizationId, tripId), MembershipRef(organizationId, driverMembershipId), state)

    private val replayStates = mapOf(TrackingCause.TRIP_PAUSED to TripState.PAUSED, TrackingCause.TRIP_COMPLETED to TripState.COMPLETED,
        TrackingCause.TRIP_CANCELLED to TripState.CANCELLED)

    private fun command(session: TrackingSession, scope: String, event: TrackingEvent): TrackingSession = when (event.cause) {
        TrackingCause.TRIP_PAUSED, TrackingCause.LONG_STATIONARY -> session.suspend(scope, event)
        TrackingCause.PRECISION_REDUCED -> session.reducePrecision(scope, event)
        TrackingCause.TRIP_COMPLETED, TrackingCause.TRIP_CANCELLED, TrackingCause.MEMBERSHIP_ENDED, TrackingCause.DEVICE_REVOKED -> session.end(scope, event)
        else -> session.interrupt(scope, event)
    }

    /** Replays the stored transitions through the domain and proves the replay reproduces every row. */
    fun rebuild(organizationId: String, ownerMembershipId: String, local: LocalTrackingSession): TrackingSession {
        val rows = local.transitions
        val owner = MembershipRef(organizationId, ownerMembershipId)
        fun facts(state: TripState) = facts(organizationId, local.tripId, ownerMembershipId, state)
        var session = TrackingSession.open(organizationId, TrackingOpenInput(TrackingSessionRef(organizationId, local.sessionId),
            facts(TripState.IN_PROGRESS), owner, TrackingOrigin.TRIP_SCREEN, local.policy, rows.first().at))
        for (row in rows.drop(1)) {
            val previous = session.state
            val attempt = row.cause == TrackingCause.PERMISSION_DENIED || row.cause == TrackingCause.NOTIFICATION_BLOCKED && row.state == previous
            session = if (row.actor == TrackingActor.DRIVER && (row.cause == null || attempt)) {
                val action = TrackingDriverAction(facts(TripState.IN_PROGRESS), owner, TrackingOrigin.TRIP_SCREEN,
                    TrackingDevice(row.permission, row.cause != TrackingCause.NOTIFICATION_BLOCKED), row.at)
                if (previous == TrackingSessionState.NOT_STARTED) session.start(organizationId, action) else session.resume(organizationId, action)
            } else {
                command(session, organizationId, TrackingEvent(facts(replayStates[row.cause] ?: TripState.IN_PROGRESS), row.cause, owner, row.at))
            }
        }
        val history = session.history
        require(history.size == rows.size)
        history.zip(rows).forEach { (h, r) ->
            require(h.state == r.state && h.cause == r.cause && h.actor == r.actor && h.permission == r.permission && h.at == r.at)
        }
        return session
    }

    /**
     * Applies one decoded operation with the trip facts at its instant to the trip's session ([existing], null when it has
     * none); a first start opens the session under [policy] (B-BR-EMP-497).
     */
    fun decide(organizationId: String, authorMembershipId: String, fact: TrackingOperationFact, trip: TrackingTripFacts,
        existing: TrackingSession?, policy: TrackingPolicy?): TrackingSession {
        val author = MembershipRef(organizationId, authorMembershipId)
        val at = fact.declared.occurredAt
        return when (fact) {
            is TrackingOperationFact.DriverAction -> {
                val action = TrackingDriverAction(trip, author, fact.origin, TrackingDevice(fact.permission, fact.notificationVisible), at)
                if (fact.type == TrackingOperations.START) {
                    if (existing != null && existing.reference.id == fact.sessionId) existing.start(organizationId, action)
                    else TrackingSession.open(organizationId, TrackingOpenInput(TrackingSessionRef(organizationId, fact.sessionId), trip, author,
                        fact.origin, policy, at, existing)).start(organizationId, action)
                } else {
                    require(existing != null && existing.reference.id == fact.sessionId)
                    existing.resume(organizationId, action)
                }
            }
            is TrackingOperationFact.Event -> {
                require(existing != null && existing.reference.id == fact.sessionId)
                val event = TrackingEvent(trip, fact.cause, author, at)
                when (fact.type) {
                    TrackingOperations.SUSPEND -> existing.suspend(organizationId, event)
                    TrackingOperations.INTERRUPT -> existing.interrupt(organizationId, event)
                    TrackingOperations.REDUCE -> existing.reducePrecision(organizationId, event)
                    TrackingOperations.END -> existing.end(organizationId, event)
                    else -> throw TrackingDomainException()
                }
            }
        }
    }

    /** The stored session after one accepted decision: the previous rows plus every new history entry of [next]. */
    fun record(previous: LocalTrackingSession?, next: TrackingSession, operationId: String): LocalTrackingSession {
        val history = next.history
        val from = previous?.transitions?.size ?: 0
        require(history.size > from)
        val added = history.drop(from).map { StoredTrackingTransition(it.state, it.cause, it.actor, it.permission, it.at, operationId) }
        return previous?.copy(transitions = previous.transitions + added)
            ?: LocalTrackingSession(next.reference.id, next.trip.id, next.policy, added)
    }
}
