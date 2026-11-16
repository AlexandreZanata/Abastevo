package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncProjection
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.SyncJson
import java.time.Instant

/*
 * The device's missions inbox (EMP-TRIP-021, B-BR-EMP-527..531, EDR-342..344): a sealed copy of the member's own
 * EMP-TRIP-008 list with the device receipt clock that states its age, the answers saved on this device, and the
 * offline eligibility of each action. It decides nothing on the server.
 */

/** Local refusals before queueing; never a server outcome. */
enum class MissionLocalFailure {
    /** The mission is not in this partition's copy. */
    UNKNOWN_MISSION,
    /** Only an `ASSIGNED` mission at the copy's revision can be answered. */
    NOT_ANSWERABLE,
    /** This device already saved an answer for that revision. */
    ALREADY_ANSWERED,
    /** The company has not confirmed the acceptance yet (B-BR-EMP-529). */
    NOT_CONFIRMED,
    /** The mission already has a trip on this device whose start was not refused. */
    ALREADY_STARTED,
    /** A planned trip starts on the mission's own vehicle. */
    WRONG_VEHICLE,
    /** The copy belongs to another membership, enrollment or employee record, or an item to another assignee. */
    FOREIGN,
    /** The copy reached its bound. */
    FULL,
    /** The operation identifier already exists in the copy. */
    DUPLICATE,
}

class MissionLocalException(val failure: MissionLocalFailure) : IllegalStateException("mission local: ${failure.name.lowercase()}")

/** A mission's current plan as the own list returns it (EMP-TRIP-009); display only. */
class MissionPlanView(
    val revision: Long,
    val destinationText: String?,
    val destinationUnitId: String?,
    val reason: String,
    val costCentreId: String?,
    val load: TripMissionLoad,
    val cargoDescription: String?,
    val passengerCount: Int?,
    val plannedAt: Instant,
) {
    init {
        // The EMP-TRIP-009 plan shape: exactly one destination, the plan text profile, the load and its only field.
        require(revision >= 2 && (destinationText == null) != (destinationUnitId == null))
        destinationText?.let(::planText); destinationUnitId?.let(EnterprisePartition::requireId); costCentreId?.let(EnterprisePartition::requireId)
        planText(reason)
        require(when (load) {
            TripMissionLoad.NONE -> cargoDescription == null && passengerCount == null
            TripMissionLoad.CARGO -> cargoDescription != null && passengerCount == null
            TripMissionLoad.PASSENGERS -> cargoDescription == null && passengerCount != null && passengerCount in 1..TripMissionPlan.MAX_PASSENGERS
        })
        cargoDescription?.let(::planText)
    }
    override fun toString(): String = "EnterpriseMissionPlanView(redacted)"
}

private fun planText(raw: String) = try { TripMissionPlanText.validate(raw) } catch (_: TripMissionException) { throw IllegalArgumentException("Invalid mission text") }
private fun missionText(raw: String) = try { TripMissionText.validate(raw) } catch (_: TripMissionException) { throw IllegalArgumentException("Invalid mission text") }

/** One mission of the member's own list (B-BR-EMP-527). */
class MissionInboxItem(
    val missionId: String,
    val unitId: String,
    val assetId: String?,
    val title: String,
    val startsAt: Instant,
    val endsAt: Instant,
    val state: TripMissionState,
    val revision: Long,
    val assigneeEmployeeId: String,
    val answer: TripMissionResponse?,
    val answerReason: String?,
    val plan: MissionPlanView?,
    val createdAt: Instant,
    val updatedAt: Instant,
) {
    init {
        for (id in listOfNotNull(missionId, unitId, assetId, assigneeEmployeeId)) EnterprisePartition.requireId(id)
        missionText(title)
        require(endsAt.isAfter(startsAt) && revision >= 1)
        // The own list's answer invariants (the EMP-TRIP-022 board parser rule): an assigned mission has no answer yet.
        when (state) {
            TripMissionState.ASSIGNED -> require(answer == null)
            TripMissionState.ACCEPTED -> require(answer == TripMissionResponse.ACCEPT)
            TripMissionState.DECLINED -> require(answer == TripMissionResponse.DECLINE)
            else -> Unit
        }
        require(answerReason == null || answer == TripMissionResponse.DECLINE)
        answerReason?.let(::missionText)
    }
    override fun toString(): String = "EnterpriseMissionInboxItem(redacted)"
}

