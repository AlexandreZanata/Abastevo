package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.meter.MeterDomainException
import com.anpfuel.domain.enterprise.meter.MeterPresence
import java.time.Instant

/** Closed mission failure vocabulary shared with the Go package through mission-v2 (EMP-TRIP-008/009). */
enum class TripMissionFailure { INVALID, SCOPE, FORBIDDEN, CONFLICT, INVALID_TRANSITION, UNSUPPORTED_COMMAND }

class TripMissionException(val failure: TripMissionFailure = TripMissionFailure.INVALID) :
    IllegalArgumentException("trip mission: ${failure.name.lowercase()}")

private fun mission(failure: TripMissionFailure = TripMissionFailure.INVALID) = TripMissionException(failure)

/** Mission operational states of the canonical catalog (08-state-model §3). */
enum class TripMissionState {
    DRAFT, PLANNED, ASSIGNED, ACCEPTED, DECLINED, IN_PROGRESS, COMPLETED, CANCELLED;

    val terminal: Boolean get() = this == COMPLETED || this == CANCELLED

    companion object {
        /** Exact tokens only; anything else is INVALID. */
        fun parse(raw: String): TripMissionState = entries.firstOrNull { it.name == raw } ?: throw mission()
    }
}

/** Mission commands. */
enum class TripMissionCommand {
    PLAN, ASSIGN, ACCEPT, DECLINE, START, COMPLETE, CANCEL;

    companion object {
        fun parse(raw: String): TripMissionCommand = entries.firstOrNull { it.name == raw } ?: throw mission()
    }
}

/** The assignee's answer. */
enum class TripMissionResponse {
    ACCEPT, DECLINE;

    val command: TripMissionCommand get() = if (this == ACCEPT) TripMissionCommand.ACCEPT else TripMissionCommand.DECLINE

    companion object {
        fun parse(raw: String): TripMissionResponse = entries.firstOrNull { it.name == raw } ?: throw mission()
    }
}

class MissionRef(organizationId: String, id: String) : ScopedTripRef(organizationId, id)
class UnitRef(organizationId: String, id: String) : ScopedTripRef(organizationId, id)
class MissionAssetRef(organizationId: String, id: String) : ScopedTripRef(organizationId, id)

private val firstInstant: Instant = Instant.parse("0001-01-01T00:00:00Z")
private val afterLastInstant: Instant = Instant.parse("+10000-01-01T00:00:00Z")

/** The planned half-open window [from, to): whole-microsecond bounds, `to` after `from`, no maximum span (EDR-228). */
data class TripMissionWindow(val from: Instant, val to: Instant) {
    internal fun validated(): TripMissionWindow {
        if (!whole(from) || !whole(to) || to <= from) throw mission()
        return this
    }

    private fun whole(at: Instant): Boolean = at > firstInstant && at < afterLastInstant && at.nano % 1000 == 0
}

/** The METER-001 text profile shared by titles and reasons. */
object TripMissionText {
    fun validate(raw: String) {
        try { MeterPresence.missing(raw) } catch (_: MeterDomainException) { throw mission() }
    }
}

/** A new mission as the planner submitted it. */
data class TripMissionInput(
    val reference: MissionRef? = null,
    val unit: UnitRef? = null,
    val asset: MissionAssetRef? = null,
    val title: String = "",
    val window: TripMissionWindow,
) {
    override fun toString(): String = "EnterpriseTripMissionInput(redacted)"
}

/** The employee record and the membership that may answer. */
data class TripMissionAssignee(val employee: EmployeeRef?, val member: MembershipRef?) {
    override fun toString(): String = "EnterpriseTripMissionAssignee(redacted)"
}

/** The assignee's stored response; the kind is the raw token. */
data class TripMissionAnswer(val kind: String, val member: MembershipRef?, val reason: String?) {
    override fun toString(): String = "EnterpriseTripMissionAnswer(redacted)"
}

/** The stored form of a mission; the state is the raw token. */
data class TripMissionSnapshot(
    val reference: MissionRef?,
    val unit: UnitRef?,
    val asset: MissionAssetRef?,
    val title: String,
    val window: TripMissionWindow,
    val state: String,
    val revision: Long,
    val assignee: TripMissionAssignee?,
    val answer: TripMissionAnswer?,
) {
    override fun toString(): String = "EnterpriseTripMissionSnapshot(redacted)"
}

