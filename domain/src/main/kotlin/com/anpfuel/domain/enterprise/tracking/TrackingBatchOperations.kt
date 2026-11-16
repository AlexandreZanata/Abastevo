package com.anpfuel.domain.enterprise.tracking

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncJson

/*
 * EMP-GPS-003 (B-BR-EMP-504, EDR-326): the `gps.track.batch` schema 1 envelope exactly as the EMP-GPS-004 intake parses it
 * (ParseBatch), pinned by contracts/testdata/enterprise/tracking/track-batch-v1.psv. A fix is
 * [offset_ms, Δlat_e7, Δlon_e7, accuracy_m, speed_cm_s|null, bearing_deg|null, flags], each delta from the previous fix and
 * the first from the base; the base is the first fix and its instant is the operation's declared instant.
 */

/** One fix of a batch with its absolute position; the builder writes the deltas. */
class TrackingBatchFix(val offsetMillis: Long, val latE7: Int, val lonE7: Int, val accuracyMetres: Int, val speedCmPerSecond: Int?,
    val bearingDegrees: Int?, val flags: Int) {
    override fun equals(other: Any?): Boolean = other is TrackingBatchFix && offsetMillis == other.offsetMillis && latE7 == other.latE7 &&
        lonE7 == other.lonE7 && accuracyMetres == other.accuracyMetres && speedCmPerSecond == other.speedCmPerSecond &&
        bearingDegrees == other.bearingDegrees && flags == other.flags
    override fun hashCode(): Int = java.util.Objects.hash(offsetMillis, latE7, lonE7, accuracyMetres, speedCmPerSecond, bearingDegrees, flags)
    override fun toString(): String = "EnterpriseTrackingBatchFix(redacted)"
}

/** The device's per-batch counters of fixes it did not record (BR-GPS-017). */
data class TrackingDropped(val accuracy: Long = 0, val stale: Long = 0, val jump: Long = 0, val mock: Long = 0) {
    init { require(listOf(accuracy, stale, jump, mock).all { it in 0..TrackingBatchOperations.MAX_DROPPED }) }
    fun plus(reason: TrackingDropReason): TrackingDropped = when (reason) {
        TrackingDropReason.ACCURACY -> copy(accuracy = minOf(accuracy + 1, TrackingBatchOperations.MAX_DROPPED))
        TrackingDropReason.STALE -> copy(stale = minOf(stale + 1, TrackingBatchOperations.MAX_DROPPED))
        TrackingDropReason.JUMP -> copy(jump = minOf(jump + 1, TrackingBatchOperations.MAX_DROPPED))
        TrackingDropReason.MOCK -> copy(mock = minOf(mock + 1, TrackingBatchOperations.MAX_DROPPED))
    }
    override fun toString(): String = "EnterpriseTrackingDropped(redacted)"
}

enum class TrackingDropReason { ACCURACY, STALE, JUMP, MOCK }

/** One decoded batch; it proves only its own shape. */
class TrackingBatchFact(val operationId: String, val declared: com.anpfuel.domain.enterprise.TimeDeclaration, dependsOn: List<String>,
    val sessionId: String, val tripId: String, val sequence: Long, val bootId: String?, val baseElapsedMillis: Long?,
    fixes: List<TrackingBatchFix>, val dropped: TrackingDropped) {
    private val dependencies = dependsOn.toList()
    private val values = fixes.toList()
    val dependsOn: List<String> get() = dependencies.toList()
    val fixes: List<TrackingBatchFix> get() = values.toList()
    override fun toString(): String = "EnterpriseTrackingBatchFact(redacted)"
}

private fun malformed() = TrackingOperationException()
private fun identity(raw: String): String = try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw malformed() }
private val BOOT = Regex("^[A-Za-z0-9._-]{1,64}$")
private val BATCH_KEYS = setOf("trip_id", "seq", "source", "boot_id", "base_elapsed_millis", "base", "fixes", "dropped")