/** One parsed page of the own list; [truncated] when the server named more missions than the page holds. */
class MissionInboxPage(items: List<MissionInboxItem>, val truncated: Boolean) {
    private val values = items.toList()
    val items: List<MissionInboxItem> get() = values.toList()
    override fun toString(): String = "EnterpriseMissionInboxPage(redacted)"
}

/** One answer saved on this device: its operation, the revision it answered and what it answered. */
class MissionResponseLine(val missionId: String, val operationId: String, val revision: Long, val response: TripMissionResponse) {
    init { EnterprisePartition.requireId(missionId); EnterprisePartition.requireId(operationId); require(revision >= 1) }
    override fun toString(): String = "EnterpriseMissionResponseLine(redacted)"
}

/** The versioned `mission-inbox` record of one partition (EDR-342). */
class MissionInbox private constructor(
    val membershipId: String,
    val enrollmentId: String,
    val employeeId: String?,
    val received: EnterpriseDeviceClock?,
    val truncated: Boolean,
    items: List<MissionInboxItem>,
    responses: List<MissionResponseLine>,
) {
    private val itemValues = items.toList()
    private val responseValues = responses.toList()
    init {
        EnterprisePartition.requireId(membershipId); EnterprisePartition.requireId(enrollmentId); employeeId?.let(EnterprisePartition::requireId)
        require(itemValues.size <= MAX_ITEMS && responseValues.size <= MAX_RESPONSES)
        require(itemValues.map { it.missionId }.distinct().size == itemValues.size)
        require(responseValues.map { it.operationId }.distinct().size == responseValues.size)
        require(responseValues.map { it.missionId to it.revision }.distinct().size == responseValues.size)
        require(employeeId != null || itemValues.isEmpty() && received == null)
        require(itemValues.all { it.assigneeEmployeeId == employeeId })
    }
    val items: List<MissionInboxItem> get() = itemValues.toList()
    val responses: List<MissionResponseLine> get() = responseValues.toList()

    fun item(missionId: String): MissionInboxItem? = itemValues.firstOrNull { it.missionId == missionId }

    /** Replaces the copy with a server page received at [received]; every item must be assigned to [employeeId]. */
    fun withPage(employeeId: String, page: MissionInboxPage, received: EnterpriseDeviceClock): MissionInbox {
        EnterprisePartition.requireId(employeeId)
        // The membership's employee record never changes (EDR-054); any other assignee makes the whole page foreign.
        if (this.employeeId != null && this.employeeId != employeeId || page.items.any { it.assigneeEmployeeId != employeeId })
            throw MissionLocalException(MissionLocalFailure.FOREIGN)
        if (page.items.size > MAX_ITEMS) throw MissionLocalException(MissionLocalFailure.FULL)
        if (page.items.map { it.missionId }.distinct().size != page.items.size) throw MissionLocalException(MissionLocalFailure.DUPLICATE)
        return MissionInbox(membershipId, enrollmentId, employeeId, received, page.truncated, page.items, responseValues)
    }

    /** Indexes an answer saved on this device (B-BR-EMP-528). */
    fun withResponse(line: MissionResponseLine): MissionInbox {
        val item = item(line.missionId) ?: throw MissionLocalException(MissionLocalFailure.UNKNOWN_MISSION)
        if (responseValues.any { it.operationId == line.operationId }) throw MissionLocalException(MissionLocalFailure.DUPLICATE)
        if (item.state != TripMissionState.ASSIGNED || item.revision != line.revision) throw MissionLocalException(MissionLocalFailure.NOT_ANSWERABLE)
        if (responseValues.any { it.missionId == line.missionId && it.revision == line.revision }) throw MissionLocalException(MissionLocalFailure.ALREADY_ANSWERED)
        if (responseValues.size >= MAX_RESPONSES) throw MissionLocalException(MissionLocalFailure.FULL)
        return MissionInbox(membershipId, enrollmentId, employeeId, received, truncated, itemValues, responseValues + line)
    }

    /** Refuses a copy bound to another membership or enrollment. */
    fun requireBinding(membershipId: String, enrollmentId: String): MissionInbox {
        if (membershipId != this.membershipId || enrollmentId != this.enrollmentId) throw MissionLocalException(MissionLocalFailure.FOREIGN)
        return this
    }

    val canonicalBytes: ByteArray get() = CanonicalJson.encode(SyncJson.Object(mapOf(
        "format" to SyncJson.Text(FORMAT), "version" to SyncJson.Integer(VERSION.toString()),
        "membership_id" to SyncJson.Text(membershipId), "enrollment_id" to SyncJson.Text(enrollmentId),
        "employee_id" to optional(employeeId),
        "received" to (received?.let { SyncJson.Object(mapOf("boot" to SyncJson.Text(it.boot),
            "elapsed_millis" to SyncJson.Integer(it.elapsedMillis.toString()), "wall_millis" to SyncJson.Integer(it.wallMillis.toString()))) }
            ?: SyncJson.Null),
        "truncated" to SyncJson.Bool(truncated),
        "missions" to SyncJson.Array(itemValues.map(::itemJson)),
        "responses" to SyncJson.Array(responseValues.map { SyncJson.Object(mapOf("mission_id" to SyncJson.Text(it.missionId),
            "operation_id" to SyncJson.Text(it.operationId), "revision" to SyncJson.Integer(it.revision.toString()),
            "response" to SyncJson.Text(it.response.name))) }),
    )))
    override fun toString(): String = "EnterpriseMissionInbox(redacted)"

    companion object {
        const val FORMAT = "mission-inbox"
        const val VERSION = 1L
        /** One own-list page (`limit` 100). */
        const val MAX_ITEMS = 100
        const val MAX_RESPONSES = 1000
        const val MAX_BYTES = 1024 * 1024
        fun empty(membershipId: String, enrollmentId: String): MissionInbox {
            EnterprisePartition.requireId(membershipId); EnterprisePartition.requireId(enrollmentId)
            return MissionInbox(membershipId, enrollmentId, null, null, false, emptyList(), emptyList())
        }
        private val ROOT_KEYS = setOf("format", "version", "membership_id", "enrollment_id", "employee_id", "received", "truncated", "missions", "responses")
        private val ITEM_KEYS = setOf("mission_id", "unit_id", "asset_id", "title", "starts_at", "ends_at", "state", "revision",
            "assignee_employee_id", "answer", "answer_reason", "plan", "created_at", "updated_at")
        private val PLAN_KEYS = setOf("revision", "destination_text", "destination_unit_id", "reason", "cost_centre_id", "load",
            "cargo_description", "passenger_count", "planned_at")

        /** Strict decode; an unknown format or version is refused, never guessed. */
        fun decode(raw: ByteArray): MissionInbox {
            val root = obj(CanonicalJson.parseLarge(raw, MAX_BYTES), ROOT_KEYS)
            require(text(root, "format") == FORMAT && (root.fields["version"] as? SyncJson.Integer)?.lexeme == VERSION.toString())
            val received = root.fields["received"].let { value ->
                if (value === SyncJson.Null) null else obj(value, setOf("boot", "elapsed_millis", "wall_millis")).let {
                    EnterpriseDeviceClock(text(it, "boot"), long(it, "elapsed_millis"), long(it, "wall_millis"))
                }
            }
            val items = array(root, "missions").map { value ->
                val item = obj(value, ITEM_KEYS)
                val plan = item.fields["plan"].let { if (it === SyncJson.Null) null else obj(it, PLAN_KEYS) }?.let { p ->
                    MissionPlanView(long(p, "revision"), nullable(p, "destination_text"), nullable(p, "destination_unit_id"), text(p, "reason"),
                        nullable(p, "cost_centre_id"), enumOf(text(p, "load"), TripMissionLoad.entries), nullable(p, "cargo_description"),
                        p.fields["passenger_count"].let { if (it === SyncJson.Null) null else Math.toIntExact(long(p, "passenger_count")) },
                        instant(text(p, "planned_at")))
                }
                MissionInboxItem(text(item, "mission_id"), text(item, "unit_id"), nullable(item, "asset_id"), text(item, "title"),
                    instant(text(item, "starts_at")), instant(text(item, "ends_at")), enumOf(text(item, "state"), TripMissionState.entries),
                    long(item, "revision"), text(item, "assignee_employee_id"), nullable(item, "answer")?.let { enumOf(it, TripMissionResponse.entries) },
                    nullable(item, "answer_reason"), plan, instant(text(item, "created_at")), instant(text(item, "updated_at")))
            }
            val responses = array(root, "responses").map { value ->
                val line = obj(value, setOf("mission_id", "operation_id", "revision", "response"))
                MissionResponseLine(text(line, "mission_id"), text(line, "operation_id"), long(line, "revision"),
                    enumOf(text(line, "response"), TripMissionResponse.entries))
            }
            val truncated = (root.fields["truncated"] as? SyncJson.Bool)?.value ?: throw IllegalArgumentException("Invalid mission inbox")
            val inbox = MissionInbox(text(root, "membership_id"), text(root, "enrollment_id"), nullable(root, "employee_id"), received,
                truncated, items, responses)
            // Only the exact canonical spelling is a stored copy.
            require(inbox.canonicalBytes.contentEquals(raw))
            return inbox
        }

        private fun obj(value: SyncJson?, keys: Set<String>): SyncJson.Object {
            val result = value as? SyncJson.Object ?: throw IllegalArgumentException("Invalid mission inbox")
            require(result.fields.keys == keys) { "Invalid mission inbox" }
            return result
        }
        private fun array(value: SyncJson.Object, key: String) = (value.fields[key] as? SyncJson.Array ?: throw IllegalArgumentException()).values
        private fun text(value: SyncJson.Object, key: String): String = (value.fields[key] as? SyncJson.Text)?.value ?: throw IllegalArgumentException()
        private fun nullable(value: SyncJson.Object, key: String): String? = value.fields[key].let { if (it === SyncJson.Null) null else text(value, key) }
        private fun long(value: SyncJson.Object, key: String): Long = (value.fields[key] as? SyncJson.Integer)?.lexeme?.toLongOrNull()
            ?: throw IllegalArgumentException("Invalid mission inbox")
        private fun <T : Enum<T>> enumOf(raw: String, values: List<T>): T = values.firstOrNull { it.name == raw } ?: throw IllegalArgumentException()
        private fun instant(raw: String): Instant {
            val at = EnterpriseTime.parseInstant(raw)
            require(EnterpriseTime.formatInstant(at) == raw)
            return at
        }
        private fun optional(value: String?): SyncJson = value?.let { SyncJson.Text(it) } ?: SyncJson.Null
        private fun instantJson(at: Instant): SyncJson = SyncJson.Text(EnterpriseTime.formatInstant(at))
        private fun itemJson(item: MissionInboxItem): SyncJson = SyncJson.Object(mapOf(
            "mission_id" to SyncJson.Text(item.missionId), "unit_id" to SyncJson.Text(item.unitId), "asset_id" to optional(item.assetId),
            "title" to SyncJson.Text(item.title), "starts_at" to instantJson(item.startsAt), "ends_at" to instantJson(item.endsAt),
            "state" to SyncJson.Text(item.state.name), "revision" to SyncJson.Integer(item.revision.toString()),
            "assignee_employee_id" to SyncJson.Text(item.assigneeEmployeeId), "answer" to optional(item.answer?.name),
            "answer_reason" to optional(item.answerReason),
            "plan" to (item.plan?.let { p -> SyncJson.Object(mapOf(
                "revision" to SyncJson.Integer(p.revision.toString()), "destination_text" to optional(p.destinationText),
                "destination_unit_id" to optional(p.destinationUnitId), "reason" to SyncJson.Text(p.reason),
                "cost_centre_id" to optional(p.costCentreId), "load" to SyncJson.Text(p.load.name),
                "cargo_description" to optional(p.cargoDescription),
                "passenger_count" to (p.passengerCount?.let { SyncJson.Integer(it.toString()) } ?: SyncJson.Null),
                "planned_at" to instantJson(p.plannedAt))) } ?: SyncJson.Null),
            "created_at" to instantJson(item.createdAt), "updated_at" to instantJson(item.updatedAt),
        ))
    }
}

