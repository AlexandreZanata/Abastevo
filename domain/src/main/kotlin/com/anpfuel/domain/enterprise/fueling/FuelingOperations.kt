package com.anpfuel.domain.enterprise.fueling

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
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

/*
 * The Android producer's signed fueling (EMP-FUEL-011, B-BR-EMP-455, EDR-294): the exact schema 1 payload of
 * EMP-FUEL-002 (EDR-175), pinned by contracts/testdata/enterprise/fueling/fueling-submit-v1.psv against the real Go
 * intake parser. Recognised forms whose owners do not exist yet are refused UNSUPPORTED, exactly like the intake.
 */

/** The intake refusal class a builder or decoder refusal corresponds to. */
enum class FuelingOperationFailure { MALFORMED, UNSUPPORTED }

class FuelingOperationException(val failure: FuelingOperationFailure = FuelingOperationFailure.MALFORMED) :
    IllegalArgumentException("fueling operation: ${failure.name.lowercase()}")

/** The envelope facts of one device-produced fueling; the binding is the creation partition (BR-OFFL-003). */
class FuelingEnvelope(val binding: SyncBinding, val membershipId: String, val enrollmentId: String, val operationId: String,
    val declared: TimeDeclaration) {
    override fun toString(): String = "EnterpriseFuelingEnvelope(redacted)"
}

/** A captured reading in wire form (base units as canonical decimal text); the builder validates it. */
class FuelingReadingInput(val meterId: String, val segmentId: String, val readingId: String, val kind: String, val value: String) {
    override fun toString(): String = "EnterpriseFuelingReadingInput(redacted)"
}

/** The 17 payload facts in wire form; optional fields are null when absent (EDR-175). */
class FuelingSubmitInput(
    val assetId: String,
    val responsibleEmployeeId: String,
    val product: String,
    val unit: String,
    val quantity: String,
    val paidTotal: String,
    val discount: String? = null,
    val unitPrice: String? = null,
    val unitPriceText: String? = null,
    val fill: String,
    val payer: String,
    val paymentMethod: String,
    val reading: FuelingReadingInput? = null,
    val missingReadingReason: String? = null,
    /** `NOT_FOUND` with [placeName], or a retained kind the intake answers UNSUPPORTED. */
    val placeKind: String? = null,
    val placeName: String? = null,
    /** Retained references: never produced by the app (their owners do not exist yet). */
    val tripId: String? = null,
    val contractedSupplierId: String? = null,
) {
    override fun toString(): String = "EnterpriseFuelingSubmitInput(redacted)"
}

/** The validated reading evidence of a fueling. */
sealed class FuelingEvidence {
    class Reading(val meterId: String, val segmentId: String, val readingId: String, val value: MeterValue) : FuelingEvidence()
    class Absent(val reason: String) : FuelingEvidence()
    final override fun toString(): String = "EnterpriseFuelingEvidence(redacted)"
}

/** One decoded device fueling; it proves only its own shape. */
class FuelingFact(
    val operationId: String,
    val declared: TimeDeclaration,
    val fuelingId: String,
    val assetId: String,
    val responsibleEmployeeId: String,
    val quantity: ProductQuantity,
    val amounts: RecordedAmounts,
    val fill: FillLevel,
    val payer: Payer,
    val payment: PaymentMethod,
    val evidence: FuelingEvidence,
    val placeName: String?,
) {
    override fun toString(): String = "EnterpriseFuelingFact(redacted)"
}

private fun malformed() = FuelingOperationException(FuelingOperationFailure.MALFORMED)
private fun unsupported() = FuelingOperationException(FuelingOperationFailure.UNSUPPORTED)
private fun identity(raw: String): String = try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw malformed() }
private fun canonical(raw: String): Boolean = try { EnterprisePartition.requireId(raw); true } catch (_: IllegalArgumentException) { false }

private const val ENTITY = "fueling"
private val PAYLOAD_KEYS = setOf("asset_id", "responsible_employee_id", "product", "unit", "quantity", "paid_total", "discount", "unit_price",
    "unit_price_text", "fill", "payer", "payment_method", "reading", "missing_reading_reason", "place", "trip_id", "contracted_supplier_id")
