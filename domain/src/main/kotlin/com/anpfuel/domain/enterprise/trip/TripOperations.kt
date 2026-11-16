package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.meter.MeterDomainException
import com.anpfuel.domain.enterprise.meter.MeterEntryForm
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.meter.MeterPresence
import com.anpfuel.domain.enterprise.meter.MeterValue
import com.anpfuel.domain.enterprise.meter.MeterValues
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncJson
import com.anpfuel.domain.enterprise.sync.SyncProtocolException
import java.time.Instant
import java.time.ZoneId

/*
 * The Android producer's signed trip operations (EMP-TRIP-005, B-BR-EMP-375, EDR-245): the exact schema 1
 * payloads of EMP-TRIP-002/003/004, pinned by contracts/testdata/enterprise/trip/trip-operations-v3.psv; a start may name its
 * departure checklist run and a finish its return run through one `checklist_run` soft reference (EMP-TRIP-019, EDR-331); a
 * start may name the mission it executes through one `mission` soft reference (EMP-TRIP-023), and the EMP-TRIP-011
 * lifecycle commands are their own operations (EMP-TRIP-021, B-BR-EMP-529/530, EDR-344).
 */

/** The intake refusal class a builder or decoder refusal corresponds to. */
enum class TripOperationFailure { MALFORMED, UNSUPPORTED }

class TripOperationException(val failure: TripOperationFailure = TripOperationFailure.MALFORMED) :
    IllegalArgumentException("trip operation: ${failure.name.lowercase()}")

/** The envelope facts of one device-produced operation; the binding is the creation partition (BR-OFFL-003). */
class TripEnvelope(val binding: SyncBinding, val membershipId: String, val enrollmentId: String, val operationId: String,
    val declared: TimeDeclaration) {
    override fun toString(): String = "EnterpriseTripEnvelope(redacted)"
}

/** A captured reading in wire form (base units as canonical decimal text); the builders validate it. */
class TripReadingInput(val meterId: String, val segmentId: String, val readingId: String, val kind: String, val value: String) {
    override fun toString(): String = "EnterpriseTripReadingInput(redacted)"
}

/** A raw run-clock observation (B-BR-EMP-378); the builders validate it as a [TripRunClock]. */
class TripClockReading(val bootId: String, val elapsedMillis: Long) {
    override fun toString(): String = "EnterpriseTripClockReading(redacted)"
}

/** The validated departure or final evidence of a start or finish. */
sealed class TripEvidence {
    class Reading(val meterId: String, val segmentId: String, val readingId: String, val value: MeterValue) : TripEvidence()
    class Absent(val reason: String) : TripEvidence()
    final override fun toString(): String = "EnterpriseTripEvidence(redacted)"
}

/** One decoded device trip operation; it proves only its own shape. */
sealed class TripOperationFact(val operationId: String, val declared: TimeDeclaration, dependsOn: List<String>) {
    private val dependencies = dependsOn.toList()
    val dependsOn: List<String> get() = dependencies.toList()
    /** [missionId] is the EMP-TRIP-023 soft reference of a planned trip's start, absent for an ordinary trip. */
    class Start(operationId: String, declared: TimeDeclaration, dependsOn: List<String>, val tripId: String, val assetId: String,
        val driverEmployeeId: String, val evidence: TripEvidence, val clock: TripRunClock?, val departureRunId: String? = null,
        val missionId: String? = null) :
        TripOperationFact(operationId, declared, dependsOn)
    class Event(operationId: String, declared: TimeDeclaration, dependsOn: List<String>, val eventId: String, val tripId: String,
        val type: TripLogbookType, val text: String?) : TripOperationFact(operationId, declared, dependsOn)
    class Finish(operationId: String, declared: TimeDeclaration, dependsOn: List<String>, val tripId: String,
        val evidence: TripEvidence, val clock: TripRunClock?, val returnRunId: String? = null) : TripOperationFact(operationId, declared, dependsOn)
    class Submit(operationId: String, declared: TimeDeclaration, dependsOn: List<String>, val tripId: String,
        val revision: Long) : TripOperationFact(operationId, declared, dependsOn)
    /** One EMP-TRIP-011 command ([TripCommand.PAUSE], [TripCommand.RESUME] or [TripCommand.CANCEL]) at its expected revision. */
    class Lifecycle(operationId: String, declared: TimeDeclaration, dependsOn: List<String>, val command: TripCommand, val tripId: String,
        val revision: Long, val reason: String?) : TripOperationFact(operationId, declared, dependsOn)
    final override fun toString(): String = "EnterpriseTripOperationFact(redacted)"
}