/** The age of the copy from the device receipt clock (BR-OFFL-016, the FLEET-013 rule). */
object MissionInboxAge {
    /** At this age or more the copy is labelled as possibly out of date. */
    const val STALE_MILLIS = 86_400_000L

    /** Elapsed time within one boot, wall time after a reboot; null when it cannot be known (negative). */
    fun ageMillis(received: EnterpriseDeviceClock, now: EnterpriseDeviceClock): Long? {
        val age = if (now.boot == received.boot) now.elapsedMillis - received.elapsedMillis else now.wallMillis - received.wallMillis
        return age.takeIf { it >= 0 }
    }
}

/** What an answer saved on this device may state (B-BR-EMP-531). */
enum class MissionAnswerState {
    NONE,
    /** Saved on the device, no final receipt. */
    SAVED,
    /** Its `ACCEPTED` receipt for an acceptance. */
    ACCEPTED,
    /** Its `ACCEPTED` receipt for a decline. */
    DECLINE_RECORDED,
    /** A final refusal or review receipt (never expected for an answer). */
    REFUSED,
    /** No receipt and a later copy shows the mission at a later revision. */
    NOT_CONFIRMED_CHANGED,
    /** No receipt and a later copy no longer lists the mission. */
    NOT_CONFIRMED_REMOVED,
}

