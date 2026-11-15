package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.meter.MeterDomainException
import com.anpfuel.domain.enterprise.meter.MeterPresence
import java.time.Instant

/** Closed logbook failure vocabulary shared with the Go package through logbook-v1 (EMP-TRIP-003). */
enum class TripLogbookFailure { INVALID, SCOPE, FORBIDDEN, UNSUPPORTED_EVENT, EVENT_BEFORE_START, EVENT_AFTER_FINISH }

class TripLogbookException(val failure: TripLogbookFailure = TripLogbookFailure.INVALID) :
    IllegalArgumentException("trip logbook: ${failure.name.lowercase()}")

private fun logbook(failure: TripLogbookFailure = TripLogbookFailure.INVALID) = TripLogbookException(failure)

/**
 * The closed trip-logbook-v1 vocabulary (B-BR-EMP-320): four supported instantaneous facts and the types
 * reserved for their owners (fuelings and expenses, incidents, handover, pause/resume/cancel), never recorded here.
 */
enum class TripLogbookType(val supported: Boolean) {
    NOTE(true), STOP(true), ARRIVAL(true), DEPARTURE(true),
    FUELING(false), TOLL(false), PARKING(false), INCIDENT(false), DAMAGE(false), HANDOVER(false),
    PAUSE(false), RESUME(false), CANCEL(false);

    /** Only NOTE requires a text. */
    val requiresText: Boolean get() = this == NOTE

    companion object {
        /** Exact tokens only; anything else is INVALID. */
        fun parse(raw: String): TripLogbookType = entries.firstOrNull { it.name == raw } ?: throw logbook()
    }
}

/** The device identity of one logbook event; its shape proves no existence. */
class TripLogbookRef(organizationId: String, id: String) : ScopedTripRef(organizationId, id)

/** One logbook event as the author's device recorded it; the type is the raw token. */
data class TripLogbookInput(
    val reference: TripLogbookRef? = null,
    val author: MembershipRef? = null,
    val type: String? = null,
    val text: String? = null,
    val declared: TimeDeclaration? = null,
) {
    override fun toString(): String = "EnterpriseTripLogbookInput(redacted)"
}

/** One logbook fact of a trip; it never changes the trip. */
class TripLogbookEntry internal constructor(
    val trip: TripRef,
    val reference: TripLogbookRef,
    val author: MembershipRef,
    val type: TripLogbookType,
    val text: String?,
    val declared: TimeDeclaration,
) {
    override fun toString(): String = "EnterpriseTripLogbookEntry(redacted)"
}

/**
 * Validates one logbook event against this trip (B-BR-EMP-320/323), first failure first: scope, event identity
 * and author, the type (unknown INVALID, reserved UNSUPPORTED_EVENT), the text, the declaration, the starting
 * member, then the time window (before the start; after the finish of a completed trip). The trip is unchanged.
 */
fun Trip.log(scope: String, input: TripLogbookInput): TripLogbookEntry {
    try { EnterprisePartition.requireId(scope) } catch (_: IllegalArgumentException) { throw logbook(TripLogbookFailure.SCOPE) }
    if (organizationId != scope) throw logbook(TripLogbookFailure.SCOPE)
    val event = input.reference ?: throw logbook()
    if (event.organizationId != scope) throw logbook(TripLogbookFailure.SCOPE)
    val author = input.author ?: throw logbook()
    if (author.organizationId != scope) throw logbook(TripLogbookFailure.SCOPE)
    val type = TripLogbookType.parse(input.type ?: throw logbook())
    if (!type.supported) throw logbook(TripLogbookFailure.UNSUPPORTED_EVENT)
    val text = input.text
    if (text == null && type.requiresText) throw logbook()
    if (text != null) {
        try { MeterPresence.missing(text) } catch (_: MeterDomainException) { throw logbook() }
    }
    val declared = input.declared ?: throw logbook()
    try { EnterpriseTime.validateDeclaration(declared) } catch (_: EnterpriseTimeException) { throw logbook() }
    if (author != started.author) throw logbook(TripLogbookFailure.FORBIDDEN)
    if (declared.occurredAt < started.declared.occurredAt) throw logbook(TripLogbookFailure.EVENT_BEFORE_START)
    val end = finished
    if (end != null && declared.occurredAt > end.declared.occurredAt) throw logbook(TripLogbookFailure.EVENT_AFTER_FINISH)
    return TripLogbookEntry(reference, event, author, type, text, declared)
}

/** The kind of a timeline item: the trip's start, one logbook event, or its finish. */
enum class TripTimelineKind { START, EVENT, FINISH }

/** One timeline item; receipt is shown beside it but is never an ordering key. */
data class TripTimelineItem(val kind: TripTimelineKind, val id: String, val at: Instant)

/**
 * The timeline order shared with Go (B-BR-EMP-324): original instant; at equal instants the start, then events
 * by identity, then the finish.
 */
object TripTimeline {
    val order: Comparator<TripTimelineItem> = compareBy<TripTimelineItem>({ it.at }, { it.kind.ordinal }, { it.id })

    fun sort(items: Iterable<TripTimelineItem>): List<TripTimelineItem> = items.sortedWith(order)
}
