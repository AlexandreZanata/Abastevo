package com.anpfuel.app.ui.map

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.domain.routing.*
import com.anpfuel.maps.engine.MapEngineHolder
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Explicit local native fixture matrix; no API, external intent or fake engine. */
@RunWith(AndroidJUnit4::class)
class NativeRoutePlannerInstrumentedTest {
    private val instrumentation=InstrumentationRegistry.getInstrumentation()
    private val engine get()=MapEngineHolder.engine
    private fun main(action:()->Unit)=instrumentation.runOnMainSync(action)
    private fun initialize() {
        val context=ApplicationProvider.getApplicationContext<Context>()
        assertTrue("admitted regional fixture is required",File(context.filesDir,"261004/Brazil_Parana_East.mwm").isFile)
        val ready=CountDownLatch(1)
        main { assertTrue(engine.initialize(context){ready.countDown()}) }
        assertTrue("native readiness",ready.await(45,TimeUnit.SECONDS))
        main { assertTrue(engine.reloadMaps()) }
    }
    private fun route(points:List<RoutePoint>, avoid:Set<RoadAvoidance> = emptySet()):NativeRouteResult {
        val result=AtomicReference<NativeRouteResult>();val latch=CountDownLatch(1)
        main {assertTrue(engine.buildRoute(NativeRouteRequest(points,avoid,"en")){result.set(it);latch.countDown()})}
        assertTrue("native routing callback",latch.await(60,TimeUnit.SECONDS))
        return result.get()
    }
    @Test fun offlineNativeCarRouteStopsManeuversOptionsAndCancellation() {
        initialize()
        val start=RoutePoint(-25.4295,-49.2719,"Synthetic origin")
        val destination=RoutePoint(-25.4222,-49.2640,"Synthetic destination")
        val first=route(listOf(start,destination))
        assertTrue("actual native route expected: $first",first is NativeRouteResult.Ready)
        val preview=(first as NativeRouteResult.Ready).preview
        assertTrue(preview.distanceMeters>0);assertTrue(preview.seconds>0);assertTrue(preview.geometry.size>2);assertTrue("original junctions must stay bounded",preview.geometry.size<5000)
        assertTrue("native instructions",preview.maneuvers.any { it.instruction.isNotBlank() })
        val stop=RoutePoint(-25.4280,-49.2630,"Synthetic stop")
        val stopped=route(listOf(start,stop,destination),setOf(RoadAvoidance.TOLL,RoadAvoidance.FERRY))
        assertTrue("native multi-stop route: $stopped",stopped is NativeRouteResult.Ready)
        assertTrue((stopped as NativeRouteResult.Ready).preview.geometry.any { kotlin.math.abs(it.latitude-stop.latitude)<0.002 && kotlin.math.abs(it.longitude-stop.longitude)<0.002 })
        main { engine.cancelRoute() }
        val replacement=route(listOf(destination,start))
        assertTrue(replacement is NativeRouteResult.Ready)
        main { engine.cancelRoute() }
    }
    @Test fun absentNeighborOrRegionNeverBecomesDrivableSuccess() {
        initialize()
        val result=route(listOf(RoutePoint(-25.4295,-49.2719,"Curitiba fixture"),RoutePoint(-27.5949,-48.5482,"Missing neighbor fixture")))
        assertTrue("missing data must refuse: $result",result is NativeRouteResult.Failed)
        val failed=result as NativeRouteResult.Failed
        assertTrue(failed.code in setOf(3,4,6,7,8,9,11))
        main {engine.cancelRoute()}
    }

    @Test fun sourceOneWayStreetRequiresReverseDetourAndCancelledGenerationCannotPublish() {
        initialize()
        // OSM way 125539246, Rua Barão do Serro Azul, oneway=yes in the
        // admitted source.o5m. Interior points avoid snapping at intersections.
        val a=RoutePoint(-25.4289774,-49.2712169,"One-way source A")
        val b=RoutePoint(-25.4286512,-49.2711350,"One-way source B")
        val forward=route(listOf(a,b)) as NativeRouteResult.Ready
        val reverse=route(listOf(b,a)) as NativeRouteResult.Ready
        assertTrue("one-way reverse must detour: ${forward.preview.distanceMeters}/${reverse.preview.distanceMeters}",reverse.preview.distanceMeters>forward.preview.distanceMeters+20)
        val stale=java.util.concurrent.atomic.AtomicInteger()
        val done=CountDownLatch(1)
        main {
            engine.buildRoute(NativeRouteRequest(listOf(a,RoutePoint(-27.5949,-48.5482,"Missing region")))){stale.incrementAndGet()}
            engine.cancelRoute()
            engine.buildRoute(NativeRouteRequest(listOf(a,b))){if(it is NativeRouteResult.Ready) done.countDown()}
        }
        assertTrue(done.await(60,TimeUnit.SECONDS));assertEquals(0,stale.get())
        main { engine.cancelRoute() }
    }

    @Test fun actualOfflineNativeSearchResolvesStreetCoordinates() {
        initialize()
        val found=AtomicReference<List<com.anpfuel.maps.engine.MapSearchResult>>();val done=CountDownLatch(1)
        val context=ApplicationProvider.getApplicationContext<Context>()
        main { engine.setViewport(-25.4295,-49.2719,15);assertTrue(engine.search(context,"Barão do Serro Azul"){found.set(it);done.countDown()}) }
        assertTrue(done.await(30,TimeUnit.SECONDS));assertTrue("street search",found.get().isNotEmpty())
        assertTrue(found.get().any { kotlin.math.abs(it.latitude+25.43)<0.05 && kotlin.math.abs(it.longitude+49.27)<0.05 })
        main {engine.cancelSearch()}
    }

    @Test fun nativeAndJavaDiagnosticsNeverEmitRouteOrExceptionPayloads() {
        initialize()
        Class.forName("app.organicmaps.sdk.util.log.Logger").getMethod("e",String::class.java,String::class.java,Throwable::class.java)
            .invoke(null,"PrivateSyntheticTest","MAP22_PRIVATE_SENTINEL",IllegalArgumentException("MAP22_PRIVATE_SENTINEL"))
        route(listOf(RoutePoint(-25.4295,-49.2719,"Synthetic privacy origin"),RoutePoint(-25.4222,-49.2640,"Synthetic privacy destination")))
        val descriptor=instrumentation.uiAutomation.executeShellCommand("logcat -d -v brief --pid="+android.os.Process.myPid())
        val text=android.os.ParcelFileDescriptor.AutoCloseInputStream(descriptor).bufferedReader().use {it.readText()}
        assertFalse("Java SDK payload leaked",text.contains("MAP22_PRIVATE_SENTINEL"))
        assertFalse("native route checkpoints leaked",text.contains("Checkpoints("))
        main {engine.cancelRoute()}
    }
}
