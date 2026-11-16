package com.anpfuel.domain.enterprise.tracking

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.SyncJson
import java.time.Instant

/*
 * EMP-GPS-003 (B-BR-EMP-501..505, EDR-325..327): the approved sampling policy, the per-fix filter, the fix-level gaps and
 * the sealed local buffer of one partition. Coordinates exist only inside the sealed buffer and the batches; every
 * value here prints redacted.
 */

/** The approved policy `sampling-v1` (gps-navigation §10.2 defaults; B-BR-EMP-501). */
object TrackingSamplingPolicy {
    const val VERSION = "sampling-v1"
    /** The platform request interval (EDR-323); the sampler decides what is recorded. */
    const val REQUEST_MILLIS = 5_000L
    const val MOVING_CM_PER_SECOND = 150
    const val ROUTINE_MILLIS = 10_000L
    const val ECONOMY_MILLIS = 30_000L
    const val SLOW_METRES = 50.0
    const val TURN_DEGREES = 30
    const val TURN_MIN_MILLIS = 5_000L
    const val HEARTBEAT_MILLIS = 120_000L
    const val MAX_ACCURACY_METRES = 50
    const val MAX_AGE_MILLIS = 15_000L
    const val MAX_SPEED_METRES_PER_SECOND = 70.0
    const val ECONOMY_BATTERY_PERCENT = 15
    /** BR-GPS-024: no accepted fix for more than this inside a recording period is a gap. */
    const val GAP_MILLIS = 180_000L
    /** BR-GPS-020: pending fixes are sealed into a batch at least this often. */
    const val SEAL_MILLIS = 60_000L
    /** A bound on unsealed fixes; past it nothing new is recorded and the overflow is counted (EDR-327). */
    const val MAX_PENDING = 2_000
    const val MAX_GAPS = 500
    const val EARTH_RADIUS_METRES = 6_371_008.8
}

/** A platform report in device units: the monotonic age gives its instant on the device's own clock (EDR-325). */
class TrackingObservation(val latitude: Double, val longitude: Double, val accuracyMetres: Float?, val speedMetresPerSecond: Float?,
    val bearingDegrees: Float?, val mock: Boolean, val elapsedMillis: Long) {
    override fun toString(): String = "EnterpriseTrackingObservation(redacted)"
}

enum class TrackingGapReason { SIGNAL_LOST, PROVIDER_DISABLED, MOCK_LOCATION }

/** A fix-level gap inside a recording period: a reason and an interval, never a position (BR-GPS-024). */
class TrackingGap(val reason: TrackingGapReason, val start: Instant, val end: Instant?) {
    init { require(end == null || !end.isBefore(start)) }
    override fun toString(): String = "EnterpriseTrackingGap(redacted)"
}

/** One recorded fix in the buffer, on the device clock, in the batch units. */
class BufferedFix(val at: Instant, val elapsedMillis: Long, val latE7: Int, val lonE7: Int, val accuracyMetres: Int, val speedCmPerSecond: Int?,
    val bearingDegrees: Int?, val coarse: Boolean) {
    override fun toString(): String = "EnterpriseBufferedFix(redacted)"
}

/** What one observation became. */
enum class TrackingObservationOutcome { RECORDED, ACCEPTED_NOT_RECORDED, DROPPED, OVERFLOW, IGNORED }

