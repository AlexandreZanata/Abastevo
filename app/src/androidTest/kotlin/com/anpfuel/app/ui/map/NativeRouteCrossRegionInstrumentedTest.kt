package com.anpfuel.app.ui.map

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.domain.routing.*
import com.anpfuel.maps.engine.MapEngineHolder
import java.io.File
import java.security.MessageDigest
import java.util.Base64
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Separate fixture profile: complete official pinned neighboring regions.
 * Kept outside the APK/Git and never enabled as a product download origin.
 */
@RunWith(AndroidJUnit4::class)
class NativeRouteCrossRegionInstrumentedTest {
    private val instrumentation=InstrumentationRegistry.getInstrumentation()
    private val engine get()=MapEngineHolder.engine
    private fun main(action:()->Unit)=instrumentation.runOnMainSync(action)
    private fun route():NativeRouteResult {
        val done=CountDownLatch(1);val result=AtomicReference<NativeRouteResult>()
        main {assertTrue(engine.buildRoute(NativeRouteRequest(listOf(
            RoutePoint(-25.4295,-49.2719,"Curitiba synthetic route origin"),
            RoutePoint(-25.3905,-51.4654,"Guarapuava synthetic destination"),
        ))){result.set(it);done.countDown()})}
        assertTrue(done.await(90,TimeUnit.SECONDS));return result.get()
    }
    @Test fun completeNeighborRouteThenMissingDependencyRefusesOffline() {
        val context=ApplicationProvider.getApplicationContext<Context>()
        val root=File(context.filesDir,"261004")
        // Size and SHA-1 from countries.txt at source 4587b8ad (series 2026.06.28).
        listOf(Triple("Brazil_Parana_East.mwm",86360029L,"YvAmb3q4ziBfKgbWgj/K0a1qbwo="),
            Triple("Brazil_Parana_West.mwm",117513367L,"TIEi1UCmo6CDOOHIZMXkNF/1SYk=")).forEach { (name,size,expected) ->
            val file=File(root,name);assertEquals(name,size,file.length())
            val digest=MessageDigest.getInstance("SHA-1");file.inputStream().use { input -> val buffer=ByteArray(65536);while(true){val n=input.read(buffer);if(n<0)break;digest.update(buffer,0,n)} }
            assertEquals(name,expected,Base64.getEncoder().encodeToString(digest.digest()))
        }
        val ready=CountDownLatch(1);main {engine.initialize(context){ready.countDown()}}
        assertTrue(ready.await(45,TimeUnit.SECONDS));main {assertTrue(engine.reloadMaps())}
        val complete=route();assertTrue("complete offline cross-region route: $complete",complete is NativeRouteResult.Ready)
        val preview=(complete as NativeRouteResult.Ready).preview
        assertTrue(preview.distanceMeters>150_000);assertTrue(preview.geometry.any {it.longitude < -51.2});assertTrue(preview.geometry.any {it.longitude > -50.0})
        val west=File(root,"Brazil_Parana_West.mwm");val parked=File(context.cacheDir,"map22-west.suspended")
        main {engine.cancelRoute();assertTrue(engine.suspendMapReads())}
        assertTrue(west.renameTo(parked))
        try {
            main {assertTrue(engine.reloadMaps())}
            val missing=route();assertTrue("missing neighbor must refuse: $missing",missing is NativeRouteResult.Failed)
            val failed=missing as NativeRouteResult.Failed
            assertTrue("native dependency must name the missing map",failed.missingMaps.any {it.contains("Parana_West")})
        } finally {
            main {engine.cancelRoute();engine.suspendMapReads()}
            assertTrue(parked.renameTo(west));main {engine.reloadMaps()}
        }
    }
}
