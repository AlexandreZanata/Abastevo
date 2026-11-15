package com.anpfuel.domain.enterprise.meter

import java.math.BigInteger
import java.time.Instant

/** Reasons for SUSPECT (fleet §10.5), in canonical order. Never a refusal. */
enum class ReadingFlag { READING_BELOW_PREVIOUS, READING_ABOVE_NEXT, IMPLAUSIBLE_RATE, FUTURE_INSTANT }

private fun invalid(): MeterDomainException = MeterDomainException()

/** One validated plausibility configuration (B-BR-EMP-282/283). */
class MeterPlausibility private constructor(val distancePerHour: Long, val clockSkewSeconds: Long) {
    override fun equals(other: Any?): Boolean = other is MeterPlausibility && distancePerHour == other.distancePerHour && clockSkewSeconds == other.clockSkewSeconds
    override fun hashCode(): Int = 31 * distancePerHour.hashCode() + clockSkewSeconds.hashCode()
    override fun toString(): String = "EnterpriseMeterValue(redacted)"
    companion object {
        const val DEFAULT_DISTANCE_PER_HOUR = 150_000L
        const val MIN_DISTANCE_PER_HOUR = 20_000L
        const val MAX_DISTANCE_PER_HOUR = 200_000L
        const val DEFAULT_CLOCK_SKEW_SECONDS = 300L
        fun of(distancePerHour: Long, clockSkewSeconds: Long): MeterPlausibility {
            if (distancePerHour !in MIN_DISTANCE_PER_HOUR..MAX_DISTANCE_PER_HOUR || clockSkewSeconds < 0) throw invalid()
            return MeterPlausibility(distancePerHour, clockSkewSeconds)
        }
        val DEFAULT: MeterPlausibility = MeterPlausibility(DEFAULT_DISTANCE_PER_HOUR, DEFAULT_CLOCK_SKEW_SECONDS)
    }
}

/** One observation by its ordering key and value. */
data class MeterPoint(val id: String, val value: Long, val occurredAt: Instant, val receivedAt: Instant)

/** A series member with its current classification. */
data class MeterSeriesEntry(val point: MeterPoint, val state: ReadingState, val flags: List<ReadingFlag> = emptyList())

/** Classification against the valid neighbours; null neighbours are absent. */
data class MeterVerdict(val state: ReadingState, val flags: List<ReadingFlag>, val retroactive: Boolean, val previous: String?, val next: String?)

data class MeterVerdictChange(val id: String, val verdict: MeterVerdict)

/** The same validator as the server (B-BR-EMP-280..285); the app warns with it and never blocks saving. */
object MeterValidation {
    const val NOTE_RETROACTIVE = "RETROACTIVE"
    private val NANOS: BigInteger = BigInteger.valueOf(1_000_000_000L)
    private val HOUR: BigInteger = BigInteger.valueOf(3_600L)
    private val DISPLAY: BigInteger = BigInteger.valueOf(3_600_000L).multiply(NANOS)
    private val HOUR_TOLERANCE: BigInteger = BigInteger.valueOf(300L).multiply(NANOS)
    private val MIN_INSTANT: Instant = Instant.parse("0001-01-01T00:00:00Z")
    private val MAX_INSTANT: Instant = Instant.parse("9999-12-31T23:59:59.999999999Z")
    private val ORDER = compareBy<MeterPoint> { it.occurredAt }.thenBy { it.receivedAt }.thenBy { it.id }