private val READING_KEYS = setOf("meter_id", "segment_id", "reading_id", "kind", "value")
private val RETAINED_PLACES = setOf(PlaceKind.CATALOG_STATION.name, PlaceKind.ORG_SUPPLIER.name, PlaceKind.INTERNAL_TANK.name)

/** The validated payload, in the Go intake's order: shape refusals, then candidate refusals, then retained forms. */
private class Checked(val quantity: ProductQuantity, val amounts: RecordedAmounts, val fill: FillLevel, val payer: Payer, val payment: PaymentMethod,
    val evidence: FuelingEvidence, val placeName: String?, val unsupported: Boolean)

private fun check(input: FuelingSubmitInput): Checked {
    var unsupported = false
    fun retained(value: String?) { if (value != null) { if (canonical(value)) unsupported = true else throw malformed() } }
    retained(input.tripId)
    retained(input.contractedSupplierId)
    if (!canonical(input.assetId) || !canonical(input.responsibleEmployeeId)) throw malformed()
    val text = input.unitPriceText
    if (text != null && (input.unitPrice == null || text.isEmpty() || text.contains('\u0000'))) throw malformed()
    val reading = input.reading?.let {
        val kind = try { MeterKind.parse(it.kind) } catch (_: MeterDomainException) { throw malformed() }
        val value = try { MeterValues.parse(kind, MeterEntryForm.BASE, it.value) } catch (_: MeterDomainException) { throw malformed() }
        FuelingEvidence.Reading(identity(it.meterId), identity(it.segmentId), identity(it.readingId), value)
    }
    var placeName: String? = null
    when (val kind = input.placeKind) {
        null -> if (input.placeName != null) throw malformed()
        PlaceKind.NOT_FOUND.name -> placeName = input.placeName ?: throw malformed()
        in RETAINED_PLACES -> unsupported = true
        else -> throw malformed()
    }
    // The FUEL-001 candidate rules the intake applies next.
    try {
        val quantity = ProductQuantity.parse(input.product, input.unit, input.quantity)
        val amounts = RecordedAmounts.parse(input.paidTotal, input.discount, input.unitPrice, text)
        val fill = FillLevel.parse(input.fill)
        val payer = Payer.parse(input.payer)
        val payment = PaymentMethod.parse(input.paymentMethod)
        if ((reading == null) == (input.missingReadingReason == null)) throw malformed()
        val evidence = reading ?: FuelingEvidence.Absent(MeterPresence.missing(input.missingReadingReason!!).reason)
        placeName?.let { Establishment.NotFound(it) }
        return Checked(quantity, amounts, fill, payer, payment, evidence, placeName, unsupported)
    } catch (_: FuelDomainException) { throw malformed() }
    catch (_: MeterDomainException) { throw malformed() }
}

private fun text(value: String) = SyncJson.Text(value)
private fun optional(value: String?): SyncJson = value?.let(::text) ?: SyncJson.Null

private fun field(payload: SyncJson.Object, key: String): String? = when (val value = payload.fields[key]) {
    SyncJson.Null -> null
    is SyncJson.Text -> value.value
    else -> throw malformed()
}
private fun required(payload: SyncJson.Object, key: String): String = field(payload, key) ?: throw malformed()
private fun exact(value: SyncJson?, keys: Set<String>): SyncJson.Object {
    val obj = value as? SyncJson.Object ?: throw malformed()
    if (obj.fields.keys != keys) throw malformed()
    return obj
}

object FuelingOperations {
    const val TYPE = "fueling.submit"