/** One session's sealed buffer (B-BR-EMP-503). */
class SessionBuffer(
    val sessionId: String,
    val tripId: String,
    val bootId: String,
    val nextSequence: Long,
    pending: List<BufferedFix>,
    val dropped: TrackingDropped,
    /** The last fix that passed the filter (anchor of the implied-speed check and of gap detection). */
    val lastAcceptedAt: Instant?,
    val lastAcceptedElapsed: Long?,
    val lastAcceptedLatE7: Int?,
    val lastAcceptedLonE7: Int?,
    /** The last recorded fix (anchor of the sampler). */
    val lastRecorded: BufferedFix?,
    gaps: List<TrackingGap>,
    /** A mock report was received since the last accepted fix (the reason of a gap that opens now). */
    val mockSinceAccepted: Boolean = false,
    val overflow: Long = 0,
    batchOperationIds: List<String> = emptyList(),
    val recorded: Long = 0,
) {
    private val fixes = pending.toList()
    private val gapValues = gaps.toList()
    private val batches = batchOperationIds.toList()
    val pending: List<BufferedFix> get() = fixes.toList()
    val gaps: List<TrackingGap> get() = gapValues.toList()
    val batchOperationIds: List<String> get() = batches.toList()
    init {
        EnterprisePartition.requireId(sessionId); EnterprisePartition.requireId(tripId)
        require(Regex("^[A-Za-z0-9._-]{1,64}$").matches(bootId) && nextSequence in 1..TrackingBatchOperations.MAX_SEQUENCE &&
            fixes.size <= TrackingSamplingPolicy.MAX_PENDING && gapValues.size <= TrackingSamplingPolicy.MAX_GAPS && overflow >= 0 && recorded >= 0)
        require(fixes.zipWithNext().all { (a, b) -> b.at.isAfter(a.at) })
        require(gapValues.dropLast(1).all { it.end != null })
        require(batches.all { EnterprisePartition.requireId(it); true } && batches.distinct().size == batches.size)
        require(listOf(lastAcceptedAt == null, lastAcceptedElapsed == null, lastAcceptedLatE7 == null, lastAcceptedLonE7 == null).distinct().size == 1)
    }
    val openGap: TrackingGap? get() = gapValues.lastOrNull()?.takeIf { it.end == null }
    /** The buffer continuing on another boot: only once every fix of the previous boot was sealed (B-BR-EMP-504). */
    fun onBoot(bootId: String): SessionBuffer {
        if (bootId == this.bootId) return this
        require(fixes.isEmpty()) { "Seal the previous boot's fixes first" }
        return copy(bootId = bootId)
    }
    val checkpointAt: Instant? get() = listOfNotNull(lastRecorded?.at, lastAcceptedAt).maxOrNull()

    internal fun copy(bootId: String = this.bootId, nextSequence: Long = this.nextSequence, pending: List<BufferedFix> = fixes,
        dropped: TrackingDropped = this.dropped, lastAcceptedAt: Instant? = this.lastAcceptedAt, lastAcceptedElapsed: Long? = this.lastAcceptedElapsed,
        lastAcceptedLatE7: Int? = this.lastAcceptedLatE7, lastAcceptedLonE7: Int? = this.lastAcceptedLonE7, lastRecorded: BufferedFix? = this.lastRecorded,
        gaps: List<TrackingGap> = gapValues, mockSinceAccepted: Boolean = this.mockSinceAccepted, overflow: Long = this.overflow,
        batchOperationIds: List<String> = batches, recorded: Long = this.recorded) = SessionBuffer(sessionId, tripId, bootId, nextSequence, pending,
        dropped, lastAcceptedAt, lastAcceptedElapsed, lastAcceptedLatE7, lastAcceptedLonE7, lastRecorded, gaps, mockSinceAccepted, overflow,
        batchOperationIds, recorded)
    override fun toString(): String = "EnterpriseSessionBuffer(redacted)"

    companion object {
        fun empty(sessionId: String, tripId: String, bootId: String): SessionBuffer =
            SessionBuffer(sessionId, tripId, bootId, 1, emptyList(), TrackingDropped(), null, null, null, null, null, emptyList())
    }
}

/** The pure sampling, filter and gap rules (BR-GPS-017/018/019/024 on the device). */
object TrackingSampler {
    private fun e7(degrees: Double): Int = Math.round(degrees * 10_000_000.0).toInt()
    private fun metres(aLat: Int, aLon: Int, bLat: Int, bLon: Int): Double {
        val p1 = Math.toRadians(aLat / 1e7); val p2 = Math.toRadians(bLat / 1e7)
        val dp = p2 - p1; val dl = Math.toRadians((bLon - aLon) / 1e7)
        val h = Math.sin(dp / 2).let { it * it } + Math.cos(p1) * Math.cos(p2) * Math.sin(dl / 2).let { it * it }
        return 2 * TrackingSamplingPolicy.EARTH_RADIUS_METRES * Math.asin(Math.min(1.0, Math.sqrt(h)))
    }
    private fun turn(a: Int?, b: Int?): Int? = if (a == null || b == null) null else Math.abs(a - b).let { if (it > 180) 360 - it else it }