private fun malformed() = TripOperationException(TripOperationFailure.MALFORMED)
private fun unsupported() = TripOperationException(TripOperationFailure.UNSUPPORTED)
private fun identity(raw: String): String = try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw malformed() }
private fun reason(raw: String): String = try { MeterPresence.missing(raw).reason } catch (_: MeterDomainException) { throw malformed() }

private const val ENTITY_TRIP = "trip"
private const val ENTITY_EVENT = "trip_event"
private const val ENTITY_REVIEW = "trip_review"
private val START_KEYS = setOf("asset_id", "driver_employee_id", "reading", "missing_reading_reason", "run_clock")
private val FINISH_KEYS = setOf("reading", "missing_reading_reason", "run_clock")
private val EVENT_KEYS = setOf("trip_id", "type", "text")
private val READING_KEYS = setOf("meter_id", "segment_id", "reading_id", "kind", "value")
private val CLOCK_KEYS = setOf("boot_id", "elapsed_millis")

/** Exactly one of a reading and a reason, both checked with the METER-001 profiles the intake uses. */
private fun evidence(reading: TripReadingInput?, missingReason: String?): TripEvidence {
    if ((reading == null) == (missingReason == null)) throw malformed()
    if (reading == null) return TripEvidence.Absent(reason(missingReason!!))
    val kind = try { MeterKind.parse(reading.kind) } catch (_: MeterDomainException) { throw malformed() }
    val value = try { MeterValues.parse(kind, MeterEntryForm.BASE, reading.value) } catch (_: MeterDomainException) { throw malformed() }
    return TripEvidence.Reading(identity(reading.meterId), identity(reading.segmentId), identity(reading.readingId), value)
}

private fun clock(raw: TripClockReading?): TripRunClock? =
    raw?.let { try { TripRunClock(it.bootId, it.elapsedMillis) } catch (_: TripDomainException) { throw malformed() } }

private fun text(value: String) = SyncJson.Text(value)
private fun optional(value: String?): SyncJson = value?.let(::text) ?: SyncJson.Null

private fun evidenceJson(value: TripEvidence): Pair<SyncJson, SyncJson> = when (value) {
    is TripEvidence.Reading -> SyncJson.Object(mapOf("meter_id" to text(value.meterId), "segment_id" to text(value.segmentId),
        "reading_id" to text(value.readingId), "kind" to text(value.value.kind.name), "value" to text(value.value.units.toString()))) to SyncJson.Null
    is TripEvidence.Absent -> SyncJson.Null to text(value.reason)
}

private fun clockJson(value: TripRunClock?): SyncJson = value?.let {
    SyncJson.Object(mapOf("boot_id" to text(it.bootId), "elapsed_millis" to text(it.elapsedMillis.toString())))
} ?: SyncJson.Null

/** The soft-reference type naming a checklist run (EMP-TRIP-019, EDR-331). */
private const val CHECKLIST_RUN_REFERENCE = "checklist_run"
/** The soft-reference type naming the mission a start executes (EMP-TRIP-023, EMP-TRIP-021 EDR-344). */
private const val MISSION_REFERENCE_TYPE = "mission"

/**
 * Seals the canonical envelope; any envelope-profile refusal is a malformed record that is never queued. [checklistRun] is the
 * operation's one `checklist_run` soft reference and [mission] its one `mission` soft reference, if any (canonical set order).
 */