    /** `fueling.submit` schema 1 for [fuelingId]: no dependency, reference or envelope attachment (EDR-175, EDR-269). */
    fun submit(envelope: FuelingEnvelope, fuelingId: String, input: FuelingSubmitInput): EnterpriseOperation {
        val id = identity(fuelingId)
        val checked = check(input)
        if (checked.unsupported) throw unsupported()
        val reading = (checked.evidence as? FuelingEvidence.Reading)?.let {
            SyncJson.Object(mapOf("meter_id" to text(it.meterId), "segment_id" to text(it.segmentId), "reading_id" to text(it.readingId),
                "kind" to text(it.value.kind.name), "value" to text(it.value.units.toString())))
        } ?: SyncJson.Null
        val reason = (checked.evidence as? FuelingEvidence.Absent)?.reason
        val place = checked.placeName?.let { SyncJson.Object(mapOf("kind" to text(PlaceKind.NOT_FOUND.name), "name" to text(it))) } ?: SyncJson.Null
        val payload = mapOf("asset_id" to text(input.assetId), "responsible_employee_id" to text(input.responsibleEmployeeId),
            "product" to text(input.product), "unit" to text(input.unit), "quantity" to text(input.quantity), "paid_total" to text(input.paidTotal),
            "discount" to optional(input.discount), "unit_price" to optional(input.unitPrice), "unit_price_text" to optional(input.unitPriceText),
            "fill" to text(input.fill), "payer" to text(input.payer), "payment_method" to text(input.paymentMethod), "reading" to reading,
            "missing_reading_reason" to optional(reason), "place" to place, "trip_id" to SyncJson.Null, "contracted_supplier_id" to SyncJson.Null)
        return try {
            val declared = envelope.declared
            val root = SyncJson.Object(mapOf(
                "schema_version" to SyncJson.Integer("1"), "operation_id" to text(envelope.operationId),
                "owner_account_id" to text(envelope.binding.ownerAccountId), "api_origin" to text(envelope.binding.apiOrigin),
                "org_id" to text(envelope.binding.organizationId), "membership_id" to text(envelope.membershipId),
                "enrollment_id" to text(envelope.enrollmentId), "type" to text(TYPE), "operation_schema" to SyncJson.Integer("1"),
                "entity" to SyncJson.Object(mapOf("type" to text(ENTITY), "id" to text(id), "expected_revision" to SyncJson.Null)),
                "occurred_at" to text(EnterpriseTime.formatInstant(declared.occurredAt)), "entry_zone" to text(declared.entryZone),
                "device_offset_seconds" to SyncJson.Integer(declared.deviceOffsetSeconds.toString()),
                "depends_on" to SyncJson.Array(emptyList()), "soft_references" to SyncJson.Array(emptyList()),
                "attachments" to SyncJson.Array(emptyList()), "payload" to SyncJson.Object(payload),
            ))
            EnterpriseOperation.seal(CanonicalJson.encode(root, CanonicalJson.MAX_OPERATION_DEPTH), envelope.binding)
        } catch (_: SyncProtocolException) { throw malformed() }
    }

    /** Strict decode mirroring the Go intake parser; anything else is refused with its intake class. */
    fun read(operation: EnterpriseOperation): FuelingFact {
        val entity = operation.entity
        if (operation.type != TYPE || !operation.supportsSchema || entity.reference.type != ENTITY || entity.expectedRevision != null ||
            !canonical(entity.reference.id)) throw malformed()
        val fields = exact(operation.payload, PAYLOAD_KEYS)
        val rawReading = fields.fields["reading"]
        val reading = if (rawReading === SyncJson.Null) null else exact(rawReading, READING_KEYS).let {
            FuelingReadingInput(required(it, "meter_id"), required(it, "segment_id"), required(it, "reading_id"), required(it, "kind"), required(it, "value"))
        }
        val rawPlace = fields.fields["place"]
        var placeKind: String? = null
        var placeName: String? = null
        if (rawPlace !== SyncJson.Null) {
            val place = rawPlace as? SyncJson.Object ?: throw malformed()
            placeKind = required(place, "kind")
            if (placeKind == PlaceKind.NOT_FOUND.name) placeName = required(exact(place, setOf("kind", "name")), "name")
        }
        val input = FuelingSubmitInput(required(fields, "asset_id"), required(fields, "responsible_employee_id"), required(fields, "product"),
            required(fields, "unit"), required(fields, "quantity"), required(fields, "paid_total"), field(fields, "discount"),
            field(fields, "unit_price"), field(fields, "unit_price_text"), required(fields, "fill"), required(fields, "payer"),
            required(fields, "payment_method"), reading, field(fields, "missing_reading_reason"), placeKind, placeName,
            field(fields, "trip_id"), field(fields, "contracted_supplier_id"))
        val checked = check(input)
        if (checked.unsupported || operation.attachments.isNotEmpty() || operation.dependsOn.isNotEmpty() || operation.softReferences.isNotEmpty())
            throw unsupported()
        return FuelingFact(operation.operationId, operation.declaration, entity.reference.id, input.assetId, input.responsibleEmployeeId,
            checked.quantity, checked.amounts, checked.fill, checked.payer, checked.payment, checked.evidence, checked.placeName)
    }
}
