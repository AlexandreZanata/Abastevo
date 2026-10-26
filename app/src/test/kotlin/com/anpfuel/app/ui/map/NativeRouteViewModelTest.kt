package com.anpfuel.app.ui.map

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.domain.routing.*
import com.anpfuel.maps.engine.MapEngine
import io.mockk.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.*
import org.junit.jupiter.api.Assertions.*

@OptIn(ExperimentalCoroutinesApi::class)
class NativeRouteViewModelTest {
    private val dispatcher=StandardTestDispatcher()
    private val start=RoutePoint(-25.43,-49.27,"Origin")
    private val end=RoutePoint(-25.42,-49.26,"Destination")
    private val preview=NativeRoutePreview(1575.15,288,listOf(start,end),listOf(RouteManeuver("Turn",100.0,"Street")),false)
    @BeforeEach fun setup(){Dispatchers.setMain(dispatcher)}
    @AfterEach fun teardown(){Dispatchers.resetMain()}
    @Test fun `native route carries ordered stops and options without requesting GPS`()=runTest {
        val engine=mockk<MapEngine>(relaxed=true);val request=slot<NativeRouteRequest>()
        every {engine.buildRoute(capture(request),any())} returns true
        val vm=NativeRouteViewModel(engine,SavedStateHandle())
        vm.point(RouteInputSlot.ORIGIN,start);vm.point(RouteInputSlot.DESTINATION,end)
        val first=RoutePoint(-25.425,-49.265,"one");val second=RoutePoint(-25.423,-49.263,"two")
        vm.point(RouteInputSlot.STOP,first);vm.point(RouteInputSlot.STOP,second);vm.moveStop(1,-1)
        vm.avoidance(RoadAvoidance.TOLL);vm.build("pt")
        assertEquals(listOf(start,second,first,end),request.captured.points)
        assertEquals(setOf(RoadAvoidance.TOLL),request.captured.avoid)
        verify(exactly=0){engine.recenter()};vm.cancel()
    }
    @Test fun `obsolete build callback cannot replace changed request and missing maps revoke preview`()=runTest {
        val engine=mockk<MapEngine>(relaxed=true);val callbacks=mutableListOf<(NativeRouteResult)->Unit>()
        every {engine.buildRoute(any(),any())} answers {callbacks+=secondArg<(NativeRouteResult)->Unit>();true}
        val vm=NativeRouteViewModel(engine,SavedStateHandle())
        vm.point(RouteInputSlot.ORIGIN,start);vm.point(RouteInputSlot.DESTINATION,end);vm.build("en")
        vm.avoidance(RoadAvoidance.FERRY);callbacks.first()(NativeRouteResult.Ready(preview))
        assertNull(vm.ui.value.preview)
        vm.build("en");callbacks.last()(NativeRouteResult.Ready(preview));assertNotNull(vm.ui.value.preview)
        callbacks.last()(NativeRouteResult.Failed(9,listOf("Brazil_Santa Catarina")))
        assertNull(vm.ui.value.preview);assertEquals(listOf("Brazil_Santa Catarina"),vm.ui.value.missingMaps)
        vm.cancel();callbacks.last()(NativeRouteResult.Ready(preview));assertNull(vm.ui.value.preview)
    }
    @Test fun `manual invalid equal points and missing origin never build a drivable route`()=runTest {
        val engine=mockk<MapEngine>(relaxed=true);val vm=NativeRouteViewModel(engine,SavedStateHandle())
        vm.build("en");vm.manual(RouteInputSlot.ORIGIN,"NaN","0","bad");vm.point(RouteInputSlot.ORIGIN,start);vm.point(RouteInputSlot.DESTINATION,start);vm.build("en")
        verify(exactly=0){engine.buildRoute(any(),any())};assertNull(vm.ui.value.preview)
    }
    @Test fun `saved plan restores points and avoid without a native route`()=runTest {
        val engine=mockk<MapEngine>(relaxed=true)
        val saved=SavedStateHandle()
        val vm=NativeRouteViewModel(engine,saved)
        val via=RoutePoint(-25.425,-49.265,"saved via")
        vm.restorePlan(listOf(start,via,end),setOf(RoadAvoidance.TOLL))
        assertEquals(start,vm.ui.value.origin)
        assertEquals(end,vm.ui.value.destination)
        assertEquals(listOf(via),vm.ui.value.stops)
        assertEquals(setOf(RoadAvoidance.TOLL),vm.ui.value.avoid)
        val restored=NativeRouteViewModel(engine,saved)
        assertEquals(start,restored.ui.value.origin)
        assertEquals(listOf(via),restored.ui.value.stops)
        verify(exactly=0){engine.buildRoute(any(),any())}
    }
    @Test fun `timeout cancels native work and configuration restores only planning inputs`()=runTest {
        val engine=mockk<MapEngine>(relaxed=true);every{engine.buildRoute(any(),any())}returns true
        val saved=SavedStateHandle();val vm=NativeRouteViewModel(engine,saved)
        vm.point(RouteInputSlot.ORIGIN,start);vm.point(RouteInputSlot.DESTINATION,end);vm.avoidance(RoadAvoidance.MOTORWAY);vm.build("en")
        advanceTimeBy(90_001);runCurrent();assertEquals("timeout",vm.ui.value.error);assertFalse(vm.ui.value.building)
        val restored=NativeRouteViewModel(engine,saved)
        assertEquals(start,restored.ui.value.origin);assertEquals(setOf(RoadAvoidance.MOTORWAY),restored.ui.value.avoid);assertNull(restored.ui.value.preview)
    }
}