internal fun requireMissionScope(scope: String) {
    try { EnterprisePartition.requireId(scope) } catch (_: IllegalArgumentException) { throw mission(TripMissionFailure.SCOPE) }
}

/** An absent reference is INVALID, another organization's SCOPE. */
internal fun <T : ScopedTripRef> ownedByScope(scope: String, ref: T?): T {
    val value = ref ?: throw mission()
    if (value.organizationId != scope) throw mission(TripMissionFailure.SCOPE)
    return value
}

/** A reason is required for DECLINE and refused for ACCEPT. */
private fun requireReason(kind: TripMissionResponse, reason: String?) {
    if (kind == TripMissionResponse.ACCEPT) {
        if (reason != null) throw mission()
        return
    }
    TripMissionText.validate(reason ?: throw mission())
}

/** One organization mission; commands return a new value and never change the receiver. */
class TripMission private constructor(
    val reference: MissionRef,
    val unit: UnitRef,
    val asset: MissionAssetRef?,
    val title: String,
    val window: TripMissionWindow,
    val state: TripMissionState,
    val revision: Long,
    val assignee: TripMissionAssignee?,
    val answer: TripMissionAnswer?,
) {
    override fun toString(): String = "EnterpriseTripMission(redacted)"

    private fun copy(state: TripMissionState, assignee: TripMissionAssignee?, answer: TripMissionAnswer?) =
        TripMission(reference, unit, asset, title, window, state, revision + 1, assignee, answer)

    private fun subject(scope: String) {
        requireMissionScope(scope)
        if (reference.organizationId != scope) throw mission(TripMissionFailure.SCOPE)
    }

    /** The stale-revision rule, then the table. */
    private fun decide(expected: Long, command: TripMissionCommand): TripMissionState {
        if (expected != revision) throw mission(TripMissionFailure.CONFLICT)
        return TripMissionLifecycle.transition(state, command)
    }

    /**
     * Names a new assignee (B-BR-EMP-355): scope, the new employee and membership, a positive expected revision,
     * CONFLICT for a stale revision, the table, then the current membership again while ASSIGNED or ACCEPTED.
     */
    fun assign(scope: String, expected: Long, to: TripMissionAssignee): TripMission {
        subject(scope)
        val employee = ownedByScope(scope, to.employee)
        val member = ownedByScope(scope, to.member)
        if (expected < 1) throw mission()
        val next = decide(expected, TripMissionCommand.ASSIGN)
        if ((state == TripMissionState.ASSIGNED || state == TripMissionState.ACCEPTED) && assignee?.member == member) {
            throw mission(TripMissionFailure.INVALID_TRANSITION)
        }
        return copy(next, TripMissionAssignee(employee, member), null)
    }

    /**
     * Records the current assignee's answer (B-BR-EMP-356): scope, the responding membership, the response token and
     * reason, a positive expected revision, FORBIDDEN unless the membership is the current assignee's, then CONFLICT for
     * a stale revision, then the table.
     */
    fun respond(scope: String, expected: Long, member: MembershipRef?, kind: String, reason: String?): TripMission {
        subject(scope)
        val by = ownedByScope(scope, member)
        val response = TripMissionResponse.parse(kind)
        requireReason(response, reason)
        if (expected < 1) throw mission()
        if (assignee?.member != by) throw mission(TripMissionFailure.FORBIDDEN)
        val next = decide(expected, response.command)
        return copy(next, assignee, TripMissionAnswer(response.name, by, reason))
    }

    /**
     * Records a planning revision (B-BR-EMP-381): scope, the plan content, a positive expected revision, CONFLICT for a
     * stale revision, then the table; PLANNED at the next revision with the assignee and answer withdrawn.
     */
    fun plan(scope: String, expected: Long, input: TripMissionPlanInput): Pair<TripMission, TripMissionPlan> {
        subject(scope)
        val plan = TripMissionPlan.create(scope, input)
        if (expected < 1) throw mission()
        val next = decide(expected, TripMissionCommand.PLAN)
        return copy(next, null, null) to plan
    }

    /** Ends the mission with a reason, keeping assignee and answer: scope, reason, expected revision, CONFLICT, table. */
    fun cancel(scope: String, expected: Long, reason: String): TripMission {
        subject(scope)
        TripMissionText.validate(reason)
        if (expected < 1) throw mission()
        val next = decide(expected, TripMissionCommand.CANCEL)
        return copy(next, assignee, answer)
    }

    companion object {
        private fun content(scope: String, reference: MissionRef?, unit: UnitRef?, asset: MissionAssetRef?, title: String,
                            window: TripMissionWindow): Triple<MissionRef, UnitRef, TripMissionWindow> {
            requireMissionScope(scope)
            val id = ownedByScope(scope, reference)
            val place = ownedByScope(scope, unit)
            if (asset != null) ownedByScope(scope, asset)
            TripMissionText.validate(title)
            return Triple(id, place, window.validated())
        }

        /** A new mission: DRAFT at revision 1 without assignee (B-BR-EMP-354). */
        fun create(scope: String, input: TripMissionInput): TripMission {
            val (id, place, window) = content(scope, input.reference, input.unit, input.asset, input.title, input.window)
            return TripMission(id, place, input.asset, input.title, window, TripMissionState.DRAFT, 1, null, null)
        }

        private fun invariant(state: TripMissionState, revision: Long, assignee: TripMissionAssignee?, answer: TripMissionAnswer?): Boolean {
            if ((state == TripMissionState.DRAFT) != (revision == 1L)) return false
            if (answer != null && (assignee == null || answer.member != assignee.member)) return false
            return when (state) {
                TripMissionState.DRAFT, TripMissionState.PLANNED -> assignee == null && answer == null
                TripMissionState.ASSIGNED -> assignee != null && answer == null
                TripMissionState.ACCEPTED, TripMissionState.IN_PROGRESS, TripMissionState.COMPLETED -> answer?.kind == TripMissionResponse.ACCEPT.name
                TripMissionState.DECLINED -> answer?.kind == TripMissionResponse.DECLINE.name
                TripMissionState.CANCELLED -> true
            }
        }

        /** Proves the stored invariants of a mission (EDR-228). */
        fun restore(scope: String, snapshot: TripMissionSnapshot): TripMission {
            val (id, place, window) = content(scope, snapshot.reference, snapshot.unit, snapshot.asset, snapshot.title, snapshot.window)
            val state = TripMissionState.parse(snapshot.state)
            if (snapshot.revision < 1) throw mission()
            val assignee = snapshot.assignee?.let { TripMissionAssignee(ownedByScope(scope, it.employee), ownedByScope(scope, it.member)) }
            val answer = snapshot.answer?.let {
                val by = ownedByScope(scope, it.member)
                requireReason(TripMissionResponse.parse(it.kind), it.reason)
                TripMissionAnswer(it.kind, by, it.reason)
            }
            if (!invariant(state, snapshot.revision, assignee, answer)) throw mission()
            return TripMission(id, place, snapshot.asset, snapshot.title, window, state, snapshot.revision, assignee, answer)
        }
    }
}

