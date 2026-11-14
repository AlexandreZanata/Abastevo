package com.anpfuel.domain.enterprise.meter

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.ReportingZones
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetRef
import java.time.Instant

private fun invalid(): MeterDomainException = MeterDomainException()
private fun scope(): MeterDomainException = MeterDomainException(MeterFailure.SCOPE)
private fun identifier(raw: String) { try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw invalid() } }
private fun organization(raw: String) { try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw scope() } }

/** All refs validate supplied ownership only; no UUID proves repository provenance or authority. */
data class MeterRef(val organizationId: String, val asset: AssetRef, val id: String, val kind: MeterKind, val modulus: Long? = null) {
    init {
        organization(organizationId); identifier(id)
        if (asset.organizationId != organizationId) throw scope()
        if (modulus != null && modulus <= 0) throw invalid()
    }
    override fun toString(): String = "EnterpriseMeterRef(redacted)"
}
data class SegmentRef(val organizationId: String, val meter: MeterRef, val id: String) {
    init { organization(organizationId); identifier(id); if (meter.organizationId != organizationId) throw scope() }
    override fun toString(): String = "EnterpriseSegmentRef(redacted)"
}
data class ReadingRef(val organizationId: String, val segment: SegmentRef, val id: String) {
    init { organization(organizationId); identifier(id); if (segment.organizationId != organizationId) throw scope() }
    override fun toString(): String = "EnterpriseReadingRef(redacted)"
}
sealed class ScopedEvidenceRef(val organizationId: String, val id: String) {
    init { organization(organizationId); identifier(id) }
    final override fun equals(other: Any?): Boolean = other is ScopedEvidenceRef && other.javaClass == javaClass &&
        organizationId == other.organizationId && id == other.id
    final override fun hashCode(): Int = 31 * (31 * javaClass.hashCode() + organizationId.hashCode()) + id.hashCode()
    final override fun toString(): String = "EnterpriseScopedEvidenceRef(redacted)"
}
class MembershipRef(organizationId: String, id: String) : ScopedEvidenceRef(organizationId, id)
class DeviceRef(organizationId: String, id: String) : ScopedEvidenceRef(organizationId, id)
class OperationRef(organizationId: String, id: String) : ScopedEvidenceRef(organizationId, id)
class PhotoRef(organizationId: String, id: String) : ScopedEvidenceRef(organizationId, id)

