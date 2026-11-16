package com.anpfuel.domain.enterprise.tracking

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncJson
import com.anpfuel.domain.enterprise.sync.SyncProtocolException

/*
 * The Android producer's signed tracking session operations (EMP-GPS-002, B-BR-EMP-497, EDR-322): the exact schema 1
 * payloads that the EMP-GPS-004 intake parses (ParseSession), pinned by
 * contracts/testdata/enterprise/tracking/session-operations-v1.psv. The builders and the decoder check shape only, like
 * the intake parser; the explicit-start rule lives in the session domain and the service.
 */

/** The intake refusal class of a builder or decoder refusal (always MALFORMED for tracking). */
class TrackingOperationException : IllegalArgumentException("tracking operation: malformed")

/** The envelope facts of one device-produced operation; the binding is the creation partition (BR-OFFL-003). */
class TrackingEnvelope(val binding: SyncBinding, val membershipId: String, val enrollmentId: String, val operationId: String,
    val declared: TimeDeclaration) {
    override fun toString(): String = "EnterpriseTrackingEnvelope(redacted)"
}

/** One decoded device session operation; it proves only its own shape. */
sealed class TrackingOperationFact(val type: String, val operationId: String, val declared: TimeDeclaration, dependsOn: List<String>,
    val sessionId: String, val tripId: String) {
    private val dependencies = dependsOn.toList()
    val dependsOn: List<String> get() = dependencies.toList()
    class DriverAction(type: String, operationId: String, declared: TimeDeclaration, dependsOn: List<String>, sessionId: String, tripId: String,
        val origin: TrackingOrigin, val permission: LocationPermission, val notificationVisible: Boolean) :
        TrackingOperationFact(type, operationId, declared, dependsOn, sessionId, tripId)
    class Event(type: String, operationId: String, declared: TimeDeclaration, dependsOn: List<String>, sessionId: String, tripId: String,
        val cause: TrackingCause) : TrackingOperationFact(type, operationId, declared, dependsOn, sessionId, tripId)
    final override fun toString(): String = "EnterpriseTrackingOperationFact(redacted)"
}

private fun malformed() = TrackingOperationException()
private fun identity(raw: String): String = try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw malformed() }
private fun text(value: String) = SyncJson.Text(value)
private val DRIVER_KEYS = setOf("trip_id", "origin", "permission", "notification_visible")
private val EVENT_KEYS = setOf("trip_id", "cause")

object TrackingOperations {
    const val START = "gps.session.start"
    const val RESUME = "gps.session.resume"
    const val SUSPEND = "gps.session.suspend"
    const val INTERRUPT = "gps.session.interrupt"
    const val REDUCE = "gps.session.reduce"
    const val END = "gps.session.end"
    const val ENTITY = "tracking_session"
    /** At most the previous session operation and, for an end that follows a finish, the finish. */
    const val MAX_DEPENDENCIES = 2
    val SESSION_TYPES: Set<String> = setOf(START, RESUME, SUSPEND, INTERRUPT, REDUCE, END)
    private val EVENT_TYPES = setOf(SUSPEND, INTERRUPT, REDUCE, END)

    private fun origin(raw: String) = try { TrackingOrigin.parse(raw) } catch (_: TrackingDomainException) { throw malformed() }
    private fun permission(raw: String) = try { LocationPermission.parse(raw) } catch (_: TrackingDomainException) { throw malformed() }
    /** Any session cause except NOT_STARTED parses; which command owns it is the domain's decision (EDR-274). */
    private fun cause(raw: String): TrackingCause {
        val cause = try { TrackingCause.parse(raw) } catch (_: TrackingDomainException) { throw malformed() }
        if (cause == TrackingCause.NOT_STARTED) throw malformed()
        return cause
    }
    /** The intake keeps whole microseconds (EDR-279). */
    private fun declared(value: TimeDeclaration): TimeDeclaration {
        if (value.occurredAt.nano % 1000 != 0) throw malformed()
        return value
    }
    private fun dependencies(values: List<String>): List<String> {
        if (values.size > MAX_DEPENDENCIES) throw malformed()
        return values.map(::identity)
    }

