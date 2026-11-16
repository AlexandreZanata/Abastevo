package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncJson
import com.anpfuel.domain.enterprise.sync.SyncProtocolException

/*
 * The member's answer to an assigned mission (EMP-TRIP-021, B-BR-EMP-528, EDR-343): the frozen EMP-TRIP-008
 * `trip.mission.respond` schema 1 (entity `mission` at the expected revision, payload exactly `response` and `reason`),
 * pinned by contracts/testdata/enterprise/trip/trip-operations-v2.psv. It decides nothing: the server decides the
 * answer on current facts at receipt (EDR-229).
 */

/** One decoded answer; it proves only its own shape. */
class TripMissionResponseFact(val operationId: String, val declared: TimeDeclaration, val missionId: String, val revision: Long,
    val response: TripMissionResponse, val reason: String?) {
    override fun toString(): String = "EnterpriseTripMissionResponseFact(redacted)"
}

object TripMissionOperations {
    const val RESPOND = "trip.mission.respond"
    const val ENTITY_MISSION = "mission"

    private fun malformed() = TripOperationException(TripOperationFailure.MALFORMED)
    private fun answer(raw: String, reason: String?): Pair<TripMissionResponse, String?> {
        val response = try { TripMissionResponse.parse(raw) } catch (_: TripMissionException) { throw malformed() }
        when (response) {
            TripMissionResponse.ACCEPT -> if (reason != null) throw malformed()
            TripMissionResponse.DECLINE -> try { TripMissionText.validate(reason ?: throw malformed()) } catch (_: TripMissionException) { throw malformed() }
        }
        return response to reason
    }

    /** An acceptance carries no reason; a decline needs one in the METER-001 text profile. Refusals are MALFORMED. */
    fun respond(envelope: TripEnvelope, missionId: String, expectedRevision: Long, response: String, reason: String?): EnterpriseOperation {
        val mission = try { EnterprisePartition.requireId(missionId) } catch (_: IllegalArgumentException) { throw malformed() }
        val (kind, why) = answer(response, reason)
        val declared = envelope.declared
        val root = SyncJson.Object(mapOf(
            "schema_version" to SyncJson.Integer("1"), "operation_id" to SyncJson.Text(envelope.operationId),
            "owner_account_id" to SyncJson.Text(envelope.binding.ownerAccountId), "api_origin" to SyncJson.Text(envelope.binding.apiOrigin),
            "org_id" to SyncJson.Text(envelope.binding.organizationId), "membership_id" to SyncJson.Text(envelope.membershipId),
            "enrollment_id" to SyncJson.Text(envelope.enrollmentId), "type" to SyncJson.Text(RESPOND), "operation_schema" to SyncJson.Integer("1"),
            "entity" to SyncJson.Object(mapOf("type" to SyncJson.Text(ENTITY_MISSION), "id" to SyncJson.Text(mission),
                "expected_revision" to SyncJson.Integer(expectedRevision.toString()))),
            "occurred_at" to SyncJson.Text(EnterpriseTime.formatInstant(declared.occurredAt)), "entry_zone" to SyncJson.Text(declared.entryZone),
            "device_offset_seconds" to SyncJson.Integer(declared.deviceOffsetSeconds.toString()),
            "depends_on" to SyncJson.Array(emptyList()), "soft_references" to SyncJson.Array(emptyList()), "attachments" to SyncJson.Array(emptyList()),
            "payload" to SyncJson.Object(mapOf("response" to SyncJson.Text(kind.name), "reason" to (why?.let { SyncJson.Text(it) } ?: SyncJson.Null))),
        ))
        return try { EnterpriseOperation.seal(CanonicalJson.encode(root, CanonicalJson.MAX_OPERATION_DEPTH), envelope.binding) }
        catch (_: SyncProtocolException) { throw malformed() }
    }

    /** Strict decode mirroring the Go `ParseRespond`; anything else is MALFORMED. */
    fun read(operation: EnterpriseOperation): TripMissionResponseFact {
        val entity = operation.entity
        val revision = entity.expectedRevision
        if (operation.type != RESPOND || !operation.supportsSchema || entity.reference.type != ENTITY_MISSION || revision == null || revision < 1 ||
            operation.dependsOn.isNotEmpty() || operation.softReferences.isNotEmpty() || operation.attachments.isNotEmpty()) throw malformed()
        val fields = operation.payload.fields
        if (fields.keys != setOf("response", "reason")) throw malformed()
        val raw = (fields["response"] as? SyncJson.Text)?.value ?: throw malformed()
        val reason = when (val value = fields["reason"]) { SyncJson.Null -> null; is SyncJson.Text -> value.value; else -> throw malformed() }
        val (kind, why) = answer(raw, reason)
        try { EnterpriseTime.validateDeclaration(operation.declaration) } catch (_: EnterpriseTimeException) { throw malformed() }
        return TripMissionResponseFact(operation.operationId, operation.declaration, entity.reference.id, revision, kind, why)
    }
}