enum class ReadingState { VALID, SUSPECT, VOIDED;
    companion object { fun parse(raw: String): ReadingState = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}
enum class ReadingOrigin { TRIP_START, TRIP_END, HANDOVER, FUELING, CHECKLIST, MAINTENANCE, STANDALONE, IMPORT, METER_INSTALLATION, METER_REMOVAL;
    companion object { fun parse(raw: String): ReadingOrigin = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}
enum class ReadingMethod { TYPED, TYPED_WITH_PHOTO, IMPORTED, INTEGRATION;
    companion object { fun parse(raw: String): ReadingMethod = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}

private fun validateTime(declared: TimeDeclaration, received: Instant) {
    try {
        // Only validation is borrowed; this evidence does not invent a reporting snapshot.
        EnterpriseTime.capture(declared, received, ReportingZones("UTC"))
    } catch (_: EnterpriseTimeException) { throw invalid() }
}
private fun queryTime(at: Instant) {
    try { if (EnterpriseTime.parseInstant(EnterpriseTime.formatInstant(at)) != at) throw invalid() }
    catch (_: EnterpriseTimeException) { throw invalid() }
}

class Reading(val reference: ReadingRef, val value: MeterValue, val declared: TimeDeclaration, val receivedAt: Instant,
    val author: MembershipRef, val device: DeviceRef, val origin: ReadingOrigin, val method: ReadingMethod,
    val operation: OperationRef, val photo: PhotoRef? = null, val classification: ReadingState) {
    init {
        val org = reference.organizationId
        if (author.organizationId != org || device.organizationId != org || operation.organizationId != org ||
            photo != null && photo.organizationId != org) throw scope()
        val meter = reference.segment.meter
        if (classification == ReadingState.VOIDED || value.kind != meter.kind) throw invalid()
        MeterValue.of(meter.kind, value.units, meter.modulus)
        validateTime(declared, receivedAt)
    }
    override fun toString(): String = "EnterpriseMeterReading(redacted)"
}

sealed class CorrectionFact(val segment: SegmentRef, val id: String, val operation: OperationRef,
    val target: ReadingRef, val author: MembershipRef, val device: DeviceRef, val receivedAt: Instant, val reason: String) {
    init {
        identifier(id)
        if (target.segment != segment || operation.organizationId != segment.organizationId || author.organizationId != segment.organizationId ||
            device.organizationId != segment.organizationId) throw scope()
        meterReason(reason)
        validateTime(TimeDeclaration(receivedAt, "UTC", 0), receivedAt)
    }
    final override fun toString(): String = "EnterpriseMeterCorrectionFact(redacted)"
}
class VoidFact(segment: SegmentRef, id: String, operation: OperationRef, target: ReadingRef, author: MembershipRef,
    device: DeviceRef, receivedAt: Instant, reason: String) : CorrectionFact(segment, id, operation, target, author, device, receivedAt, reason)
class SupersessionFact(segment: SegmentRef, id: String, operation: OperationRef, target: ReadingRef, val replacement: ReadingRef,
    author: MembershipRef, device: DeviceRef, receivedAt: Instant, reason: String) : CorrectionFact(segment, id, operation, target, author, device, receivedAt, reason) {
    init { if (replacement.segment != segment) throw scope() }
}

enum class UsageStatus { OK, INSUFFICIENT_DATA }
class MeterUsage internal constructor(val kind: MeterKind, val value: MeterValue?, val from: Reading?, val to: Reading?, val reason: String) {
    val status: UsageStatus get() = if (value == null) UsageStatus.INSUFFICIENT_DATA else UsageStatus.OK
    override fun toString(): String = "EnterpriseMeterUsage(redacted)"
}

/** One observed open segment. New aggregates preserve each original reading and fact. */
class MeterSeries private constructor(val segment: SegmentRef, values: List<Reading>, changes: List<CorrectionFact>) {
    private val values = values.toList()
    private val changes = changes.toList()
    val readings: List<Reading> get() = values.toList()
    val facts: List<CorrectionFact> get() = changes.toList()
    private fun same(reference: ReadingRef) { if (reference.segment != segment) throw scope() }
    private fun freshId(id: String) { if (values.any { it.reference.id == id } || changes.any { it.id == id }) throw invalid() }
    private fun freshOperation(operation: OperationRef) {
        if (values.any { it.operation == operation } || changes.any { it.operation == operation })
            throw MeterDomainException(MeterFailure.OPERATION_ID_REUSED)
    }
    private fun currentTarget(fact: CorrectionFact) {
        if (fact.segment != segment) throw scope()
        same(fact.target)
        if (values.none { it.reference == fact.target }) throw invalid()
        if (changes.any { it.target == fact.target }) throw MeterDomainException(MeterFailure.READING_ALREADY_SUPERSEDED)
        freshId(fact.id)
        freshOperation(fact.operation)
    }
    fun append(reading: Reading): MeterSeries {
        same(reading.reference); freshId(reading.reference.id); freshOperation(reading.operation)
        return MeterSeries(segment, values + reading, changes)
    }
    fun void(fact: VoidFact): MeterSeries {
        currentTarget(fact)
        return MeterSeries(segment, values, changes + fact)
    }
    fun supersede(fact: SupersessionFact, replacement: Reading): MeterSeries {
        currentTarget(fact); same(replacement.reference)
        if (fact.replacement != replacement.reference) throw scope()
        freshId(replacement.reference.id)
        if (fact.id == replacement.reference.id || fact.target == replacement.reference || fact.operation != replacement.operation) throw invalid()
        return MeterSeries(segment, values + replacement, changes + fact)
    }
    fun ordered(): List<Reading> = values.sortedWith(order)
    fun state(reference: ReadingRef): ReadingState {
        same(reference)
        val original = values.firstOrNull { it.reference == reference } ?: throw invalid()
        return if (changes.any { it.target == reference }) ReadingState.VOIDED else original.classification
    }
    fun latest(at: Instant): Reading? {
        queryTime(at)
        val terminal = changes.map { it.target.id }.toSet()
        return values.filter { it.classification == ReadingState.VALID && it.reference.id !in terminal && it.declared.occurredAt <= at }.maxWithOrNull(order)
    }
    fun usage(start: Instant, end: Instant): MeterUsage {
        queryTime(start); queryTime(end)
        if (start > end) throw invalid()
        val from = latest(start)
        val to = latest(end)
        val kind = segment.meter.kind
        if (from == null || to == null) return MeterUsage(kind, null, from, to, "NO_READING_BEFORE")
        if (to.value.units < from.value.units) throw invalid()
        return MeterUsage(kind, MeterValue.of(kind, to.value.units - from.value.units), from, to, "")
    }
    override fun toString(): String = "EnterpriseMeterSeries(redacted)"
    companion object {
        private val order = compareBy<Reading> { it.declared.occurredAt }.thenBy { it.receivedAt }.thenBy { it.reference.id }
        fun open(segment: SegmentRef, opening: Reading): MeterSeries {
            if (opening.reference.segment != segment) throw scope()
            return MeterSeries(segment, listOf(opening), emptyList())
        }
    }
}

object MeterOpenSet {
    fun validate(organizationId: String, meters: List<MeterRef>): List<MeterRef> {
        val values = meters.toList()
        organization(organizationId)
        if (values.any { it.organizationId != organizationId }) throw scope()
        if (values.map { it.id }.toSet().size != values.size) throw invalid()
        if (values.map { it.asset to it.kind }.toSet().size != values.size) throw MeterDomainException(MeterFailure.METER_ALREADY_IN_SERVICE)
        return values.toList()
    }
}
