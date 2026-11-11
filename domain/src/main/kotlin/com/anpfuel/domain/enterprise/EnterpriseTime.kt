package com.anpfuel.domain.enterprise

import java.time.DateTimeException
import java.time.Duration
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneId

class EnterpriseTimeException : IllegalArgumentException("orgtime: invalid-time")

data class TimeDeclaration(val occurredAt: Instant, val entryZone: String, val deviceOffsetSeconds: Long)
data class ReportingZones(val organization: String, val unit: String? = null)

/** Historical value snapshot; only the owning server sets receivedAt. Values are not authority proof. */
data class EnterpriseTimeRecord(
    val occurredAt: Instant,
    val receivedAt: Instant,
    val entryZone: String,
    val deviceOffsetSeconds: Long,
    val reportingDay: String,
    val reportingZone: String,
    val reportingSource: String,
) {
    fun authorityAt(): Instant = receivedAt
}

/** device minus server = seconds + nonnegative nanos, including negative fractional differences. */
data class EnterpriseClockSkew(val seconds: Long, val nanos: Int, val limitSeconds: Long, val flag: String)

/** Pure time values; no membership resolution, sync acceptance, clock acquisition or persistence. */
object EnterpriseTime {
    const val DEFAULT_SKEW_LIMIT_SECONDS = 300L
    private val instantPattern = Regex(
        """^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-][0-9]{2}:[0-9]{2})$""",
    )
    private val zonePattern = Regex("^[A-Za-z0-9_+/-]{1,128}$")
    private val zoneIds = ZoneId.getAvailableZoneIds()
    private val trailingZeros = Regex("""(\.[0-9]*?)0+Z$""")

    private val minimum = Instant.parse("0001-01-01T00:00:00Z")
    private val maximum = Instant.parse("9999-12-31T23:59:59.999999999Z")

    fun parseInstant(raw: String): Instant {
        val match = if (raw.length in 20..35) instantPattern.matchEntire(raw) else null
        if (match == null || raw.startsWith("0000")) {
            throw EnterpriseTimeException()
        }
        val offset = match.groupValues[2]
        if (offset == "-00:00") throw EnterpriseTimeException()
        val at = try {
            OffsetDateTime.parse(raw).toInstant()
        } catch (_: DateTimeException) {
            throw EnterpriseTimeException()
        }
        if (!validInstant(at)) throw EnterpriseTimeException()
        return at
    }

    /** Canonical UTC for a previously validated instant, matching Go RFC3339Nano. */
    fun formatInstant(at: Instant): String = at.toString().replace(trailingZeros, "$1Z").replace(".Z", "Z")

    private fun validInstant(at: Instant): Boolean = at >= minimum && at <= maximum

    // The minimum instant is Go's unset time.Time; a server receipt never takes it.
    private fun validReceipt(at: Instant): Boolean = validInstant(at) && at != minimum

    private fun zone(id: String): ZoneId {
        if (!zonePattern.matches(id) ||
            (id != "UTC" && !id.contains('/')) ||
            id.contains("//") || id.startsWith("posix/") || id.startsWith("right/") ||
            id.startsWith("SystemV/") || id !in zoneIds
        ) throw EnterpriseTimeException()
        return ZoneId.of(id)
    }

    /** A trip caller supplies its start. A supplied invalid unit never falls back to organization. */
    fun capture(declared: TimeDeclaration, receivedAt: Instant, zones: ReportingZones): EnterpriseTimeRecord {
        if (!validInstant(declared.occurredAt) || !validReceipt(receivedAt) ||
            declared.deviceOffsetSeconds !in -64800L..64800L
        ) throw EnterpriseTimeException()
        zone(declared.entryZone)
        val organization = zone(zones.organization)
        val reportingZone = zones.unit?.let(::zone) ?: organization
        val day = declared.occurredAt.atZone(reportingZone).toLocalDate()
        if (day.year !in 1..9999) throw EnterpriseTimeException()
        return EnterpriseTimeRecord(
            declared.occurredAt, receivedAt, declared.entryZone, declared.deviceOffsetSeconds,
            day.toString(), reportingZone.id, if (zones.unit == null) "organization" else "unit",
        )
    }

    /** Authorization callers pass authorityAt(), never occurredAt. */
    fun inPeriod(at: Instant, start: Instant, end: Instant): Boolean {
        if (!validInstant(at) || !validInstant(start) || !validInstant(end) || start >= end) {
            throw EnterpriseTimeException()
        }
        return at >= start && at < end
    }

    /** One-way sync sample estimate; transit delay can affect it. Never rewrites either input. */
    fun assessClockSkew(
        deviceSample: Instant,
        serverReceipt: Instant,
        limitSeconds: Long? = null,
    ): EnterpriseClockSkew {
        val limit = limitSeconds ?: DEFAULT_SKEW_LIMIT_SECONDS
        if (!validInstant(deviceSample) || !validReceipt(serverReceipt) || limit < 0) {
            throw EnterpriseTimeException()
        }
        val delta = Duration.between(serverReceipt, deviceSample)
        val exceeds = delta.seconds < -limit || delta.seconds > limit ||
            (delta.seconds == limit && delta.nano > 0)
        return EnterpriseClockSkew(delta.seconds, delta.nano, limit, if (exceeds) "CLOCK_SKEW" else "")
    }
}