private fun seal(envelope: TripEnvelope, type: String, entityType: String, entityId: String, revision: Long?, dependsOn: String?,
    payload: Map<String, SyncJson>, checklistRun: String? = null, mission: String? = null): EnterpriseOperation = try {
    val declared = envelope.declared
    val root = SyncJson.Object(mapOf(
        "schema_version" to SyncJson.Integer("1"), "operation_id" to text(envelope.operationId),
        "owner_account_id" to text(envelope.binding.ownerAccountId), "api_origin" to text(envelope.binding.apiOrigin),
        "org_id" to text(envelope.binding.organizationId), "membership_id" to text(envelope.membershipId),
        "enrollment_id" to text(envelope.enrollmentId), "type" to text(type), "operation_schema" to SyncJson.Integer("1"),
        "entity" to SyncJson.Object(mapOf("type" to text(entityType), "id" to text(entityId),
            "expected_revision" to (revision?.let { SyncJson.Integer(it.toString()) } ?: SyncJson.Null))),
        "occurred_at" to text(EnterpriseTime.formatInstant(declared.occurredAt)), "entry_zone" to text(declared.entryZone),
        "device_offset_seconds" to SyncJson.Integer(declared.deviceOffsetSeconds.toString()),
        "depends_on" to SyncJson.Array(listOfNotNull(dependsOn).map(::text)),
        "soft_references" to SyncJson.Array(listOfNotNull(checklistRun?.let { CHECKLIST_RUN_REFERENCE to it },
            mission?.let { MISSION_REFERENCE_TYPE to it }).map { (kind, id) -> SyncJson.Object(mapOf("type" to text(kind), "id" to text(id))) }),
        "attachments" to SyncJson.Array(emptyList()),
        "payload" to SyncJson.Object(payload),
    ))
    EnterpriseOperation.seal(CanonicalJson.encode(root, CanonicalJson.MAX_OPERATION_DEPTH), envelope.binding)
} catch (_: SyncProtocolException) { throw malformed() }

/** A decoded payload value: canonical text, null, or a refusal. */
private fun field(payload: SyncJson.Object, key: String): String? = when (val value = payload.fields[key]) {
    SyncJson.Null -> null
    is SyncJson.Text -> value.value
    else -> throw malformed()
}
private fun exact(value: SyncJson?, keys: Set<String>): SyncJson.Object {
    val obj = value as? SyncJson.Object ?: throw malformed()
    if (obj.fields.keys != keys) throw malformed()
    return obj
}
private fun decodedEvidence(payload: SyncJson.Object): TripEvidence {
    val raw = payload.fields["reading"]
    val reading = if (raw === SyncJson.Null) null else exact(raw, READING_KEYS).let { r ->
        TripReadingInput(field(r, "meter_id") ?: throw malformed(), field(r, "segment_id") ?: throw malformed(),
            field(r, "reading_id") ?: throw malformed(), field(r, "kind") ?: throw malformed(), field(r, "value") ?: throw malformed())
    }
    return evidence(reading, field(payload, "missing_reading_reason"))
}
private fun decodedClock(payload: SyncJson.Object): TripRunClock? {
    val raw = payload.fields["run_clock"]
    if (raw === SyncJson.Null) return null
    val value = exact(raw, CLOCK_KEYS)
    val elapsed = field(value, "elapsed_millis") ?: throw malformed()
    // The canonical non-negative int64 text only, like the Go intake.
    if (elapsed.isEmpty() || elapsed.length > 19 || elapsed.length > 1 && elapsed[0] == '0' || elapsed.any { it !in '0'..'9' }) throw malformed()
    return clock(TripClockReading(field(value, "boot_id") ?: throw malformed(), elapsed.toLongOrNull() ?: throw malformed()))
}

object TripOperations {
    const val START = "trip.start"
    const val FINISH = "trip.finish"
    const val EVENT = "trip.event.record"
    const val SUBMIT = "trip.submit"
    const val PAUSE = "trip.pause"
    const val RESUME = "trip.resume"
    const val CANCEL = "trip.cancel"
    /** The only soft reference a start may carry (EMP-TRIP-023, EDR-265). */
    const val MISSION_REFERENCE = "mission"
    const val MAX_REVIEW_REVISION = 1_000_000L
    /** The EMP-TRIP-011 trip revision bound (`MaxTripRevision`). */
    const val MAX_TRIP_REVISION = 1_000_000L
    /** The lifecycle operation of each command. */
    val LIFECYCLE: Map<TripCommand, String> = mapOf(TripCommand.PAUSE to PAUSE, TripCommand.RESUME to RESUME, TripCommand.CANCEL to CANCEL)
    val TYPES: Set<String> = setOf(START, FINISH, EVENT, SUBMIT, PAUSE, RESUME, CANCEL)

