package com.anpfuel.domain.enterprise.trip

import java.time.Instant

/** The single stops-v1 failure (EMP-TRIP-010), shared with the Go package. */
class TripStopsException : IllegalArgumentException("trip stops: invalid")

/** One end of a planned leg: the trip start, a planned stop (1-based sequence) or the plan's destination. */
enum class TripPlannedPointKind { START, STOP, DESTINATION }

data class TripPlannedPoint(val kind: TripPlannedPointKind, val stop: Int = 0)

/** One derived planned leg; it carries no distance or duration (BR-ROUTE-008). */
data class TripPlannedLeg(val sequence: Int, val from: TripPlannedPoint, val to: TripPlannedPoint)

/** A planning revision's validated, ordered intermediate stops (B-BR-EMP-475, EDR-307). */
class TripPlannedStops private constructor(private val stored: List<String>) {
    val places: List<String> get() = stored.toList()
    val size: Int get() = stored.size

    /** start → stop 1 → … → stop n → destination. */
    val legs: List<TripPlannedLeg>
        get() {
            val points = listOf(TripPlannedPoint(TripPlannedPointKind.START)) +
                stored.indices.map { TripPlannedPoint(TripPlannedPointKind.STOP, it + 1) } +
                TripPlannedPoint(TripPlannedPointKind.DESTINATION)
            return points.zipWithNext().mapIndexed { i, (from, to) -> TripPlannedLeg(i + 1, from, to) }
        }

    override fun toString(): String = "EnterpriseTripPlannedStops(redacted)"

    companion object {
        /** The technical bound of a plan's intermediate stops (EDR-307). */
        const val MAX_STOPS: Int = 20

        /** null is no stop; otherwise 1..20 places with the plan text profile; an empty list is invalid. */
        fun create(places: List<String>?): TripPlannedStops {
            if (places == null) return TripPlannedStops(emptyList())
            if (places.isEmpty() || places.size > MAX_STOPS) throw TripStopsException()
            for (place in places) {
                try { TripMissionPlanText.validate(place) } catch (_: IllegalArgumentException) { throw TripStopsException() }
            }
            return TripPlannedStops(places.toList())
        }
    }
}

/** The kind of one fact bounding an observed stop or leg. */
enum class TripStopMark {
    START, STOP, ARRIVAL, DEPARTURE, FINISH, CANCEL;

    companion object {
        /** Exact tokens only; anything else (a NOTE, a lowercase token) is invalid. */
        fun parse(raw: String): TripStopMark = entries.firstOrNull { it.name == raw } ?: throw TripStopsException()
    }
}

/** An incoherent boundary (B-BR-EMP-477). */
enum class TripStopIssue { ARRIVAL_MISSING, DEPARTURE_MISSING }

data class TripStopFact(val mark: TripStopMark, val id: String, val at: Instant)

data class TripObservedStop(val sequence: Int, val arrival: TripStopFact?, val departure: TripStopFact?, val issue: TripStopIssue?)

data class TripObservedLeg(val sequence: Int, val departure: TripStopFact?, val arrival: TripStopFact?, val issue: TripStopIssue?)

/** The derived observed side; [afterEnd] lists the facts declared after the recorded end, never a stop and never dropped. */
data class TripStopObservation(val stops: List<TripObservedStop>, val legs: List<TripObservedLeg>, val afterEnd: List<TripStopFact>)

/** The observed stop and leg derivation shared with Go through stops-v1 (B-BR-EMP-476/477, EDR-308). */
object TripStops {
    private val events = setOf(TripStopMark.STOP, TripStopMark.ARRIVAL, TripStopMark.DEPARTURE)

    /** The shared timeline order between events (B-BR-EMP-324): declared instant, then identity. */
    private val order: Comparator<TripStopFact> = compareBy<TripStopFact>({ it.at }, { it.id })

    /**
     * A STOP or ARRIVAL opens a stop and ends the moving leg; a DEPARTURE closes the open stop and opens the next
     * leg; the end closes the moving leg or leaves the trip at its open stop. Incoherent sequences are surfaced
     * (ARRIVAL_MISSING, DEPARTURE_MISSING), never paired, reordered or dropped; a fact declared after the recorded end
     * is listed in afterEnd. A start that is not a START, an end that is neither FINISH nor CANCEL or precedes the
     * start, another kind of fact, a fact before the start, a blank identity or a repeated fact identity is invalid.
     */
    fun observe(start: TripStopFact, end: TripStopFact?, facts: List<TripStopFact>): TripStopObservation {
        if (start.mark != TripStopMark.START || start.id.isEmpty()) throw TripStopsException()
        if (end != null && (end.mark != TripStopMark.FINISH && end.mark != TripStopMark.CANCEL || end.id.isEmpty() || end.at < start.at)) {
            throw TripStopsException()
        }
        val seen = mutableSetOf<String>()
        for (f in facts) {
            if (f.mark !in events || f.id.isEmpty() || !seen.add(f.id) || f.at < start.at) throw TripStopsException()
        }
        val (inside, afterEnd) = facts.partition { end == null || it.at <= end.at }
        val stops = mutableListOf<TripObservedStop>()
        val legs = mutableListOf<TripObservedLeg>()
        var from: TripStopFact? = start
        var open = -1
        for (f in inside.sortedWith(order)) {
            if (f.mark == TripStopMark.DEPARTURE) {
                if (open >= 0) {
                    stops[open] = stops[open].copy(departure = f)
                } else {
                    stops += TripObservedStop(stops.size + 1, null, f, TripStopIssue.ARRIVAL_MISSING)
                    legs += TripObservedLeg(legs.size + 1, from, null, TripStopIssue.ARRIVAL_MISSING)
                }
                open = -1
                from = f
            } else {
                if (open < 0) {
                    legs += TripObservedLeg(legs.size + 1, from, f, null)
                } else {
                    stops[open] = stops[open].copy(issue = TripStopIssue.DEPARTURE_MISSING)
                    legs += TripObservedLeg(legs.size + 1, null, f, TripStopIssue.DEPARTURE_MISSING)
                }
                stops += TripObservedStop(stops.size + 1, f, null, null)
                open = stops.size - 1
                from = null
            }
        }
        if (open < 0) legs += TripObservedLeg(legs.size + 1, from, end, null)
        return TripStopObservation(stops.toList(), legs.toList(), afterEnd.sortedWith(order))
    }
}