/**
 * The EDR-228 transition table with the EDR-246 PLAN rows, first failure first: a terminal state; START, COMPLETE and
 * every IN_PROGRESS change (later owners); ACCEPT and DECLINE only from ASSIGNED; PLAN never from ACCEPTED, else to
 * PLANNED; ASSIGN and CANCEL from the five open states.
 */
object TripMissionLifecycle {
    fun transition(from: TripMissionState, command: TripMissionCommand): TripMissionState {
        if (from.terminal) throw mission(TripMissionFailure.INVALID_TRANSITION)
        if (command == TripMissionCommand.START || command == TripMissionCommand.COMPLETE ||
            from == TripMissionState.IN_PROGRESS) throw mission(TripMissionFailure.UNSUPPORTED_COMMAND)
        return when (command) {
            TripMissionCommand.PLAN -> {
                if (from == TripMissionState.ACCEPTED) throw mission(TripMissionFailure.INVALID_TRANSITION)
                TripMissionState.PLANNED
            }
            TripMissionCommand.ACCEPT, TripMissionCommand.DECLINE -> {
                if (from != TripMissionState.ASSIGNED) throw mission(TripMissionFailure.INVALID_TRANSITION)
                if (command == TripMissionCommand.ACCEPT) TripMissionState.ACCEPTED else TripMissionState.DECLINED
            }
            TripMissionCommand.ASSIGN -> TripMissionState.ASSIGNED
            else -> TripMissionState.CANCELLED
        }
    }
}
