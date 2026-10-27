package com.anpfuel.app.ui.driver

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.location.LocationManager
import androidx.core.content.ContextCompat
import androidx.lifecycle.viewModelScope
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.debug.InstrumentedDbEntryPoint
import com.anpfuel.app.ui.driver.work.DriverGpsService
import com.anpfuel.app.ui.driver.work.DriverWorkViewModel
import com.anpfuel.data.local.entity.DriverShiftEntity
import dagger.hilt.android.EntryPointAccessors
import java.util.UUID
import kotlinx.coroutines.delay
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Run only on an isolated emulator/test installation with GPS enabled. */
@RunWith(AndroidJUnit4::class)
class DriverGpsServiceTest {
    @Test fun pauseCloseAndRecoveryStopRealGpsWithoutRestartingIt() = runBlocking {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val context = instrumentation.targetContext
        val dependencies = EntryPointAccessors.fromApplication(context, InstrumentedDbEntryPoint::class.java)
        val store = dependencies.driverStore()
        val database = dependencies.database()
        val sessions = dependencies.sessions()
        val owner = sessions.load()?.accountId ?: "local"
        val vehicle = "gps-test-vehicle-${UUID.randomUUID()}"
        val hadPermission = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
        val shifts = mutableListOf<String>()
        val models = mutableListOf<DriverWorkViewModel>()
        assertTrue(context.getSystemService(LocationManager::class.java).isProviderEnabled(LocationManager.GPS_PROVIDER))
        assertNull("isolated test account must have no active shift", store.currentShiftChecked(owner, "android"))

        fun onMain(action: () -> Unit) = instrumentation.runOnMainSync { action() }
        suspend fun awaitState(model: DriverWorkViewModel, predicate: (com.anpfuel.app.ui.driver.work.DriverWorkState) -> Boolean) = withTimeout(15_000) {
            while (!predicate(model.uiState.value)) delay(20)
        }
        suspend fun awaitObservation(shiftId: String, previousId: String? = null, final: Boolean = false) = withTimeout(15_000) {
            while (true) {
                val row = store.latestGpsObservation(owner, "android", shiftId)
                if (row != null && row.observationId != previousId && (!final || row.toAtMillis > row.fromAtMillis)) return@withTimeout row
                delay(20)
            }
            error("unreachable")
        }
        suspend fun ensureTimeAdvances(from: Long) = withTimeout(2_000) { while (System.currentTimeMillis() <= from) delay(1) }
        try {
            if (!hadPermission) instrumentation.uiAutomation.grantRuntimePermission(context.packageName, Manifest.permission.ACCESS_FINE_LOCATION)
            val vm = DriverWorkViewModel(store, sessions, context).also { models.add(it) }
            onMain { vm.load(vehicle) }
            awaitState(vm) { !it.loading && !it.shiftBlocked }
            onMain { vm.start(vehicle, "100") }
            awaitState(vm) { it.shift?.state == "RUNNING" }
            val firstId = vm.uiState.value.shift!!.shiftId.also { shifts.add(it) }
            onMain { vm.startGps() }
            val first = awaitObservation(firstId)
            ensureTimeAdvances(first.fromAtMillis)
            onMain { vm.pause(vehicle) }
            awaitState(vm) { it.shift?.state == "PAUSED" }
            val stoppedAtPause = awaitObservation(firstId, final = true)
            assertEquals(first.observationId, stoppedAtPause.observationId)
            assertFalse(stoppedAtPause.complete)

            onMain { vm.resume(vehicle) }
            awaitState(vm) { it.shift?.state == "RUNNING" }
            onMain { vm.startGps() }
            val second = awaitObservation(firstId, first.observationId)
            ensureTimeAdvances(second.fromAtMillis)
            onMain { vm.end(vehicle, "101") }
            awaitState(vm) { !it.loading && it.shift == null }
            val stoppedAtClose = awaitObservation(firstId, first.observationId, final = true)
            assertEquals(second.observationId, stoppedAtClose.observationId)
            assertTrue(stoppedAtClose.hasGap)

            onMain { vm.start(vehicle, "101") }
            awaitState(vm) { it.shift?.state == "RUNNING" }
            val secondId = vm.uiState.value.shift!!.shiftId.also { shifts.add(it) }
            onMain { vm.startGps() }
            val third = awaitObservation(secondId)
            ensureTimeAdvances(third.fromAtMillis)
            onMain { vm.viewModelScope.cancel() }
            val recovered = DriverWorkViewModel(store, sessions, context).also { models.add(it) }
            onMain { recovered.load(vehicle) }
            awaitState(recovered) { it.shift?.state == "RECOVERY_REQUIRED" }
            val stoppedAtRecovery = awaitObservation(secondId, final = true)
            assertEquals(third.observationId, stoppedAtRecovery.observationId)
            assertTrue(stoppedAtRecovery.hasGap)
        } finally {
            onMain { models.forEach { it.viewModelScope.cancel() } }
            context.stopService(android.content.Intent(context, DriverGpsService::class.java))
            for (shift in shifts) {
                database.openHelper.writableDatabase.execSQL("DELETE FROM driver_gps_observation WHERE owner_account = ? AND origin = ? AND shift_id = ?", arrayOf(owner, "android", shift))
                database.openHelper.writableDatabase.execSQL("DELETE FROM driver_shift_gap WHERE owner_account = ? AND origin = ? AND shift_id = ?", arrayOf(owner, "android", shift))
                database.openHelper.writableDatabase.execSQL("DELETE FROM driver_shift WHERE owner_account = ? AND origin = ? AND shift_id = ?", arrayOf(owner, "android", shift))
            }
            if (!hadPermission) instrumentation.uiAutomation.revokeRuntimePermission(context.packageName, Manifest.permission.ACCESS_FINE_LOCATION)
        }
    }

