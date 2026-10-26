package com.anpfuel.maps.engine

import com.anpfuel.domain.ev.EvRoadLeg
import com.anpfuel.domain.ev.EvRoadUnavailable
import com.anpfuel.domain.routing.NativeRoutePlanner
import com.anpfuel.domain.routing.NativeRoutePreview
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.NativeRouteResult
import com.anpfuel.domain.routing.RoutePoint
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.async
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class NativeEvRoadRouterTest {
    private val a = RoutePoint(-25.0, -49.0, "a")
    private val b = RoutePoint(-24.9, -49.0, "b")
    private val c = RoutePoint(-24.8, -49.0, "c")
    private val d = RoutePoint(-24.7, -49.0, "d")

    private class FakePlanner : NativeRoutePlanner {
        val callbacks = mutableListOf<(NativeRouteResult) -> Unit>()
        var cancels = 0
        override fun buildRoute(request: NativeRouteRequest, onResult: (NativeRouteResult) -> Unit): Boolean {
            callbacks += onResult
            return true
        }
        override fun cancelRoute() {
            cancels++
        }
    }

    private fun preview(distance: Double, missing: Boolean = false) = NativeRoutePreview(
        distanceMeters = distance,
        seconds = 100,
        geometry = listOf(a, b),
        maneuvers = emptyList(),
        warnings = missing,
    )

    @Test
    fun `late callback from an old generation cannot publish`() = runTest {
        val planner = FakePlanner()
        val router = NativeEvRoadRouter(planner, StandardTestDispatcher(testScheduler), timeoutMillis = 60_000)
        val first = async { router.leg(a, b, "en") }
        runCurrent()
        val second = async { router.leg(c, d, "en") }
        runCurrent()
        assertEquals(2, planner.callbacks.size)
        planner.callbacks[0](NativeRouteResult.Ready(preview(1_000.0)))
        runCurrent()
        assertFalse(first.isCompleted)
        planner.callbacks[1](NativeRouteResult.Ready(preview(2_000.0)))
        runCurrent()
        val result = second.await()
        assertTrue(result is EvRoadLeg.Ready)
        assertEquals(2_000L, (result as EvRoadLeg.Ready).distanceMeters)
        first.cancelAndJoin()
    }

    @Test
    fun `missing maps are refused and a timeout cancels the native route`() = runTest {
        val planner = FakePlanner()
        val router = NativeEvRoadRouter(planner, StandardTestDispatcher(testScheduler), timeoutMillis = 1_000)
        val missing = async { router.leg(a, b, "en") }
        runCurrent()
        planner.callbacks.single()(NativeRouteResult.Failed(9, listOf("Brazil_Santa_Catarina")))
        runCurrent()
        assertEquals(EvRoadUnavailable.MISSING_MAPS, (missing.await() as EvRoadLeg.Unavailable).reason)

        val timed = async { router.leg(a, b, "en") }
        runCurrent()
        advanceTimeBy(1_001)
        runCurrent()
        assertEquals(EvRoadUnavailable.TIMEOUT, (timed.await() as EvRoadLeg.Unavailable).reason)
        assertTrue(planner.cancels > 0)
    }

    @Test
    fun `explicit cancellation cancels the native route`() = runTest {
        val planner = FakePlanner()
        val router = NativeEvRoadRouter(planner, StandardTestDispatcher(testScheduler))
        val job = async { router.leg(a, b, "en") }
        runCurrent()
        job.cancelAndJoin()
        runCurrent()
        assertTrue(planner.cancels > 0)
    }
}
