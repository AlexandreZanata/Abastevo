package com.anpfuel.domain.enterprise.meter

enum class MeterFailure { INVALID, SCOPE, READING_ALREADY_SUPERSEDED, OPERATION_ID_REUSED, METER_ALREADY_IN_SERVICE }
class MeterDomainException(val failure: MeterFailure = MeterFailure.INVALID) : IllegalArgumentException("meter: ${failure.name.lowercase()}")
private fun invalid(): MeterDomainException = MeterDomainException()
enum class MeterKind(val unit: String) { ODOMETER("m"), HOUR_METER("s");
    companion object { fun parse(raw: String): MeterKind = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}

/** Independent exact units; neither value exposes cross-kind arithmetic. */
sealed class MeterValue {
    abstract val kind: MeterKind
    abstract val units: Long
    class Metres internal constructor(override val units: Long) : MeterValue() {
        override val kind = MeterKind.ODOMETER
        init { if (units !in 0..1_000_000_000_000L) throw invalid() }
    }
    class Seconds internal constructor(override val units: Long) : MeterValue() {
        override val kind = MeterKind.HOUR_METER
        init { if (units !in 0..3_600_000_000L) throw invalid() }
    }
    final override fun equals(other: Any?): Boolean = other is MeterValue && kind == other.kind && units == other.units
    final override fun hashCode(): Int = 31 * kind.hashCode() + units.hashCode()
    final override fun toString(): String = "EnterpriseMeterValue(redacted)"
    companion object {
        fun of(kind: MeterKind, units: Long, modulus: Long? = null): MeterValue {
            if (modulus != null && (modulus <= 0 || units >= modulus)) throw invalid()
            return when (kind) {
                MeterKind.ODOMETER -> Metres(units)
                MeterKind.HOUR_METER -> Seconds(units)
            }
        }
    }
}

private fun whole(raw: String): Long {
    if (raw.isEmpty() || raw.length > 19 || raw.length > 1 && raw[0] == '0' || raw.any { it !in '0'..'9' }) throw invalid()
    return raw.toLongOrNull() ?: throw invalid()
}

enum class MeterEntryForm { BASE, KM, HOURS, HM;
    companion object { fun parse(raw: String): MeterEntryForm = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}

object MeterValues {
    fun parse(kind: MeterKind, form: MeterEntryForm, raw: String, minutes: String? = null, modulus: Long? = null): MeterValue {
        val value = when (form) {
            MeterEntryForm.BASE -> MeterValue.of(kind, whole(raw), modulus)
            MeterEntryForm.KM -> kilometres(raw, modulus)
            MeterEntryForm.HOURS -> hours(raw, modulus)
            MeterEntryForm.HM -> hoursMinutes(raw, minutes ?: throw invalid(), modulus)
        }
        if (value.kind != kind) throw invalid()
        return value
    }

    private fun decimal(raw: String, precision: Int, multiplier: Long, kind: MeterKind, modulus: Long?): MeterValue {
        val dot = raw.indexOf('.')
        val integer = whole(if (dot < 0) raw else raw.substring(0, dot))
        val fractional = if (dot < 0) 0L else {
            val digits = raw.substring(dot + 1)
            if (digits.isEmpty() || digits.length > precision || digits.any { it !in '0'..'9' }) throw invalid()
            val scale = if (digits.length == 1) 10L else if (digits.length == 2) 100L else 1000L
            digits.toLong() * multiplier / scale
        }
        val result = try { Math.addExact(Math.multiplyExact(integer, multiplier), fractional) } catch (_: ArithmeticException) { throw invalid() }
        return MeterValue.of(kind, result, modulus)
    }
    fun kilometres(raw: String, modulus: Long? = null): MeterValue = decimal(raw, 3, 1000, MeterKind.ODOMETER, modulus)
    fun hours(raw: String, modulus: Long? = null): MeterValue = decimal(raw, 2, 3600, MeterKind.HOUR_METER, modulus)
    fun hoursMinutes(hours: String, minutes: String, modulus: Long? = null): MeterValue {
        val h = whole(hours)
        val m = whole(minutes)
        if (m !in 0..59) throw invalid()
        val result = try { Math.addExact(Math.multiplyExact(h, 3600L), m * 60) } catch (_: ArithmeticException) { throw invalid() }
        return MeterValue.of(MeterKind.HOUR_METER, result, modulus)
    }
}

internal fun meterReason(raw: String): String {
    if (raw.isBlank() || raw.length > 560 || raw.codePointCount(0, raw.length) > 280) throw invalid()
    val points = raw.codePoints().iterator()
    while (points.hasNext()) {
        val point = points.nextInt()
        if (point in 0xd800..0xdfff || Character.isISOControl(point)) throw invalid()
    }
    return raw
}

sealed class MeterPresence {
    class Observed internal constructor(val value: MeterValue) : MeterPresence()
    class Missing internal constructor(val reason: String) : MeterPresence() {
        init { meterReason(reason) }
    }
    final override fun toString(): String = "EnterpriseMeterPresence(redacted)"
    companion object {
        fun observed(value: MeterValue): Observed = Observed(value)
        fun missing(reason: String): Missing = Missing(reason)
    }
}