    /**
     * Filters, samples and records one observation made [nowElapsed] / [nowWall] on the device, inside a recording period
     * that started at [periodStart] with [precision]. Mock, stale, imprecise and jump fixes are counted, never recorded;
     * an accepted fix closes an open gap; nothing is ever invented.
     */
    fun observe(buffer: SessionBuffer, observation: TrackingObservation, precision: LocationPermission, economy: Boolean, periodStart: Instant,
        nowElapsed: Long, nowWall: Instant): Pair<SessionBuffer, TrackingObservationOutcome> {
        if (precision == LocationPermission.NONE) return buffer to TrackingObservationOutcome.IGNORED
        val age = nowElapsed - observation.elapsedMillis
        val at = nowWall.minusMillis(age.coerceAtLeast(0))
        fun drop(reason: TrackingDropReason) = buffer.copy(dropped = buffer.dropped.plus(reason),
            mockSinceAccepted = buffer.mockSinceAccepted || reason == TrackingDropReason.MOCK) to TrackingObservationOutcome.DROPPED
        if (!observation.latitude.isFinite() || !observation.longitude.isFinite() || observation.latitude !in -90.0..90.0 ||
            observation.longitude !in -180.0..180.0) return buffer to TrackingObservationOutcome.IGNORED
        if (observation.mock) return drop(TrackingDropReason.MOCK)
        // Before the period (a report queued before the start) or older than 15 s at collection: stale.
        if (age !in 0..TrackingSamplingPolicy.MAX_AGE_MILLIS || at.isBefore(periodStart)) return drop(TrackingDropReason.STALE)
        val accuracy = observation.accuracyMetres
        if (accuracy == null || !accuracy.isFinite() || accuracy < 0 ||
            precision == LocationPermission.PRECISE && accuracy > TrackingSamplingPolicy.MAX_ACCURACY_METRES) return drop(TrackingDropReason.ACCURACY)
        val lat = e7(observation.latitude); val lon = e7(observation.longitude)
        val previousAt = buffer.lastAcceptedAt
        if (previousAt != null && !at.isAfter(previousAt)) return buffer to TrackingObservationOutcome.IGNORED
        if (previousAt != null && precision == LocationPermission.PRECISE) {
            val seconds = (at.toEpochMilli() - previousAt.toEpochMilli()) / 1000.0
            val distance = metres(buffer.lastAcceptedLatE7!!, buffer.lastAcceptedLonE7!!, lat, lon)
            if (seconds > 0 && distance / seconds > TrackingSamplingPolicy.MAX_SPEED_METRES_PER_SECOND) return drop(TrackingDropReason.JUMP)
        }
        val speed = observation.speedMetresPerSecond?.takeIf { it.isFinite() && it >= 0 }?.let { Math.round(it * 100.0).toInt().coerceAtMost(TrackingBatchOperations.MAX_SPEED) }
        val bearing = observation.bearingDegrees?.takeIf { it.isFinite() }?.let { (Math.round(it.toDouble()).toInt() % 360 + 360) % 360 }
        val fix = BufferedFix(at, observation.elapsedMillis, lat, lon, Math.ceil(accuracy.toDouble()).toInt().coerceAtMost(TrackingBatchOperations.MAX_ACCURACY),
            speed, bearing, precision == LocationPermission.APPROXIMATE)
        // The accepted fix closes an open gap at its own instant (BR-GPS-024).
        val gaps = buffer.gaps.let { list -> list.lastOrNull()?.takeIf { it.end == null }?.let { open -> list.dropLast(1) + TrackingGap(open.reason, open.start, at) } ?: list }
        val accepted = buffer.copy(lastAcceptedAt = at, lastAcceptedElapsed = observation.elapsedMillis, lastAcceptedLatE7 = lat, lastAcceptedLonE7 = lon,
            gaps = gaps, mockSinceAccepted = false)
        if (!record(buffer.lastRecorded, fix, precision, economy)) return accepted to TrackingObservationOutcome.ACCEPTED_NOT_RECORDED
        if (buffer.pending.size >= TrackingSamplingPolicy.MAX_PENDING) return accepted.copy(overflow = accepted.overflow + 1) to TrackingObservationOutcome.OVERFLOW
        return accepted.copy(pending = buffer.pending + fix, lastRecorded = fix, recorded = buffer.recorded + 1) to TrackingObservationOutcome.RECORDED
    }

