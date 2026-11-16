package com.anpfuel.domain.enterprise.meter

import java.time.Instant

/** Why a segment opened (fleet §3, B-BR-EMP-481). */
enum class SegmentReason { INSTALLATION, RESET, ROLLOVER }

/** Why a figure is INSUFFICIENT_DATA (fleet §10.5). */
enum class Insufficient { NO_READING_BEFORE, METER_GAP }

/** One segment of a track; [closedAt] null while open; [close] null while open or closed with an unknown final value. */
data class MeterSpan(val id: String, val meter: String, val reason: SegmentReason, val openedAt: Instant, val open: Long, val closedAt: Instant? = null, val close: Long? = null)

/** One VALID reading of a span. */
data class MeterAnchor(val id: String, val segment: String, val value: Long, val occurredAt: Instant, val receivedAt: Instant)

/** A usage figure with its anchors; [reason] null when known; [from] null for a cumulative figure. */
data class MeterFigure(val reason: Insufficient?, val value: Long, val equivalent: Long, val from: String?, val to: String?) {
    val known: Boolean get() = reason == null
}

/** The planned effect of a replacement, reset or rollover (B-BR-EMP-484..486). */
data class MeterBoundary(val at: Instant, val previous: String, val close: Long?, val open: Long, val implied: Long, val reason: Insufficient?)

private fun invalidSegment(): MeterDomainException = MeterDomainException()

/** A validated chain of segments of one kind with its VALID readings (B-BR-EMP-482, EDR-314). */
class MeterTrack private constructor(val kind: MeterKind, private val spans: List<MeterSpan>, private val ordered: List<Pair<Int, MeterAnchor>>) {
    override fun toString(): String = "EnterpriseMeterValue(redacted)"

    private fun anchor(at: Instant): Pair<Int, MeterAnchor>? = ordered.lastOrNull { !it.second.occurredAt.isAfter(at) }

    /** Sum of close − open over spans [from, to); null with an unknown close. */
    private fun closedUsage(from: Int, to: Int): Long? {
        var total = 0L
        for (k in from until to) total = Math.addExact(total, Math.subtractExact(spans[k].close ?: return null, spans[k].open))
        return total
    }

    /** U(t) with its anchor and the equivalent display E(t). */
    fun cumulative(at: Instant): MeterFigure {
        if (!MeterValidation.validInstant(at)) throw invalidSegment()
        val (span, a) = anchor(at) ?: return MeterFigure(Insufficient.NO_READING_BEFORE, 0, 0, null, null)
        val sum = closedUsage(0, span) ?: return MeterFigure(Insufficient.METER_GAP, 0, 0, null, a.id)
        try {
            val value = Math.addExact(sum, a.value - spans[span].open)
            return MeterFigure(null, value, Math.addExact(spans.first().open, value), null, a.id)
        } catch (_: ArithmeticException) { throw invalidSegment() }
    }

    /** usage(t1, t2) with both anchors; nothing is interpolated. */
    fun between(from: Instant, to: Instant): MeterFigure {
        if (!MeterValidation.validInstant(from) || !MeterValidation.validInstant(to) || from.isAfter(to)) throw invalidSegment()
        val a = anchor(from); val b = anchor(to)
        if (a == null || b == null) return MeterFigure(Insufficient.NO_READING_BEFORE, 0, 0, a?.second?.id, b?.second?.id)
        val (i, x) = a; val (j, y) = b
        if (i == j) return MeterFigure(null, y.value - x.value, 0, x.id, y.id)
        val first = spans[i].close ?: return MeterFigure(Insufficient.METER_GAP, 0, 0, x.id, y.id)
        val middle = closedUsage(i + 1, j) ?: return MeterFigure(Insufficient.METER_GAP, 0, 0, x.id, y.id)
        try {
            return MeterFigure(null, Math.addExact(Math.addExact(first - x.value, middle), y.value - spans[j].open), 0, x.id, y.id)
        } catch (_: ArithmeticException) { throw invalidSegment() }
    }