object TrackingBatchOperations {
    const val TYPE = "gps.track.batch"
    const val SOURCE = "PHONE"
    const val MAX_FIXES = 200
    const val MAX_OFFSET_MILLIS = 24L * 60 * 60 * 1000
    const val MAX_SEQUENCE = (1L shl 31) - 1
    const val MAX_ACCURACY = 10_000
    const val MAX_SPEED = 100_000
    const val MAX_DROPPED = 1_000_000L
    const val MAX_ELAPSED = 1L shl 53
    const val FLAG_COARSE = 1
    const val FLAG_MOCK = 2
    private const val MAX_LAT = 900_000_000L
    private const val MAX_LON = 1_800_000_000L

    private fun integer(value: Long) = SyncJson.Integer(value.toString())
    private fun optional(value: Int?) = value?.let { SyncJson.Integer(it.toString()) } ?: SyncJson.Null

    /**
     * Builds one batch; refuses anything the intake parser refuses (MALFORMED) and an operation above the 8 KiB protocol
     * bound. The base is the first fix; [dependsOn] names the session operation that opened the fixes' recording period.
     */
    fun batch(envelope: TrackingEnvelope, sessionId: String, tripId: String, sequence: Long, bootId: String?, baseElapsedMillis: Long?,
        fixes: List<TrackingBatchFix>, dropped: TrackingDropped, dependsOn: List<String>): EnterpriseOperation {
        val trip = identity(tripId)
        if (sequence !in 1..MAX_SEQUENCE || fixes.isEmpty() || fixes.size > MAX_FIXES || (bootId == null) != (baseElapsedMillis == null) ||
            bootId != null && !BOOT.matches(bootId) || baseElapsedMillis != null && baseElapsedMillis !in 0..MAX_ELAPSED) throw malformed()
        var previous = -1L
        for (f in fixes) {
            if (f.offsetMillis !in 0..MAX_OFFSET_MILLIS || f.offsetMillis <= previous || f.latE7 !in -MAX_LAT..MAX_LAT || f.lonE7 !in -MAX_LON..MAX_LON ||
                f.accuracyMetres !in 0..MAX_ACCURACY || f.speedCmPerSecond?.let { it !in 0..MAX_SPEED } == true ||
                f.bearingDegrees?.let { it !in 0..359 } == true || f.flags !in 0..(FLAG_COARSE or FLAG_MOCK)) throw malformed()
            previous = f.offsetMillis
        }
        val base = fixes.first()
        var lat = base.latE7.toLong(); var lon = base.lonE7.toLong()
        val encoded = fixes.map { f ->
            val row = SyncJson.Array(listOf(integer(f.offsetMillis), integer(f.latE7 - lat), integer(f.lonE7 - lon), integer(f.accuracyMetres.toLong()),
                optional(f.speedCmPerSecond), optional(f.bearingDegrees), integer(f.flags.toLong())))
            lat = f.latE7.toLong(); lon = f.lonE7.toLong()
            row
        }
        val payload = mapOf(
            "trip_id" to SyncJson.Text(trip), "seq" to integer(sequence), "source" to SyncJson.Text(SOURCE),
            "boot_id" to (bootId?.let { SyncJson.Text(it) } ?: SyncJson.Null), "base_elapsed_millis" to (baseElapsedMillis?.let(::integer) ?: SyncJson.Null),
            "base" to SyncJson.Object(mapOf("lat_e7" to integer(base.latE7.toLong()), "lon_e7" to integer(base.lonE7.toLong()))),
            "fixes" to SyncJson.Array(encoded),
            "dropped" to SyncJson.Object(mapOf("accuracy" to integer(dropped.accuracy), "stale" to integer(dropped.stale),
                "jump" to integer(dropped.jump), "mock" to integer(dropped.mock))),
        )
        return TrackingOperations.sealOperation(envelope, TYPE, sessionId, dependsOn, payload)
    }