    /** sampling-v1: first fix; moving every 10 s (30 s in economy) or on a 30° turn after 5 s; slow on 50 m; heartbeat 120 s; coarse at heartbeat only. */
    fun record(last: BufferedFix?, fix: BufferedFix, precision: LocationPermission, economy: Boolean): Boolean {
        if (last == null) return true
        val since = fix.at.toEpochMilli() - last.at.toEpochMilli()
        if (since >= TrackingSamplingPolicy.HEARTBEAT_MILLIS) return true
        if (precision == LocationPermission.APPROXIMATE) return false
        val moving = (fix.speedCmPerSecond ?: 0) >= TrackingSamplingPolicy.MOVING_CM_PER_SECOND
        if (moving) {
            if (since >= if (economy) TrackingSamplingPolicy.ECONOMY_MILLIS else TrackingSamplingPolicy.ROUTINE_MILLIS) return true
            val turned = turn(last.bearingDegrees, fix.bearingDegrees)
            return !economy && turned != null && turned >= TrackingSamplingPolicy.TURN_DEGREES && since >= TrackingSamplingPolicy.TURN_MIN_MILLIS
        }
        return metres(last.latE7, last.lonE7, fix.latE7, fix.lonE7) >= TrackingSamplingPolicy.SLOW_METRES
    }

    /**
     * Gap detection at a watchdog tick or a provider change: more than 180 s inside the period without an accepted fix opens
     * a gap from the last accepted fix (or the period start) with its reason; a disabled provider opens one at once.
     */
    fun tick(buffer: SessionBuffer, now: Instant, providerEnabled: Boolean, periodStart: Instant): SessionBuffer {
        if (buffer.openGap != null) return buffer
        val since = listOfNotNull(buffer.lastAcceptedAt, periodStart).maxOrNull()!!
        val reason = when {
            !providerEnabled -> TrackingGapReason.PROVIDER_DISABLED
            now.toEpochMilli() - since.toEpochMilli() <= TrackingSamplingPolicy.GAP_MILLIS -> return buffer
            buffer.mockSinceAccepted -> TrackingGapReason.MOCK_LOCATION
            else -> TrackingGapReason.SIGNAL_LOST
        }
        if (buffer.gaps.size >= TrackingSamplingPolicy.MAX_GAPS) return buffer
        return buffer.copy(gaps = buffer.gaps + TrackingGap(reason, since, null))
    }

    /** A recording period ended (stop, interruption, end): an open gap ends with it. */
    fun closePeriod(buffer: SessionBuffer, at: Instant): SessionBuffer {
        val open = buffer.openGap ?: return buffer
        return buffer.copy(gaps = buffer.gaps.dropLast(1) + TrackingGap(open.reason, open.start, maxOf(at, open.start)))
    }
}

/** The versioned sealed `tracking-buffer` record of one partition (EDR-327). */
class TrackingBufferBook private constructor(val membershipId: String, val enrollmentId: String, sessions: List<SessionBuffer>) {
    private val values = sessions.toList()
    val sessions: List<SessionBuffer> get() = values.toList()
    init {
        EnterprisePartition.requireId(membershipId); EnterprisePartition.requireId(enrollmentId)
        require(values.size <= TrackingBook.MAX_SESSIONS && values.map { it.sessionId }.distinct().size == values.size)
    }
    fun session(sessionId: String): SessionBuffer? = values.firstOrNull { it.sessionId == sessionId }
    fun with(buffer: SessionBuffer): TrackingBufferBook {
        val existing = session(buffer.sessionId)
        require(existing == null || existing.tripId == buffer.tripId)
        return TrackingBufferBook(membershipId, enrollmentId, if (existing == null) values + buffer else values.map { if (it.sessionId == buffer.sessionId) buffer else it })
    }
    fun requireBinding(membershipId: String, enrollmentId: String): TrackingBufferBook {
        require(membershipId == this.membershipId && enrollmentId == this.enrollmentId) { "Foreign tracking buffer" }
        return this
    }
    val canonicalBytes: ByteArray get() = CanonicalJson.encode(SyncJson.Object(mapOf(
        "format" to SyncJson.Text(FORMAT), "version" to SyncJson.Integer(VERSION.toString()), "membership_id" to SyncJson.Text(membershipId),
        "enrollment_id" to SyncJson.Text(enrollmentId), "sessions" to SyncJson.Array(values.map(::sessionJson)))))
    override fun toString(): String = "EnterpriseTrackingBufferBook(redacted)"

