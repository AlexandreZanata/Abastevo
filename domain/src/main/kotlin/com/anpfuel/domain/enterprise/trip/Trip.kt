package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.ReportingAttribution
import com.anpfuel.domain.enterprise.ReportingZones
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetRef
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.meter.MeterPresence
import com.anpfuel.domain.enterprise.meter.MeterValue
import com.anpfuel.domain.enterprise.meter.ReadingRef

private fun foreignScope(): TripDomainException = TripDomainException(TripFailure.SCOPE)

/** A malformed organization is a scope failure; a malformed identity is invalid. */
private fun requireScope(raw: String): String =
    try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw foreignScope() }
private fun requireIdentity(raw: String): String =
    try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw invalid() }

/** Supplied scoped identities never prove existence, acceptance or authority. */
sealed class ScopedTripRef(val organizationId: String, val id: String) {
    init {
        requireScope(organizationId)
        requireIdentity(id)
    }
    final override fun equals(other: Any?): Boolean =
        other is ScopedTripRef && javaClass == other.javaClass && organizationId == other.organizationId && id == other.id
    final override fun hashCode(): Int = 31 * (31 * javaClass.hashCode() + organizationId.hashCode()) + id.hashCode()
    final override fun toString(): String = "EnterpriseTripRef(redacted)"
}
class TripRef(organizationId: String, id: String) : ScopedTripRef(organizationId, id)
class EmployeeRef(organizationId: String, id: String) : ScopedTripRef(organizationId, id)
class MembershipRef(organizationId: String, id: String) : ScopedTripRef(organizationId, id)

/** Departure or final evidence: a reading with its exact value, or a stated absence. */
sealed class TripMeterEvidence {
    class Observed(val reference: ReadingRef, val value: MeterValue) : TripMeterEvidence() {
        init {
            val meter = reference.segment.meter
            if (value.kind != meter.kind) throw invalid()
            // Re-proves the kind bound and the reference's modulus for this exact value.
            try { MeterValue.of(meter.kind, value.units, meter.modulus) } catch (_: IllegalArgumentException) { throw invalid() }
        }
    }
    class Absent(val missing: MeterPresence.Missing) : TripMeterEvidence()
    final override fun toString(): String = "EnterpriseTripMeterEvidence(redacted)"
}

/** The driver's start. Zones come from the resolved operating context, never from a device claim. */
data class TripStartInput(
    val reference: TripRef? = null,
    val author: MembershipRef? = null,
    val asset: AssetRef? = null,
    val driver: EmployeeRef? = null,
    val departure: TripMeterEvidence? = null,
    val declared: TimeDeclaration? = null,
    val zones: ReportingZones? = null,
    /** Optional device monotonic witness (EMP-TRIP-017). */
    val clock: TripRunClock? = null,
) {
    override fun toString(): String = "EnterpriseTripStartInput(redacted)"
}

/** An operational completion; it cannot change the trip. */
data class TripFinishInput(
    val author: MembershipRef? = null,
    val final: TripMeterEvidence? = null,
    val declared: TimeDeclaration? = null,
    /** Optional device monotonic witness (EMP-TRIP-017). */
    val clock: TripRunClock? = null,
) {
    override fun toString(): String = "EnterpriseTripFinishInput(redacted)"
}

/** One recorded lifecycle event: actor, declaration and evidence. */
class TripEvent internal constructor(val author: MembershipRef, val declared: TimeDeclaration, val evidence: TripMeterEvidence,
    val clock: TripRunClock?) {
    override fun toString(): String = "EnterpriseTripEvent(redacted)"
}

/** A pause, resume or cancellation by a member at the expected revision; CANCEL needs a reason (EMP-TRIP-011). */
data class TripLifecycleInput(
    val author: MembershipRef? = null,
    val expected: Long = 0,
    val reason: String? = null,
    val declared: TimeDeclaration? = null,
) {
    override fun toString(): String = "EnterpriseTripLifecycleInput(redacted)"
}

/** One accepted lifecycle command of a trip. */
class TripLifecycleFact internal constructor(val command: TripCommand, val revision: Long, val from: TripState, val to: TripState,
    val author: MembershipRef, val declared: TimeDeclaration, val reason: String?) {
    override fun toString(): String = "EnterpriseTripLifecycleFact(redacted)"
}

enum class TripUsageStatus { MEASURED, NOT_MEASURED }
enum class TripUsageReason { NOT_FINISHED, READING_ABSENT, DIFFERENT_SEGMENT, DECREASING, CANCELLED }

