package com.anpfuel.domain.driver

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class DriverDistanceTest {
    private val w = DriverWindow("owner", "car", "BRL", 0, 1000)
    @Test fun `odometer and GPS remain alternatives rather than 390 km of combined travel`() {
        val selected = DriverDistance.select(w, DriverDistanceBasis.ODOMETER, listOf(
            DriverDistanceObservation("odo", w, "s1", DriverDistanceBasis.ODOMETER, 200000, true),
            DriverDistanceObservation("gps", w, "s1", DriverDistanceBasis.GPS_ESTIMATE, 190000, true),
        ))
        assertEquals(200000L, selected.metres)
    }
    @Test fun `complete paid classification yields sixty percent and unknown partitions stay unknown`() {
        assertEquals(DriverDistancePartition(80000, "60"), DriverDistance.partition(200000, 200000, 120000, 0, true))
        assertNull(DriverDistance.partition(200000, 200000, 120000, null, false).paidPercent)
        assertThrows(IllegalArgumentException::class.java) { DriverDistance.partition(200000, 100000, 120000, 100000, true) }
        assertThrows(IllegalArgumentException::class.java) { DriverDistance.partition(200000, 200000, 120000, 1000, true) }
    }
    @Test fun `reversed readings segment crossings predictions gaps and foreign windows refuse metrics`() {
        assertNull(DriverDistance.odometer(200, 100, "a", "a").metres)
        assertNull(DriverDistance.odometer(100, 200, "a", "b").metres)
        assertEquals(100L, DriverDistance.odometer(100, 200, "a", "a").metres)
        assertNull(DriverDistance.select(w, DriverDistanceBasis.ROUTE_PREDICTION, emptyList()).metres)
        val o = DriverDistanceObservation("a", w, "s", DriverDistanceBasis.ODOMETER, 100, true)
        assertNull(DriverDistance.select(w, o.basis, listOf(o.copy(hasGap = true))).metres)
        assertNull(DriverDistance.select(w, o.basis, listOf(o, o.copy(id = "duplicate-basis"))).metres)
        assertNull(DriverDistance.select(w, o.basis, listOf(o.copy(window = w.copy(owner = "foreign")))).metres)
    }

}