    private fun bound(kind: MeterKind): Long = if (kind == MeterKind.ODOMETER) 1_000_000_000_000L else 3_600_000_000L
    private fun validPoint(kind: MeterKind, p: MeterPoint): Boolean = p.id.isNotEmpty() && p.value in 0..bound(kind) &&
        p.occurredAt in MIN_INSTANT..MAX_INSTANT && p.receivedAt in MIN_INSTANT..MAX_INSTANT
    private fun canonical(state: ReadingState, flags: List<ReadingFlag>): Boolean = when (state) {
        ReadingState.VALID, ReadingState.VOIDED -> flags.isEmpty()
        ReadingState.SUSPECT -> flags.isNotEmpty() && flags.zipWithNext().all { (a, b) -> a.ordinal < b.ordinal }
    }
    private fun members(kind: MeterKind, series: List<MeterSeriesEntry>): List<MeterSeriesEntry> {
        val copy = series.map { it.copy(flags = it.flags.toList()) }
        if (copy.isEmpty() || copy.map { it.point.id }.toSet().size != copy.size) throw invalid()
        copy.forEachIndexed { i, e ->
            if (!validPoint(kind, e.point) || !canonical(e.state, e.flags) || i > 0 && ORDER.compare(copy[i - 1].point, e.point) >= 0) throw invalid()
        }
        return copy
    }
    private fun elapsed(a: Instant, b: Instant): BigInteger = BigInteger.valueOf(b.epochSecond - a.epochSecond).multiply(NANOS)
        .add(BigInteger.valueOf((b.nano - a.nano).toLong()))
    private fun plausible(policy: MeterPlausibility, kind: MeterKind, a: MeterPoint, b: MeterPoint): Boolean {
        val delta = BigInteger.valueOf(b.value - a.value)
        val dt = elapsed(a.occurredAt, b.occurredAt)
        return if (kind == MeterKind.ODOMETER) delta.multiply(HOUR).multiply(NANOS) <= BigInteger.valueOf(policy.distancePerHour).multiply(dt).add(DISPLAY)
        else delta.multiply(NANOS) <= dt.add(HOUR_TOLERANCE)
    }
    private fun future(policy: MeterPlausibility, p: MeterPoint): Boolean =
        elapsed(p.receivedAt, p.occurredAt) > BigInteger.valueOf(policy.clockSkewSeconds).multiply(NANOS)
    private fun verdict(policy: MeterPlausibility, kind: MeterKind, others: List<MeterSeriesEntry>, c: MeterPoint, keepFuture: Boolean?): MeterVerdict {
        val valid = others.filter { it.state == ReadingState.VALID }.map { it.point }
        val previous = valid.lastOrNull { ORDER.compare(it, c) < 0 }
        val next = valid.firstOrNull { ORDER.compare(it, c) > 0 }
        val raised = mutableSetOf<ReadingFlag>()
        if (previous != null) {
            if (c.value < previous.value) raised += ReadingFlag.READING_BELOW_PREVIOUS
            else if (!plausible(policy, kind, previous, c)) raised += ReadingFlag.IMPLAUSIBLE_RATE
        }
        if (next != null) {
            if (next.value < c.value) raised += ReadingFlag.READING_ABOVE_NEXT
            else if (!plausible(policy, kind, c, next)) raised += ReadingFlag.IMPLAUSIBLE_RATE
        }
        if (keepFuture ?: future(policy, c)) raised += ReadingFlag.FUTURE_INSTANT
        val flags = ReadingFlag.entries.filter { it in raised }
        return MeterVerdict(if (flags.isEmpty()) ReadingState.VALID else ReadingState.SUSPECT, flags, next != null, previous?.id, next?.id)
    }

    fun check(policy: MeterPlausibility, kind: MeterKind, series: List<MeterSeriesEntry>, candidate: MeterPoint): MeterVerdict {
        val all = members(kind, series)
        if (!validPoint(kind, candidate) || ORDER.compare(all.first().point, candidate) >= 0 || all.any { it.point.id == candidate.id }) throw invalid()
        return verdict(policy, kind, all, candidate, null)
    }

    fun reevaluate(policy: MeterPlausibility, kind: MeterKind, series: List<MeterSeriesEntry>): List<MeterVerdictChange> {
        val work = members(kind, series).toMutableList()
        val final = mutableMapOf<String, MeterVerdict>()
        var promoted = true
        while (promoted) {
            promoted = false
            for (i in work.indices) {
                val e = work[i]
                if (e.state != ReadingState.SUSPECT) continue
                val v = verdict(policy, kind, work.filterIndexed { j, _ -> j != i }, e.point, ReadingFlag.FUTURE_INSTANT in e.flags)
                if (v.state == e.state && v.flags == e.flags) continue
                work[i] = e.copy(state = v.state, flags = v.flags)
                final[e.point.id] = v
                if (v.state == ReadingState.VALID) promoted = true
            }
        }
        return series.mapNotNull { original ->
            final[original.point.id]?.takeIf { it.state != original.state || it.flags != original.flags }?.let { MeterVerdictChange(original.point.id, it) }
        }
    }
}