    companion object {
        const val FORMAT = "tracking-buffer"
        const val VERSION = 1L
        const val MAX_BYTES = 16 * 1024 * 1024
        private val SESSION_KEYS = setOf("session_id", "trip_id", "boot_id", "next_sequence", "pending", "dropped", "last_accepted", "last_recorded",
            "gaps", "mock_since_accepted", "overflow", "batches", "recorded")
        fun empty(membershipId: String, enrollmentId: String) = TrackingBufferBook(membershipId, enrollmentId, emptyList())

        private fun int(v: Long) = SyncJson.Integer(v.toString())
        private fun opt(v: Int?) = v?.let { SyncJson.Integer(it.toString()) } ?: SyncJson.Null
        private fun at(v: Instant) = SyncJson.Text(EnterpriseTime.formatInstant(v))
        private fun fixJson(f: BufferedFix) = SyncJson.Array(listOf(at(f.at), int(f.elapsedMillis), int(f.latE7.toLong()), int(f.lonE7.toLong()),
            int(f.accuracyMetres.toLong()), opt(f.speedCmPerSecond), opt(f.bearingDegrees), SyncJson.Bool(f.coarse)))
        private fun sessionJson(s: SessionBuffer): SyncJson = SyncJson.Object(mapOf(
            "session_id" to SyncJson.Text(s.sessionId), "trip_id" to SyncJson.Text(s.tripId), "boot_id" to SyncJson.Text(s.bootId),
            "next_sequence" to int(s.nextSequence), "pending" to SyncJson.Array(s.pending.map(::fixJson)),
            "dropped" to SyncJson.Array(listOf(int(s.dropped.accuracy), int(s.dropped.stale), int(s.dropped.jump), int(s.dropped.mock))),
            "last_accepted" to (s.lastAcceptedAt?.let { SyncJson.Array(listOf(at(it), int(s.lastAcceptedElapsed!!), int(s.lastAcceptedLatE7!!.toLong()),
                int(s.lastAcceptedLonE7!!.toLong()))) } ?: SyncJson.Null),
            "last_recorded" to (s.lastRecorded?.let(::fixJson) ?: SyncJson.Null),
            "gaps" to SyncJson.Array(s.gaps.map { SyncJson.Array(listOf(SyncJson.Text(it.reason.name), at(it.start), it.end?.let(::at) ?: SyncJson.Null)) }),
            "mock_since_accepted" to SyncJson.Bool(s.mockSinceAccepted), "overflow" to int(s.overflow),
            "batches" to SyncJson.Array(s.batchOperationIds.map { SyncJson.Text(it) }), "recorded" to int(s.recorded),
        ))

        private fun obj(v: SyncJson?, keys: Set<String>): SyncJson.Object {
            val o = v as? SyncJson.Object ?: throw IllegalArgumentException("Invalid tracking buffer")
            require(o.fields.keys == keys) { "Invalid tracking buffer" }
            return o
        }
        private fun arr(v: SyncJson?): List<SyncJson> = (v as? SyncJson.Array)?.values ?: throw IllegalArgumentException("Invalid tracking buffer")
        private fun text(v: SyncJson?): String = (v as? SyncJson.Text)?.value ?: throw IllegalArgumentException("Invalid tracking buffer")
        private fun long(v: SyncJson?): Long = (v as? SyncJson.Integer)?.lexeme?.toLongOrNull() ?: throw IllegalArgumentException("Invalid tracking buffer")
        private fun nint(v: SyncJson?): Int? = if (v === SyncJson.Null) null else Math.toIntExact(long(v))
        private fun instant(v: SyncJson?): Instant {
            val raw = text(v); val parsed = EnterpriseTime.parseInstant(raw)
            require(EnterpriseTime.formatInstant(parsed) == raw); return parsed
        }
        private fun fix(v: SyncJson?): BufferedFix {
            val a = arr(v); require(a.size == 8)
            return BufferedFix(instant(a[0]), long(a[1]), Math.toIntExact(long(a[2])), Math.toIntExact(long(a[3])), Math.toIntExact(long(a[4])),
                nint(a[5]), nint(a[6]), (a[7] as? SyncJson.Bool)?.value ?: throw IllegalArgumentException())
        }

        fun decode(raw: ByteArray): TrackingBufferBook {
            val root = obj(CanonicalJson.parseLarge(raw, MAX_BYTES), setOf("format", "version", "membership_id", "enrollment_id", "sessions"))
            require(text(root.fields["format"]) == FORMAT && long(root.fields["version"]) == VERSION)
            val sessions = arr(root.fields["sessions"]).map { item ->
                val s = obj(item, SESSION_KEYS)
                val d = arr(s.fields["dropped"]); require(d.size == 4)
                val last = s.fields["last_accepted"].let { if (it === SyncJson.Null) null else arr(it).also { a -> require(a.size == 4) } }
                SessionBuffer(text(s.fields["session_id"]), text(s.fields["trip_id"]), text(s.fields["boot_id"]), long(s.fields["next_sequence"]),
                    arr(s.fields["pending"]).map(::fix), TrackingDropped(long(d[0]), long(d[1]), long(d[2]), long(d[3])),
                    last?.let { instant(it[0]) }, last?.let { long(it[1]) }, last?.let { Math.toIntExact(long(it[2])) }, last?.let { Math.toIntExact(long(it[3])) },
                    s.fields["last_recorded"].let { if (it === SyncJson.Null) null else fix(it) },
                    arr(s.fields["gaps"]).map { g -> arr(g).also { require(it.size == 3) }.let {
                        TrackingGap(TrackingGapReason.valueOf(text(it[0])), instant(it[1]), if (it[2] === SyncJson.Null) null else instant(it[2])) } },
                    (s.fields["mock_since_accepted"] as? SyncJson.Bool)?.value ?: throw IllegalArgumentException(), long(s.fields["overflow"]),
                    arr(s.fields["batches"]).map(::text), long(s.fields["recorded"]))
            }
            val book = TrackingBufferBook(text(root.fields["membership_id"]), text(root.fields["enrollment_id"]), sessions)
            require(book.canonicalBytes.contentEquals(raw))
            return book
        }
    }
}

