package com.anpfuel.domain.routing

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*

class GuidancePolicyTest {
    private fun req(lat: Double = -25.43) = NativeRouteRequest(
        listOf(RoutePoint(lat, -49.27, "A"), RoutePoint(-25.45, -49.29, "B")),
    )

    @Test fun `same planning request binds launch and altered routes refuse`() {
        assertTrue(GuidancePolicy.sameRoute(req(), req()))
        assertFalse(GuidancePolicy.sameRoute(req(), req(lat = -25.44)))
        val altered = req().copy(avoid = setOf(RoadAvoidance.TOLL))
        assertFalse(GuidancePolicy.sameRoute(req(), altered))
    }

    @Test fun `reroute beyond sixty seconds is recoverable failure`() {
        assertFalse(GuidancePolicy.isRerouteExpired(0L, 60_000_000_000L))
        assertTrue(GuidancePolicy.isRerouteExpired(0L, 60_000_000_001L))
        assertFalse(GuidancePolicy.isRerouteExpired(100L, 50L))
    }

    @Test fun `native locale mapping never invents voice codes`() {
        assertEquals("pt-BR", GuidancePolicy.nativeLocaleFor("pt"))
        assertEquals("pt-BR", GuidancePolicy.nativeLocaleFor("pt-BR"))
        assertEquals("zh-Hans", GuidancePolicy.nativeLocaleFor("zh"))
        assertEquals("zh-Hans", GuidancePolicy.nativeLocaleFor("zh-CN"))
        assertEquals("en", GuidancePolicy.nativeLocaleFor("en"))
        assertNull(GuidancePolicy.nativeLocaleFor("xx"))
        assertNull(GuidancePolicy.nativeLocaleFor(""))
    }
}
