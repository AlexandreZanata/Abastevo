package com.anpfuel.domain.enterprise.meter

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseMeterTest {
    @Test fun `documented exact decimals preserve all integer units`() {
        assertEquals(184_532_400L, MeterValues.kilometres("184532.4").units)
        assertEquals(15_165_000L, MeterValues.hours("4212.5").units)
        assertEquals(36L, MeterValues.hours("0.01").units)
        assertEquals(12_600L, MeterValues.hoursMinutes("3", "30").units)
    }

    @Test fun `zero observation and explicit missing remain different evidence`() {
        val zero = MeterPresence.observed(MeterValues.kilometres("0"))
        val missing = MeterPresence.missing("Display is unreadable")
        assertEquals(0L, zero.value.units)
        assertEquals("Display is unreadable", missing.reason)
        assertNotEquals(zero, missing)
        for (reason in listOf("", " ", "\u00a0", "reason\n", "x".repeat(281), String(charArrayOf('\ud800'))))
            assertThrows(MeterDomainException::class.java) { MeterPresence.missing(reason) }
    }

    @Test fun `bounds are inclusive and no larger number can wrap`() {
        assertEquals(1_000_000_000_000L, MeterValues.kilometres("1000000000").units)
        assertEquals(3_600_000_000L, MeterValues.hours("1000000").units)
        assertEquals(3_600_000_000L, MeterValues.hoursMinutes("1000000", "0").units)
        for (raw in listOf("1000000000.001", "9223372036854775808", "9".repeat(1000)))
            assertThrows(MeterDomainException::class.java) { MeterValues.kilometres(raw) }
        assertThrows(MeterDomainException::class.java) { MeterValues.hours("1000000.01") }
        assertThrows(MeterDomainException::class.java) { MeterValues.hoursMinutes("1000000", "1") }
    }

    @Test fun `meter kinds remain distinct and a modulus is strict`() {
        val distance = MeterValue.of(MeterKind.ODOMETER, 1)
        val seconds = MeterValue.of(MeterKind.HOUR_METER, 1)
        assertNotEquals(distance.kind, seconds.kind)
        assertEquals(999L, MeterValues.kilometres("0.999", 1000).units)
        assertThrows(MeterDomainException::class.java) { MeterValues.kilometres("1", 1000) }
        assertThrows(MeterDomainException::class.java) { MeterValues.hours("0", 0) }
        assertThrows(MeterDomainException::class.java) { MeterValue.of(MeterKind.HOUR_METER, -1) }
    }

    @Test fun `canonical decimal grammar refuses normalization rounding and alternative numerals`() {
        for (raw in listOf("", "+1", "-1", "01", "00", ".5", "1.", "1,5", "1e3", " 1", "1 ", "1\n", "１", "١", "1_000", "1.0000", "NaN"))
            assertThrows(MeterDomainException::class.java, { MeterValues.kilometres(raw) }, "Noncanonical kilometre input")
        for (raw in listOf("1.000", "0.001", "00.01"))
            assertThrows(MeterDomainException::class.java) { MeterValues.hours(raw) }
        for ((hours, minutes) in listOf("1.5" to "0", "1" to "60", "1" to "-1", "1" to "01", "1" to "1.0"))
            assertThrows(MeterDomainException::class.java) { MeterValues.hoursMinutes(hours, minutes) }
    }

    @Test fun `exact conversion retains useful trailing precision and redacts diagnostics`() {
        val distance = MeterValues.kilometres("1.001")
        assertEquals(1001L, distance.units)
        assertEquals(1000L, MeterValues.kilometres("1.000").units)
        assertEquals(3600L, MeterValues.hours("1.00").units)
        assertEquals("EnterpriseMeterValue(redacted)", distance.toString())
        assertEquals("EnterpriseMeterPresence(redacted)", MeterPresence.missing("Synthetic private explanation").toString())
    }
}