/** The trip's own meter evidence; it is never invented. */
class TripMeterUsage internal constructor(val status: TripUsageStatus, val reason: TripUsageReason?, val kind: MeterKind?, val delta: Long?) {
    override fun toString(): String = "EnterpriseTripMeterUsage(redacted)"
}

/** Immutable operational trip; only [start] and [finish] build one. */
class Trip private constructor(
    val reference: TripRef,
    val asset: AssetRef,
    val driver: EmployeeRef,
    val state: TripState,
    val started: TripEvent,
    val reporting: ReportingAttribution,
    val finished: TripEvent?,
    /** The informational local day of the finish in the stored reporting zone (EMP-TRIP-017). */
    internal val endDay: String?,
    /** The operational revision: 1 after START, one more per lifecycle command or FINISH (EMP-TRIP-011). */
    val revision: Long,
    private val facts: List<TripLifecycleFact>,
) {
    val organizationId: String get() = reference.organizationId

    /** BR-TRIP-004: the exact difference of the trip's own two readings on one segment, or why there is none. */
    val meterUsage: TripMeterUsage get() {
        if (state == TripState.CANCELLED) return TripMeterUsage(TripUsageStatus.NOT_MEASURED, TripUsageReason.CANCELLED, null, null)
        val end = finished ?: return TripMeterUsage(TripUsageStatus.NOT_MEASURED, TripUsageReason.NOT_FINISHED, null, null)
        val from = started.evidence
        val to = end.evidence
        if (from !is TripMeterEvidence.Observed || to !is TripMeterEvidence.Observed) {
            return TripMeterUsage(TripUsageStatus.NOT_MEASURED, TripUsageReason.READING_ABSENT, null, null)
        }
        if (from.reference.segment != to.reference.segment) {
            return TripMeterUsage(TripUsageStatus.NOT_MEASURED, TripUsageReason.DIFFERENT_SEGMENT, null, null)
        }
        if (to.value.units < from.value.units) {
            return TripMeterUsage(TripUsageStatus.NOT_MEASURED, TripUsageReason.DECREASING, null, null)
        }
        return TripMeterUsage(TripUsageStatus.MEASURED, null, from.value.kind, to.value.units - from.value.units)
    }

    /**
     * Completes the trip operationally and returns the next value (B-BR-EMP-271/272): scope, actor, present
     * final evidence and declaration, absent parts, the starting member, the transition, then time order.
     */
    fun finish(scope: String, input: TripFinishInput): Trip {
        requireScope(scope)
        if (organizationId != scope) throw foreignScope()
        val author = input.author ?: throw invalid()
        if (author.organizationId != scope) throw foreignScope()
        val missing = mutableListOf<TripMissing>()
        val final = input.final
        if (final == null) missing += TripMissing.FINAL_EVIDENCE else {
            final.check(scope, asset)
            val departure = started.evidence
            if (final is TripMeterEvidence.Observed && departure is TripMeterEvidence.Observed && final.reference.id == departure.reference.id) {
                throw invalid()
            }
        }
        val declared = input.declared?.let(::declaration)
        if (declared == null) missing += TripMissing.DECLARATION
        if (missing.isNotEmpty()) throw TripDomainException(TripFailure.INCOMPLETE, missing)
        if (author != responsible) throw TripDomainException(TripFailure.FORBIDDEN)
        val next = TripLifecycle.transition(state, TripCommand.FINISH)
        if (declared!!.occurredAt < started.declared.occurredAt) throw TripDomainException(TripFailure.FINISH_BEFORE_START)
        // The trip keeps its start attribution; the end day is informational, in the stored zone (B-BR-EMP-276).
        val endDay = try { EnterpriseTime.attributeReportingDay(declared.occurredAt, ReportingZones(reporting.zone)).day }
        catch (_: EnterpriseTimeException) { throw invalid() }
        return Trip(reference, asset, driver, next, started, reporting, TripEvent(author, declared, final!!, input.clock), endDay, revision + 1, facts)
    }

    /** The accepted PAUSE, RESUME and CANCEL facts in revision order. */
    val lifecycle: List<TripLifecycleFact> get() = facts.toList()

    /** The member who may change the trip: the member who started it (EMP-TRIP-012 hands it over). */
    val responsible: MembershipRef get() = started.author

    /** The CANCEL fact of a cancelled trip. */
    val cancellation: TripLifecycleFact? get() = facts.lastOrNull()?.takeIf { state == TripState.CANCELLED && it.command == TripCommand.CANCEL }

    /** Moves an IN_PROGRESS trip to PAUSED (B-BR-EMP-390). */
    fun pause(scope: String, input: TripLifecycleInput): Trip = change(scope, TripCommand.PAUSE, input)

    /** Moves a PAUSED trip back to IN_PROGRESS. */
    fun resume(scope: String, input: TripLifecycleInput): Trip = change(scope, TripCommand.RESUME, input)

    /** Ends an IN_PROGRESS or PAUSED trip as CANCELLED, with a reason. */
    fun cancel(scope: String, input: TripLifecycleInput): Trip = change(scope, TripCommand.CANCEL, input)

    /**
     * One lifecycle command, first failure first: scope and trip, author, present reason and declaration, expected
     * revision, absent parts, the responsible member, a terminal trip, a stale revision, the table, then an instant
     * before the start. The receiver never changes.
     */
    private fun change(scope: String, command: TripCommand, input: TripLifecycleInput): Trip {
        requireScope(scope)
        if (organizationId != scope) throw foreignScope()
        val author = input.author ?: throw invalid()
        if (author.organizationId != scope) throw foreignScope()
        input.reason?.let { reason -> try { MeterPresence.missing(reason) } catch (_: IllegalArgumentException) { throw invalid() } }
        val declared = input.declared?.let(::declaration)
        if (input.expected < 1) throw invalid()
        val missing = mutableListOf<TripMissing>()
        if (command == TripCommand.CANCEL && input.reason == null) missing += TripMissing.REASON
        if (declared == null) missing += TripMissing.DECLARATION
        if (missing.isNotEmpty()) throw TripDomainException(TripFailure.INCOMPLETE, missing)
        if (author != responsible) throw TripDomainException(TripFailure.FORBIDDEN)
        if (state.terminal) throw TripDomainException(TripFailure.INVALID_TRANSITION)
        if (input.expected != revision) throw TripDomainException(TripFailure.CONFLICT)
        val next = TripLifecycle.transition(state, command)
        if (declared!!.occurredAt < started.declared.occurredAt) throw TripDomainException(TripFailure.BEFORE_START)
        val fact = TripLifecycleFact(command, revision + 1, state, next, author, declared, input.reason)
        return Trip(reference, asset, driver, next, started, reporting, finished, endDay, revision + 1, facts + fact)
    }

    override fun toString(): String = "EnterpriseTrip(redacted)"

    companion object {
        /**
         * Creates a C1 driver-initiated trip in IN_PROGRESS (B-BR-EMP-269/270): scope, identity and author, the
         * present parts of the eligible context in order, every absent part, then the start reporting day. It
         * reads no clock and needs no server receipt.
         */
        fun start(scope: String, input: TripStartInput): Trip {
            requireScope(scope)
            val reference = input.reference ?: throw invalid()
            if (reference.organizationId != scope) throw foreignScope()
            val author = input.author ?: throw invalid()
            if (author.organizationId != scope) throw foreignScope()
            val missing = mutableListOf<TripMissing>()
            val asset = input.asset
            if (asset == null) missing += TripMissing.ASSET else if (asset.organizationId != scope) throw foreignScope()
            val driver = input.driver
            if (driver == null) missing += TripMissing.DRIVER else if (driver.organizationId != scope) throw foreignScope()
            val departure = input.departure
            if (departure == null) missing += TripMissing.DEPARTURE_EVIDENCE else departure.check(scope, asset)
            val declared = input.declared?.let(::declaration)
            if (declared == null) missing += TripMissing.DECLARATION
            if (missing.isNotEmpty()) throw TripDomainException(TripFailure.INCOMPLETE, missing)
            val zones = input.zones ?: throw invalid()
            val reporting = try { EnterpriseTime.attributeReportingDay(declared!!.occurredAt, zones) }
            catch (_: EnterpriseTimeException) { throw invalid() }
            return Trip(reference, asset!!, driver!!, TripState.IN_PROGRESS, TripEvent(author, declared, departure!!, input.clock), reporting, null, null,
                1, emptyList())
        }

        private fun declaration(declared: TimeDeclaration): TimeDeclaration =
            try { EnterpriseTime.validateDeclaration(declared) } catch (_: EnterpriseTimeException) { throw invalid() }

        /** Binds a supplied reading to the organization and, when known, to the trip's asset. */
        private fun TripMeterEvidence.check(scope: String, asset: AssetRef?) {
            if (this !is TripMeterEvidence.Observed) return
            val owner = reference.segment.meter.asset
            if (reference.organizationId != scope || asset != null && owner != asset) throw foreignScope()
        }
    }
}