    private fun seal(envelope: TrackingEnvelope, type: String, sessionId: String, dependsOn: List<String>,
        payload: Map<String, SyncJson>): EnterpriseOperation = try {
        val declared = declared(envelope.declared)
        val root = SyncJson.Object(mapOf(
            "schema_version" to SyncJson.Integer("1"), "operation_id" to text(envelope.operationId),
            "owner_account_id" to text(envelope.binding.ownerAccountId), "api_origin" to text(envelope.binding.apiOrigin),
            "org_id" to text(envelope.binding.organizationId), "membership_id" to text(envelope.membershipId),
            "enrollment_id" to text(envelope.enrollmentId), "type" to text(type), "operation_schema" to SyncJson.Integer("1"),
            "entity" to SyncJson.Object(mapOf("type" to text(ENTITY), "id" to text(identity(sessionId)), "expected_revision" to SyncJson.Null)),
            "occurred_at" to text(EnterpriseTime.formatInstant(declared.occurredAt)), "entry_zone" to text(declared.entryZone),
            "device_offset_seconds" to SyncJson.Integer(declared.deviceOffsetSeconds.toString()),
            "depends_on" to SyncJson.Array(dependencies(dependsOn).map(::text)),
            "soft_references" to SyncJson.Array(emptyList()), "attachments" to SyncJson.Array(emptyList()),
            "payload" to SyncJson.Object(payload),
        ))
        EnterpriseOperation.seal(CanonicalJson.encode(root, CanonicalJson.MAX_OPERATION_DEPTH), envelope.binding)
    } catch (_: SyncProtocolException) { throw malformed() }

    private fun driver(type: String, envelope: TrackingEnvelope, sessionId: String, tripId: String, origin: String, permission: String,
        notificationVisible: Boolean, dependsOn: List<String>): EnterpriseOperation {
        val trip = identity(tripId)
        val o = origin(origin)
        val p = permission(permission)
        return seal(envelope, type, sessionId, dependsOn, mapOf("trip_id" to text(trip), "origin" to text(o.name),
            "permission" to text(p.name), "notification_visible" to SyncJson.Bool(notificationVisible)))
    }

    /** `gps.session.start`: the first start depends on the trip's `trip.start`, a retry on the previous session operation. */
    fun start(envelope: TrackingEnvelope, sessionId: String, tripId: String, origin: String, permission: String,
        notificationVisible: Boolean, dependsOn: List<String>): EnterpriseOperation =
        driver(START, envelope, sessionId, tripId, origin, permission, notificationVisible, dependsOn)

    /** `gps.session.resume`: depends on the previous session operation. */
    fun resume(envelope: TrackingEnvelope, sessionId: String, tripId: String, origin: String, permission: String,
        notificationVisible: Boolean, dependsOn: List<String>): EnterpriseOperation =
        driver(RESUME, envelope, sessionId, tripId, origin, permission, notificationVisible, dependsOn)

    /** `gps.session.suspend|interrupt|reduce|end` with its typed cause. */
    fun event(type: String, envelope: TrackingEnvelope, sessionId: String, tripId: String, cause: String,
        dependsOn: List<String>): EnterpriseOperation {
        if (type !in EVENT_TYPES) throw malformed()
        val trip = identity(tripId)
        val c = cause(cause)
        return seal(envelope, type, sessionId, dependsOn, mapOf("trip_id" to text(trip), "cause" to text(c.name)))
    }

    private fun exact(value: SyncJson.Object, keys: Set<String>): SyncJson.Object {
        if (value.fields.keys != keys) throw malformed()
        return value
    }
    private fun field(payload: SyncJson.Object, key: String): String = (payload.fields[key] as? SyncJson.Text)?.value ?: throw malformed()

    /** Strict decode of a device session operation; anything else is MALFORMED. */
    fun read(operation: EnterpriseOperation): TrackingOperationFact {
        val type = operation.type
        val entity = operation.entity
        if (type !in SESSION_TYPES || !operation.supportsSchema || entity.reference.type != ENTITY || entity.expectedRevision != null ||
            operation.attachments.isNotEmpty() || operation.softReferences.isNotEmpty() || operation.dependsOn.size > MAX_DEPENDENCIES) throw malformed()
        val declared = declared(operation.declaration)
        val session = identity(entity.reference.id)
        val payload = operation.payload
        if (type == START || type == RESUME) {
            val fields = exact(payload, DRIVER_KEYS)
            val visible = (fields.fields["notification_visible"] as? SyncJson.Bool)?.value ?: throw malformed()
            return TrackingOperationFact.DriverAction(type, operation.operationId, declared, operation.dependsOn, session,
                identity(field(fields, "trip_id")), origin(field(fields, "origin")), permission(field(fields, "permission")), visible)
        }
        val fields = exact(payload, EVENT_KEYS)
        return TrackingOperationFact.Event(type, operation.operationId, declared, operation.dependsOn, session,
            identity(field(fields, "trip_id")), cause(field(fields, "cause")))
    }
}