/** The eligible actions of the approved offline matrix (10-offline-sync §10.3). Refreshing is the flow's online action. */
enum class MissionAction { ACCEPT, DECLINE, START_TRIP, OPEN_TRIP }

class MissionView(
    val missionId: String,
    /** The copy's item; absent when only an unconfirmed answer remains. */
    val item: MissionInboxItem?,
    /** The copy's state, or the state the answer's own `ACCEPTED` receipt proves at a later revision. */
    val state: TripMissionState?,
    val answer: MissionAnswerState,
    val answerResponse: TripMissionResponse?,
    val answerCode: String?,
    actions: Set<MissionAction>,
    /** Why "Iniciar viagem da missão" is not offered, when the mission is otherwise in execution reach. */
    val startBlocked: MissionLocalFailure?,
    /** The local trip linked to the mission, if any. */
    val tripId: String?,
) {
    private val values = actions.toSet()
    val actions: Set<MissionAction> get() = values.toSet()
    override fun toString(): String = "EnterpriseMissionView(redacted)"
}

class MissionInboxView(
    /** The copy was received from the server at least once. */
    val refreshed: Boolean,
    val ageMillis: Long?,
    /** At least 24 hours old, or of unknown age; never shown unlabelled. */
    val stale: Boolean,
    val truncated: Boolean,
    missions: List<MissionView>,
) {
    private val values = missions.toList()
    val missions: List<MissionView> get() = values.toList()
    override fun toString(): String = "EnterpriseMissionInboxView(redacted)"
}