    companion object {
        private val ANCHORS = compareBy<Pair<Int, MeterAnchor>> { it.first }.thenComparator { a, b ->
            MeterValidation.order.compare(MeterPoint(a.second.id, 0, a.second.occurredAt, a.second.receivedAt), MeterPoint(b.second.id, 0, b.second.occurredAt, b.second.receivedAt))
        }

        fun of(kind: MeterKind, spans: List<MeterSpan>, anchors: List<MeterAnchor>): MeterTrack {
            val copy = spans.toList()
            if (copy.isEmpty() || copy.map { it.id }.toSet().size != copy.size) throw invalidSegment()
            val finished = mutableSetOf<String>()
            copy.forEachIndexed { i, s ->
                if (s.id.isEmpty() || s.meter.isEmpty() || !MeterValidation.validInstant(s.openedAt) || !MeterValidation.bounded(kind, null, s.open)) throw invalidSegment()
                val closed = s.closedAt
                if (closed != null) {
                    if (!MeterValidation.validInstant(closed) || !closed.isAfter(s.openedAt) || s.close != null && (s.close < s.open || !MeterValidation.bounded(kind, null, s.close))) throw invalidSegment()
                } else if (s.close != null || i < copy.size - 1) throw invalidSegment()
                if (i == 0 || s.meter != copy[i - 1].meter) {
                    if (s.reason != SegmentReason.INSTALLATION || s.meter in finished || i > 0 && s.openedAt.isBefore(copy[i - 1].closedAt!!)) throw invalidSegment()
                    if (i > 0) finished += copy[i - 1].meter
                } else {
                    val previous = copy[i - 1]
                    if (s.reason == SegmentReason.INSTALLATION || previous.close == null || s.openedAt != previous.closedAt || s.reason == SegmentReason.ROLLOVER && s.open != 0L) throw invalidSegment()
                }
                val lastOfMeter = i == copy.size - 1 || copy[i + 1].meter != s.meter
                if (closed != null && s.close == null && !lastOfMeter) throw invalidSegment()
            }
            val index = copy.withIndex().associate { it.value.id to it.index }
            if (anchors.map { it.id }.toSet().size != anchors.size) throw invalidSegment()
            val ordered = anchors.map { a ->
                val i = index[a.segment] ?: throw invalidSegment()
                val s = copy[i]
                if (a.id.isEmpty() || !MeterValidation.validInstant(a.occurredAt) || !MeterValidation.validInstant(a.receivedAt) || a.value < s.open ||
                    !MeterValidation.bounded(kind, null, a.value) || s.close != null && a.value > s.close || a.occurredAt.isBefore(s.openedAt) ||
                    s.closedAt != null && a.occurredAt.isAfter(s.closedAt)) throw invalidSegment()
                i to a
            }.sortedWith(ANCHORS)
            if (ordered.zipWithNext().any { (a, b) -> a.first == b.first && b.second.value < a.second.value }) throw invalidSegment()
            return MeterTrack(kind, copy, ordered)
        }
    }
}

/** The same planners as the server; the store adds only the database clock and locks (B-BR-EMP-484..486). */
object MeterBoundaries {
    private fun latestValidBefore(members: List<MeterSeriesEntry>, at: Instant): MeterSeriesEntry? =
        members.lastOrNull { it.state == ReadingState.VALID && it.point.occurredAt.isBefore(at) }

    fun replacement(kind: MeterKind, modulus: Long?, series: List<MeterSeriesEntry>, at: Instant, final: Long?, newModulus: Long?, opening: Long): MeterBoundary {
        val members = MeterValidation.members(kind, series, modulus)
        if (!MeterValidation.validInstant(at) || !at.isAfter(members.first().point.occurredAt) || newModulus != null && newModulus <= 0 ||
            !MeterValidation.bounded(kind, newModulus, opening)) throw invalidSegment()
        if (members.any { it.state != ReadingState.VOIDED && !it.point.occurredAt.isBefore(at) }) throw invalidSegment()
        val previous = latestValidBefore(members, at) ?: throw invalidSegment()
        if (final == null) return MeterBoundary(at, previous.point.id, null, opening, 0, Insufficient.METER_GAP)
        if (!MeterValidation.bounded(kind, modulus, final) || final < previous.point.value) throw invalidSegment()
        return MeterBoundary(at, previous.point.id, final, opening, final - previous.point.value, null)
    }

    fun reset(kind: MeterKind, modulus: Long?, series: List<MeterSeriesEntry>, at: Instant, before: Long, after: Long): MeterBoundary {
        val members = MeterValidation.members(kind, series, modulus)
        if (!MeterValidation.validInstant(at) || !at.isAfter(members.first().point.occurredAt) || !MeterValidation.bounded(kind, modulus, before) ||
            !MeterValidation.bounded(kind, modulus, after)) throw invalidSegment()
        if (members.any { it.state == ReadingState.VALID && !it.point.occurredAt.isBefore(at) || it.state != ReadingState.VOIDED && it.point.occurredAt == at }) throw invalidSegment()
        val previous = latestValidBefore(members, at) ?: throw invalidSegment()
        if (before < previous.point.value) throw invalidSegment()
        return MeterBoundary(at, previous.point.id, before, after, before - previous.point.value, null)
    }

    fun rollover(kind: MeterKind, modulus: Long?, series: List<MeterSeriesEntry>, target: String): MeterBoundary {
        if (modulus == null) throw invalidSegment()
        val members = MeterValidation.members(kind, series, modulus)
        val at = members.indexOfFirst { it.point.id == target }
        if (at <= 0 || members[at].state != ReadingState.SUSPECT) throw invalidSegment()
        val t = members[at]
        val previous = members.subList(0, at).lastOrNull { it.state == ReadingState.VALID }
        if (ReadingFlag.POSSIBLE_ROLLOVER !in t.flags || previous == null || !previous.point.occurredAt.isBefore(t.point.occurredAt) || t.point.value >= previous.point.value) throw invalidSegment()
        if (members.withIndex().any { (i, e) -> i > at && e.state == ReadingState.VALID || i != at && e.state != ReadingState.VOIDED && e.point.occurredAt == t.point.occurredAt }) throw invalidSegment()
        return MeterBoundary(t.point.occurredAt, previous.point.id, modulus, 0, (modulus - previous.point.value) + t.point.value, null)
    }
}
