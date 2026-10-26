package com.anpfuel.app.ui.map

import android.Manifest
import android.content.Context
import android.location.Location
import android.location.LocationManager
import android.location.provider.ProviderProperties
import android.os.Build
import android.os.SystemClock
import androidx.core.content.ContextCompat
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.AnpFuelDatabase
import org.junit.After
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * MAP-25 explicit recorder on a real device/emulator: the real foreground service, real location
 * provider callbacks (mock GPS provider with explicit app-op), the real Room database. Synthetic
 * coordinates only. Permission state is set by the host script (revoked for the negative test).
 */
@RunWith(AndroidJUnit4::class)
class TripRecordingServiceDeviceTest {
    private lateinit var context: Context
    private lateinit var locations: LocationManager
    private val ui = InstrumentationRegistry.getInstrumentation().uiAutomation

    private fun shell(command: String) {
        android.os.ParcelFileDescriptor.AutoCloseInputStream(ui.executeShellCommand(command)).use { it.readBytes() }
    }

    @Before fun prepare() {
        context = ApplicationProvider.getApplicationContext()
        locations = context.getSystemService(LocationManager::class.java)
    }

    @After fun cleanup() {
        runCatching { context.startService(TripRecordingService.stopIntent(context, save = false)) }
        runCatching { locations.removeTestProvider(LocationManager.GPS_PROVIDER) }
        waitFor("service idle") { !TripRecordingServiceState.state.value.active }
        sql("DELETE FROM map_saved_trip")
    }

    private fun waitFor(what: String, seconds: Int = 20, condition: () -> Boolean) {
        for (i in 1..seconds * 4) {
            if (condition()) return
            Thread.sleep(250)
        }
        fail("timeout waiting for $what; state=${TripRecordingServiceState.state.value}")
    }

    private fun db() = Room.databaseBuilder(context, AnpFuelDatabase::class.java, "anp_fuel.db").build()

    private fun sql(statement: String) {
        val database = db()
        try { database.openHelper.writableDatabase.execSQL(statement) } finally { database.close() }
    }

    private fun rows(query: String): List<List<String?>> {
        val database = db()
        try {
            database.openHelper.readableDatabase.query(query).use { c ->
                val out = ArrayList<List<String?>>()
                while (c.moveToNext()) out.add((0 until c.columnCount).map { c.getString(it) })
                return out
            }
        } finally { database.close() }
    }

    private fun mockGps() {
        shell("appops set ${context.packageName} android:mock_location allow")
        runCatching { locations.removeTestProvider(LocationManager.GPS_PROVIDER) }
        if (Build.VERSION.SDK_INT >= 31) {
            locations.addTestProvider(LocationManager.GPS_PROVIDER, false, false, false, false, true, true, true,
                ProviderProperties.POWER_USAGE_LOW, ProviderProperties.ACCURACY_FINE)
        } else {
            @Suppress("DEPRECATION")
            locations.addTestProvider(LocationManager.GPS_PROVIDER, false, false, false, false, true, true, true, 1, 1)
        }
        locations.setTestProviderEnabled(LocationManager.GPS_PROVIDER, true)
    }

    private fun push(lat: Double, accuracy: Float = 5f) {
        val location = Location(LocationManager.GPS_PROVIDER).apply {
            latitude = lat; longitude = -49.27; this.accuracy = accuracy; altitude = 900.0
            time = System.currentTimeMillis(); elapsedRealtimeNanos = SystemClock.elapsedRealtimeNanos()
        }
        locations.setTestProviderLocation(LocationManager.GPS_PROVIDER, location)
    }

    private fun granted() = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == android.content.pm.PackageManager.PERMISSION_GRANTED

    @Test fun withoutPreciseLocationPermissionNothingStartsAndNoRecordingExists() {
        assumeTrue("host revokes ACCESS_FINE_LOCATION before this test", !granted())
        ContextCompat.startForegroundService(context, TripRecordingService.startIntent(context, "no permission"))
        waitFor("no-permission refusal") { TripRecordingServiceState.state.value.error == "no-permission" }
        assertFalse(TripRecordingServiceState.state.value.active)
        assertTrue("no recording row may exist", rows("SELECT trip_id FROM map_saved_trip").isEmpty())
    }