object MissionInboxPresentation {
    /**
     * Every listed mission in window order, then each answer whose mission left the copy without a confirming receipt.
     * Every saved answer must have its retained entry; anything else is refused, never guessed.
     */
    fun view(inbox: MissionInbox?, entries: Map<String, EnterpriseQueueEntry>, trips: TripBook?, now: EnterpriseDeviceClock): MissionInboxView {
        if (inbox == null) return MissionInboxView(false, null, true, false, emptyList())
        val age = inbox.received?.let { MissionInboxAge.ageMillis(it, now) }
        val listed = inbox.items.sortedWith(compareBy<MissionInboxItem>({ it.startsAt }, { it.missionId })).map { item ->
            mission(item.missionId, item, inbox.responses.lastOrNull { it.missionId == item.missionId }, entries, trips)
        }
        val listedIds = inbox.items.map { it.missionId }.toSet()
        // An answer whose mission left the list stays visible until its own receipt settles it.
        val gone = inbox.responses.filter { it.missionId !in listedIds }.groupBy { it.missionId }.map { (id, lines) ->
            mission(id, null, lines.last(), entries, trips)
        }.filter { it.answer != MissionAnswerState.ACCEPTED && it.answer != MissionAnswerState.DECLINE_RECORDED }
        return MissionInboxView(inbox.received != null, age, age == null || age >= MissionInboxAge.STALE_MILLIS, inbox.truncated, listed + gone)
    }