    /**
     * `trip.start`: exactly one of [reading] and [missingReason]; no dependency (EDR-202); [departureRunId] names the departure
     * checklist run as the operation's one `checklist_run` soft reference (EMP-TRIP-019, B-BR-EMP-511); [missionId] adds the one
     * `{type: "mission", id}` soft reference of a planned trip (EMP-TRIP-023, EMP-TRIP-021 B-BR-EMP-529).
     */
    fun start(envelope: TripEnvelope, tripId: String, assetId: String, driverEmployeeId: String, reading: TripReadingInput?,
        missingReason: String?, clock: TripClockReading?, departureRunId: String? = null, missionId: String? = null): EnterpriseOperation {
        val trip = identity(tripId)
        val asset = identity(assetId)
        val driver = identity(driverEmployeeId)
        val (readingJson, reasonJson) = evidenceJson(evidence(reading, missingReason))
        val runClock = clock(clock)
        val run = departureRunId?.let(::identity)
        val mission = missionId?.let(::identity)
        return seal(envelope, START, ENTITY_TRIP, trip, null, null, mapOf("asset_id" to text(asset), "driver_employee_id" to text(driver),
            "reading" to readingJson, "missing_reading_reason" to reasonJson, "run_clock" to clockJson(runClock)), run, mission)
    }

    /** `trip.event.record`: the type token is checked like the intake (unknown MALFORMED, reserved UNSUPPORTED, then text). */
    fun event(envelope: TripEnvelope, eventId: String, tripId: String, type: String, text: String?, dependsOn: String?): EnterpriseOperation {
        val event = identity(eventId)
        val trip = identity(tripId)
        val kind = logbookType(type, text)
        return seal(envelope, EVENT, ENTITY_EVENT, event, null, dependsOn?.let(::identity),
            mapOf("trip_id" to text(trip), "type" to text(kind.name), "text" to optional(text)))
    }

    /** `trip.finish`: exactly one of [reading] and [missingReason], at most the start as dependency; [returnRunId] as on [start]. */
    fun finish(envelope: TripEnvelope, tripId: String, reading: TripReadingInput?, missingReason: String?, clock: TripClockReading?,
        dependsOn: String?, returnRunId: String? = null): EnterpriseOperation {
        val trip = identity(tripId)
        val (readingJson, reasonJson) = evidenceJson(evidence(reading, missingReason))
        val runClock = clock(clock)
        val run = returnRunId?.let(::identity)
        return seal(envelope, FINISH, ENTITY_TRIP, trip, null, dependsOn?.let(::identity),
            mapOf("reading" to readingJson, "missing_reading_reason" to reasonJson, "run_clock" to clockJson(runClock)), run)
    }

    /** `trip.submit`: entity `trip_review` at [revision], payload `{}`, at most the finish as dependency (EDR-220). */
    fun submit(envelope: TripEnvelope, tripId: String, revision: Long, dependsOn: String?): EnterpriseOperation {
        val trip = identity(tripId)
        if (revision !in 1..MAX_REVIEW_REVISION) throw malformed()
        return seal(envelope, SUBMIT, ENTITY_REVIEW, trip, revision, dependsOn?.let(::identity), emptyMap())
    }

    /**
     * `trip.pause`, `trip.resume` or `trip.cancel` (EMP-TRIP-011): entity `trip` at [expectedRevision], payload exactly
     * `reason` (required for a cancellation), at most the start as dependency.
     */
    fun lifecycle(envelope: TripEnvelope, command: TripCommand, tripId: String, expectedRevision: Long, reason: String?,
        dependsOn: String?): EnterpriseOperation {
        val type = LIFECYCLE[command] ?: throw malformed()
        val trip = identity(tripId)
        // Zero and below are refused by the envelope profile itself; above the trip bound by the intake.
        if (expectedRevision > MAX_TRIP_REVISION) throw malformed()
        val why = reason?.let(::reason)
        if (command == TripCommand.CANCEL && why == null) throw malformed()
        return seal(envelope, type, ENTITY_TRIP, trip, expectedRevision, dependsOn?.let(::identity), mapOf("reason" to optional(why)))
    }

    private fun logbookType(raw: String, text: String?): TripLogbookType {
        val kind = try { TripLogbookType.parse(raw) } catch (_: TripLogbookException) { throw malformed() }
        if (!kind.supported) throw unsupported()
        if (text == null) { if (kind.requiresText) throw malformed() } else reason(text)
        return kind
    }