    @Test fun explicitRecordingKeepsOnlyGoodSpacedFixesAndSavesOnStop() {
        assumeTrue("host grants ACCESS_FINE_LOCATION", granted())
        shell("pm grant ${context.packageName} android.permission.POST_NOTIFICATIONS")
        mockGps()
        // Nothing records before the explicit start, even with fixes arriving.
        push(-25.0)
        Thread.sleep(500)
        assertTrue(rows("SELECT trip_id FROM map_saved_trip").isEmpty())
        assertFalse(TripRecordingServiceState.state.value.active)

        ContextCompat.startForegroundService(context, TripRecordingService.startIntent(context, "Passeio sintético"))
        waitFor("recording active") { TripRecordingServiceState.state.value.active }
        assertEquals(1, rows("SELECT state FROM map_saved_trip WHERE state = 'RECORDING'").size)

        // The recorder subscribes with a 2 s minimum interval, so each fix is spaced accordingly.
        val gap = 2_300L
        push(-25.0)                 // kept (first)
        waitFor("first point") { TripRecordingServiceState.state.value.points == 1 }
        Thread.sleep(gap); push(-25.00001)             // 1.1 m: spaced out
        Thread.sleep(gap); push(-25.0001, accuracy = 80f) // poor accuracy
        Thread.sleep(gap)
        assertEquals(1, TripRecordingServiceState.state.value.points)
        push(-25.0002)              // 22 m from the first kept point: kept
        waitFor("second point") { TripRecordingServiceState.state.value.points == 2 }
        Thread.sleep(gap); push(-25.0004)              // 22 m more: kept
        waitFor("third point") { TripRecordingServiceState.state.value.points == 3 }
        assertEquals("3", rows("SELECT point_count FROM map_saved_trip WHERE state = 'RECORDING'").single().single())

        // The notification action path: stop and save.
        context.startService(TripRecordingService.stopIntent(context, save = true))
        waitFor("service stopped") { !TripRecordingServiceState.state.value.active }
        waitFor("trip completed") { rows("SELECT state FROM map_saved_trip WHERE state = 'COMPLETE'").size == 1 }
        val trip = rows("SELECT kind, name, point_count FROM map_saved_trip").single()
        assertEquals(listOf("RECORDED_TRACK", "Passeio sintético", "3"), trip)
        assertEquals(3, rows("SELECT seq FROM map_saved_trip_point").size)
        // After stop, further fixes are ignored: GPS is released and nothing records.
        push(-25.0010)
        Thread.sleep(800)
        assertEquals(3, rows("SELECT seq FROM map_saved_trip_point").size)
        assertFalse(isServiceRunning())
    }

    @Suppress("DEPRECATION")
    private fun isServiceRunning(): Boolean {
        val manager = context.getSystemService(android.app.ActivityManager::class.java)
        return manager.getRunningServices(100).any { it.service.className == TripRecordingService::class.java.name }
    }

    @Test fun discardRemovesTheRecordingAndSavesNothing() {
        assumeTrue("host grants ACCESS_FINE_LOCATION", granted())
        shell("pm grant ${context.packageName} android.permission.POST_NOTIFICATIONS")
        mockGps()
        ContextCompat.startForegroundService(context, TripRecordingService.startIntent(context, "Descartar"))
        waitFor("recording active") { TripRecordingServiceState.state.value.active }
        push(-25.0)
        waitFor("first point") { TripRecordingServiceState.state.value.points == 1 }
        context.startService(TripRecordingService.stopIntent(context, save = false))
        waitFor("service stopped") { !TripRecordingServiceState.state.value.active }
        waitFor("rows gone") { rows("SELECT trip_id FROM map_saved_trip").isEmpty() }
        assertTrue(rows("SELECT seq FROM map_saved_trip_point").isEmpty())
    }
}