    @Test fun explicitStopPersistsIncompleteOwnerScopedObservation() = runBlocking {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val context = instrumentation.targetContext
        val dependencies = EntryPointAccessors.fromApplication(context, InstrumentedDbEntryPoint::class.java)
        val store = dependencies.driverStore()
        val database = dependencies.database()
        val owner = dependencies.sessions().load()?.accountId ?: "local"
        val id = "gps-test-${UUID.randomUUID()}"
        val hadPermission = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
        assertTrue("GPS provider must be enabled on the isolated emulator", context.getSystemService(LocationManager::class.java).isProviderEnabled(LocationManager.GPS_PROVIDER))
        assertNull("isolated test account must have no active shift", store.currentShiftChecked(owner, "android"))
        try {
            if (!hadPermission) instrumentation.uiAutomation.grantRuntimePermission(context.packageName, Manifest.permission.ACCESS_FINE_LOCATION)
            val now = System.currentTimeMillis()
            store.startShift(DriverShiftEntity(
                owner, "android", id, "test-vehicle", "UTC", "RUNNING", 1,
                "test-boot", 1, now, 0, 0, null, null, now, now,
            ))
            DriverGpsService.start(context, id)
            val initial = withTimeout(15_000) {
                while (true) {
                    val row = store.latestGpsObservation(owner, "android", id)
                    if (row != null) return@withTimeout row
                    delay(20)
                }
                error("unreachable")
            }
            assertFalse(initial.complete)
            assertTrue(initial.hasGap)
            withTimeout(2_000) { while (System.currentTimeMillis() <= initial.fromAtMillis) delay(1) }
            DriverGpsService.stop(context)
            val final = withTimeout(15_000) {
                while (true) {
                    val row = store.latestGpsObservation(owner, "android", id)
                    if (row != null && row.toAtMillis > row.fromAtMillis) return@withTimeout row
                    delay(20)
                }
                error("unreachable")
            }
            assertEquals(initial.observationId, final.observationId)
            assertEquals(owner, final.ownerAccount)
            assertFalse(final.complete)
            assertTrue(final.hasGap)
        } finally {
            context.stopService(android.content.Intent(context, DriverGpsService::class.java))
            database.openHelper.writableDatabase.execSQL("DELETE FROM driver_gps_observation WHERE owner_account = ? AND origin = ? AND shift_id = ?", arrayOf(owner, "android", id))
            database.openHelper.writableDatabase.execSQL("DELETE FROM driver_shift WHERE owner_account = ? AND origin = ? AND shift_id = ?", arrayOf(owner, "android", id))
            if (!hadPermission) instrumentation.uiAutomation.revokeRuntimePermission(context.packageName, Manifest.permission.ACCESS_FINE_LOCATION)
        }
    }
}