    private fun exact(value: SyncJson?, keys: Set<String>): SyncJson.Object {
        val obj = value as? SyncJson.Object ?: throw malformed()
        if (obj.fields.keys != keys) throw malformed()
        return obj
    }
    private fun long(value: SyncJson?, low: Long, high: Long): Long {
        val lexeme = (value as? SyncJson.Integer)?.lexeme ?: throw malformed()
        val parsed = lexeme.toLongOrNull() ?: throw malformed()
        if (parsed !in low..high) throw malformed()
        return parsed
    }
    private fun nullableInt(value: SyncJson?, low: Long, high: Long): Int? = if (value === SyncJson.Null) null else long(value, low, high).toInt()

    /** Strict decode of a device batch; anything else is MALFORMED (the intake's ParseBatch). */
    fun read(operation: EnterpriseOperation): TrackingBatchFact {
        val entity = operation.entity
        if (operation.type != TYPE || !operation.supportsSchema || entity.reference.type != TrackingOperations.ENTITY || entity.expectedRevision != null ||
            operation.attachments.isNotEmpty() || operation.softReferences.isNotEmpty() || operation.declaration.occurredAt.nano % 1000 != 0 ||
            operation.dependsOn.size > TrackingOperations.MAX_DEPENDENCIES) throw malformed()
        val fields = exact(operation.payload, BATCH_KEYS)
        val trip = identity((fields.fields["trip_id"] as? SyncJson.Text)?.value ?: throw malformed())
        val sequence = long(fields.fields["seq"], 1, MAX_SEQUENCE)
        if ((fields.fields["source"] as? SyncJson.Text)?.value != SOURCE) throw malformed()
        val boot = fields.fields["boot_id"].let { if (it === SyncJson.Null) null else ((it as? SyncJson.Text)?.value ?: throw malformed()) }
        if (boot != null && !BOOT.matches(boot)) throw malformed()
        val elapsed = fields.fields["base_elapsed_millis"].let { if (it === SyncJson.Null) null else long(it, 0, MAX_ELAPSED) }
        if ((boot == null) != (elapsed == null)) throw malformed()
        val base = exact(fields.fields["base"], setOf("lat_e7", "lon_e7"))
        var lat = long(base.fields["lat_e7"], -MAX_LAT, MAX_LAT)
        var lon = long(base.fields["lon_e7"], -MAX_LON, MAX_LON)
        val dropped = exact(fields.fields["dropped"], setOf("accuracy", "stale", "jump", "mock")).let {
            TrackingDropped(long(it.fields["accuracy"], 0, MAX_DROPPED), long(it.fields["stale"], 0, MAX_DROPPED),
                long(it.fields["jump"], 0, MAX_DROPPED), long(it.fields["mock"], 0, MAX_DROPPED))
        }
        val list = (fields.fields["fixes"] as? SyncJson.Array)?.values ?: throw malformed()
        if (list.isEmpty() || list.size > MAX_FIXES) throw malformed()
        var previous = -1L
        val fixes = list.map { raw ->
            val values = (raw as? SyncJson.Array)?.values ?: throw malformed()
            if (values.size != 7) throw malformed()
            val offset = long(values[0], 0, MAX_OFFSET_MILLIS)
            lat += long(values[1], -2 * MAX_LAT, 2 * MAX_LAT)
            lon += long(values[2], -2 * MAX_LON, 2 * MAX_LON)
            if (offset <= previous || lat !in -MAX_LAT..MAX_LAT || lon !in -MAX_LON..MAX_LON) throw malformed()
            previous = offset
            TrackingBatchFix(offset, lat.toInt(), lon.toInt(), long(values[3], 0, MAX_ACCURACY.toLong()).toInt(),
                nullableInt(values[4], 0, MAX_SPEED.toLong()), nullableInt(values[5], 0, 359), long(values[6], 0, (FLAG_COARSE or FLAG_MOCK).toLong()).toInt())
        }
        return TrackingBatchFact(operation.operationId, operation.declaration, operation.dependsOn, identity(entity.reference.id), trip, sequence,
            boot, elapsed, fixes, dropped)
    }
}