    /** Strict decode of a device trip operation; anything else is refused with its intake class. */
    fun read(operation: EnterpriseOperation): TripOperationFact {
        if (operation.type !in TYPES || !operation.supportsSchema) throw malformed()
        val entity = operation.entity
        val payload = operation.payload
        val deps = operation.dependsOn
        val declared = operation.declaration
        val id = operation.operationId
        val extras = operation.attachments.isNotEmpty() || operation.softReferences.isNotEmpty()
        // EMP-TRIP-019 (B-BR-EMP-511): a start or a finish may carry exactly one checklist-run reference and nothing else.
        val references = operation.softReferences
        val checklistRun = references.singleOrNull()?.takeIf { it.type == CHECKLIST_RUN_REFERENCE }?.id
        val runExtras = operation.attachments.isNotEmpty() || references.isNotEmpty() && checklistRun == null
        // EMP-TRIP-021 (EDR-344): a start may also name its mission, at most one reference of each type (the Go ParseStart).
        val startRun = references.filter { it.type == CHECKLIST_RUN_REFERENCE }
        val startMission = references.filter { it.type == MISSION_REFERENCE_TYPE }
        val startExtras = operation.attachments.isNotEmpty() || startRun.size > 1 || startMission.size > 1 ||
            startRun.size + startMission.size != references.size
        return when (operation.type) {
            START -> {
                if (entity.reference.type != ENTITY_TRIP || entity.expectedRevision != null) throw malformed()
                val fields = exact(payload, START_KEYS)
                val asset = identity(field(fields, "asset_id") ?: throw malformed())
                val driver = identity(field(fields, "driver_employee_id") ?: throw malformed())
                val evidence = decodedEvidence(fields)
                val clock = decodedClock(fields)
                if (startExtras || deps.isNotEmpty()) throw unsupported()
                TripOperationFact.Start(id, declared, deps, entity.reference.id, asset, driver, evidence, clock, startRun.singleOrNull()?.id,
                    startMission.singleOrNull()?.id)
            }
            FINISH -> {
                if (entity.reference.type != ENTITY_TRIP || entity.expectedRevision != null || deps.size > 1) throw malformed()
                val fields = exact(payload, FINISH_KEYS)
                val evidence = decodedEvidence(fields)
                val clock = decodedClock(fields)
                if (runExtras) throw unsupported()
                TripOperationFact.Finish(id, declared, deps, entity.reference.id, evidence, clock, checklistRun)
            }
            EVENT -> {
                if (entity.reference.type != ENTITY_EVENT || entity.expectedRevision != null || deps.size > 1) throw malformed()
                val fields = exact(payload, EVENT_KEYS)
                val trip = identity(field(fields, "trip_id") ?: throw malformed())
                val note = field(fields, "text")
                val kind = logbookType(field(fields, "type") ?: throw malformed(), note)
                if (extras) throw unsupported()
                TripOperationFact.Event(id, declared, deps, entity.reference.id, trip, kind, note)
            }
            PAUSE, RESUME, CANCEL -> {
                // The Go ParseTransition order: shape and revision first, then the payload, then unsupported forms.
                val revision = entity.expectedRevision
                if (entity.reference.type != ENTITY_TRIP || revision == null || revision !in 1..MAX_TRIP_REVISION || deps.size > 1) throw malformed()
                val command = LIFECYCLE.entries.first { it.value == operation.type }.key
                val fields = exact(payload, setOf("reason"))
                val why = field(fields, "reason")?.let(::reason)
                if (command == TripCommand.CANCEL && why == null) throw malformed()
                if (extras) throw unsupported()
                TripOperationFact.Lifecycle(id, declared, deps, command, entity.reference.id, revision, why)
            }
            else -> {
                val revision = entity.expectedRevision
                if (entity.reference.type != ENTITY_REVIEW || revision == null || revision !in 1..MAX_REVIEW_REVISION || deps.size > 1 || extras ||
                    payload.fields.isNotEmpty()) throw malformed()
                TripOperationFact.Submit(id, declared, deps, entity.reference.id, revision)
            }
        }
    }
}

/** The device declaration of B-BR-EMP-378: wall instant, accepted zone identifier (else UTC) and its offset. */
object TripDeclarations {
    fun at(wallMillis: Long, zone: ZoneId): TimeDeclaration {
        val instant = Instant.ofEpochMilli(wallMillis)
        val offset = zone.rules.getOffset(instant).totalSeconds.toLong()
        val declared = TimeDeclaration(instant, zone.id, offset)
        return try { EnterpriseTime.validateDeclaration(declared) } catch (_: EnterpriseTimeException) { declared.copy(entryZone = "UTC") }
    }
}