    private fun mission(missionId: String, item: MissionInboxItem?, line: MissionResponseLine?, entries: Map<String, EnterpriseQueueEntry>,
        trips: TripBook?): MissionView {
        val entry = line?.let { requireNotNull(entries[it.operationId]) { "Missing mission answer" } }
        if (entry != null) require(entry.operation.operationId == line.operationId) { "Foreign mission answer" }
        val receipt = entry?.acknowledgement
        val sync = entry?.let { EnterpriseSyncProjection.status(EnterpriseSyncProjection.item(it)) }
        val answer = when {
            line == null -> MissionAnswerState.NONE
            sync == EnterpriseSyncStatus.ACCEPTED ->
                if (line.response == TripMissionResponse.ACCEPT) MissionAnswerState.ACCEPTED else MissionAnswerState.DECLINE_RECORDED
            sync != null && sync.final() -> MissionAnswerState.REFUSED
            item == null -> MissionAnswerState.NOT_CONFIRMED_REMOVED
            item.revision > line.revision -> MissionAnswerState.NOT_CONFIRMED_CHANGED
            else -> MissionAnswerState.SAVED
        }
        // The answer's own ACCEPTED receipt proves the state at its revision until a later copy shows a later one.
        val proven = receipt?.entityRevision?.takeIf { answer == MissionAnswerState.ACCEPTED || answer == MissionAnswerState.DECLINE_RECORDED }
        val state = when {
            item == null -> null
            proven != null && proven >= item.revision ->
                if (line!!.response == TripMissionResponse.ACCEPT) TripMissionState.ACCEPTED else TripMissionState.DECLINED
            else -> item.state
        }
        val trip = trips?.missionTrip(missionId)
        val actions = mutableSetOf<MissionAction>()
        var blocked: MissionLocalFailure? = null
        if (item != null && state == TripMissionState.ASSIGNED &&
            item.revision !in (if (line != null && line.missionId == missionId) listOf(line.revision) else emptyList())) {
            actions += MissionAction.ACCEPT; actions += MissionAction.DECLINE
        }
        if (trip != null) {
            actions += MissionAction.OPEN_TRIP
            if (state == TripMissionState.ACCEPTED || state == TripMissionState.IN_PROGRESS) blocked = MissionLocalFailure.ALREADY_STARTED
        } else if (state == TripMissionState.ACCEPTED) {
            actions += MissionAction.START_TRIP
        } else if (state == TripMissionState.ASSIGNED && line?.response == TripMissionResponse.ACCEPT && line.revision == item?.revision) {
            blocked = MissionLocalFailure.NOT_CONFIRMED
        }
        return MissionView(missionId, item, state, answer, line?.response, receipt?.code, actions, blocked, trip?.tripId)
    }
}

/** The answer synchronization state a receipt proves, for one retained entry. */
internal fun EnterpriseSyncStatus.final(): Boolean = this == EnterpriseSyncStatus.ACCEPTED || this == EnterpriseSyncStatus.NEEDS_REVIEW ||
    this == EnterpriseSyncStatus.REJECTED