/** Sealing pending fixes into batch envelopes (B-BR-EMP-504): ≤ 200 fixes and ≤ 8 KiB, never across a gap or a boot. */
object TrackingBatching {
    /** The oldest pending fixes that one batch may carry: a continuous run that does not cross a closed gap's start. */
    fun nextRun(buffer: SessionBuffer): List<BufferedFix> {
        val pending = buffer.pending
        if (pending.isEmpty()) return emptyList()
        val first = pending.first()
        val boundary = buffer.gaps.map { it.start }.filter { it.isAfter(first.at) }.minOrNull()
        val run = pending.takeWhile { boundary == null || !it.at.isAfter(boundary) }
        return run.take(TrackingBatchOperations.MAX_FIXES).takeWhile { it.at.toEpochMilli() - first.at.toEpochMilli() <= TrackingBatchOperations.MAX_OFFSET_MILLIS }
    }

    fun fixes(run: List<BufferedFix>): List<TrackingBatchFix> {
        val base = run.first().at.toEpochMilli()
        return run.map { TrackingBatchFix(it.at.toEpochMilli() - base, it.latE7, it.lonE7, it.accuracyMetres, it.speedCmPerSecond, it.bearingDegrees,
            if (it.coarse) TrackingBatchOperations.FLAG_COARSE else 0) }
    }

    /** Whether the pending fixes are due: forced, a full batch, or the oldest pending older than the seal interval. */
    fun due(buffer: SessionBuffer, now: Instant, force: Boolean): Boolean {
        val first = buffer.pending.firstOrNull() ?: return false
        return force || buffer.pending.size >= TrackingBatchOperations.MAX_FIXES ||
            now.toEpochMilli() - first.at.toEpochMilli() >= TrackingSamplingPolicy.SEAL_MILLIS
    }

    /** The buffer after its batch [fact] was sealed: those fixes leave, the sequence advances, the counters restart. */
    fun sealed(buffer: SessionBuffer, fact: TrackingBatchFact): SessionBuffer {
        val run = nextRun(buffer)
        val expected = fixes(run.take(fact.fixes.size))
        require(fact.sessionId == buffer.sessionId && fact.tripId == buffer.tripId && fact.sequence == buffer.nextSequence &&
            fact.fixes.size <= run.size && expected == fact.fixes && fact.declared.occurredAt == run.first().at && fact.dropped == buffer.dropped &&
            fact.bootId == buffer.bootId && fact.baseElapsedMillis == run.first().elapsedMillis) { "Batch does not match the buffer" }
        return buffer.copy(nextSequence = buffer.nextSequence + 1, pending = buffer.pending.drop(fact.fixes.size), dropped = TrackingDropped(),
            batchOperationIds = buffer.batchOperationIds + fact.operationId)
    }
}
