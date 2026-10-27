package com.anpfuel.app.ui.driver.work

import androidx.activity.ComponentActivity
import android.os.SystemClock
import android.provider.Settings
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.lifecycle.viewModelScope
import androidx.room.Room
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.entity.DriverShiftEntity
import com.anpfuel.data.local.entity.DriverShiftGapEntity
import com.anpfuel.data.repository.DriverLocalStore
import com.anpfuel.domain.portable.PortableAuth
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Actual screen, ViewModel and Room, with only synthetic sessions and data. */
@RunWith(AndroidJUnit4::class)
class DriverWorkScreenRoomTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private val databaseName = "driver-work-screen-room-test"
    private lateinit var database: AnpFuelDatabase
    private lateinit var store: DriverLocalStore
    private val sessions = TestSessions()
    private val models = mutableListOf<DriverWorkViewModel>()
    private val vehicle = "synthetic-car"

    private class TestSessions : AuthSessionStore {
        @Volatile private var current: PortableAuth.Session? = session("owner-a")
        override fun save(session: PortableAuth.Session) { current = session }
        override fun load(): PortableAuth.Session? = current
        override fun clear() { current = null }
        companion object {
            fun session(owner: String) = PortableAuth.Session("family", owner, "synthetic", "synthetic", 0, 0)
        }
    }

    @Before fun setup() {
        context.deleteDatabase(databaseName)
        database = Room.databaseBuilder(context, AnpFuelDatabase::class.java, databaseName).build()
        store = DriverLocalStore(database.driverLocalDao())
    }

    @After fun cleanup() {
        models.forEach { it.viewModelScope.cancel() }
        database.close()
        context.deleteDatabase(databaseName)
    }

    private fun model(): DriverWorkViewModel = DriverWorkViewModel(store, sessions, context).also { models.add(it) }
    private fun render(vm: DriverWorkViewModel) {
        compose.setContent { AnpFuelTheme(dynamicColor = false) { DriverWorkScreen(vehicle, {}, viewModel = vm) } }
        compose.waitUntil(10_000) { !vm.uiState.value.loading }
    }
    private fun waitFor(vm: DriverWorkViewModel, condition: (DriverWorkState) -> Boolean) {
        compose.waitUntil(10_000) { condition(vm.uiState.value) }
    }
    private fun shiftCount(owner: String): Int = database.openHelper.readableDatabase.query(
        "SELECT count(*) FROM driver_shift WHERE owner_account = ?", arrayOf(owner),
    ).use { cursor -> cursor.moveToFirst(); cursor.getInt(0) }

    @Test fun duplicateStartPauseResumeAndTwoShiftsUseRealRoom() {
        val vm = model()
        render(vm)
        compose.runOnIdle { vm.start(vehicle, "100"); vm.start(vehicle, "100") }
        waitFor(vm) { it.shift?.state == "RUNNING" }
        compose.waitUntil(10_000) { shiftCount("owner-a") == 1 }
        Thread.sleep(1_000)
        assertEquals(1, shiftCount("owner-a"))
        val first = vm.uiState.value.shift!!
        compose.onNodeWithText("Pausar").performScrollTo().performClick()
        waitFor(vm) { it.shift?.state == "PAUSED" }
        val paused = vm.uiState.value.shift!!
        assertEquals(first.shiftId, paused.shiftId)
        assertTrue(paused.activeMillis >= 0)
        Thread.sleep(1_100)
        compose.onNodeWithText("Retomar").performScrollTo().performClick()
        waitFor(vm) { it.shift?.state == "RUNNING" && (it.shift?.version ?: 0) > paused.version }
        val resumed = vm.uiState.value.shift!!
        assertEquals(paused.activeMillis, resumed.activeMillis)
        compose.onNodeWithText("Hodômetro final (km)").performScrollTo().performTextInput("101")
        compose.onNodeWithText("Encerrar turno").performScrollTo().performClick()
        waitFor(vm) { it.shift == null && !it.loading }
        val closed = runBlocking { store.findShift("owner-a", "android", first.shiftId) }
        assertEquals("CLOSED", closed?.state)
        assertEquals(100_000L, closed?.startOdometerMetres)
        assertEquals(101_000L, closed?.endOdometerMetres)
        compose.onNodeWithText("Hodômetro inicial (km)").performScrollTo().performTextInput("101")
        compose.onNodeWithText("Iniciar turno").performScrollTo().performClick()
        waitFor(vm) { it.shift?.state == "RUNNING" }
        assertNotEquals(first.shiftId, vm.uiState.value.shift!!.shiftId)
        assertEquals(2, shiftCount("owner-a"))
    }

    @Test fun recreatedViewModelRequiresReviewAndPreservesPersistedKnownTime() {
        val original = model()
        render(original)
        compose.runOnIdle { original.start(vehicle, "100") }
        waitFor(original) { it.shift?.state == "RUNNING" }
        val before = original.uiState.value.shift!!
        original.viewModelScope.cancel()
        val reopened = model()
        compose.runOnIdle { reopened.load(vehicle) }
        waitFor(reopened) { it.shift?.state == "RECOVERY_REQUIRED" }
        assertEquals(before.shiftId, reopened.uiState.value.shift!!.shiftId)
        assertEquals(before.activeMillis, reopened.uiState.value.shift!!.activeMillis)
        assertEquals(1, reopened.uiState.value.gaps.size)
        assertFalse(reopened.uiState.value.gaps.single().reviewed)
        assertNull(reopened.uiState.value.gaps.single().reviewedWorkMillis)
        assertEquals("process-recovery", reopened.uiState.value.gaps.single().reason)
    }

    @Test fun explicitReloadForAnotherOwnerDoesNotShowOrChangeTheFirstOwnerShift() {
        val vm = model()
        render(vm)
        compose.runOnIdle { vm.start(vehicle, "100") }
        waitFor(vm) { it.shift?.state == "RUNNING" }
        val ownerAShift = vm.uiState.value.shift!!
        sessions.save(TestSessions.session("owner-b"))
        compose.runOnIdle { vm.load(vehicle) }
        waitFor(vm) { it.shift == null && !it.loading }
        assertNull(runBlocking { store.currentShiftChecked("owner-b", "android") })
        assertEquals(ownerAShift, runBlocking { store.findShift("owner-a", "android", ownerAShift.shiftId) })
        compose.runOnIdle { vm.start(vehicle, "200") }
        waitFor(vm) { it.shift?.ownerAccount == "owner-b" }
        assertNotNull(runBlocking { store.findShift("owner-b", "android", vm.uiState.value.shift!!.shiftId) })
        assertEquals(ownerAShift, runBlocking { store.findShift("owner-a", "android", ownerAShift.shiftId) })
    }

    @Test fun changingOwnerRecoversThatOwnersPersistedRunningShift() {
        val now = System.currentTimeMillis()
        val elapsed = SystemClock.elapsedRealtime()
        val boot = "boot-" + Settings.Global.getInt(context.contentResolver, Settings.Global.BOOT_COUNT, 0)
        val b = DriverShiftEntity(
            ownerAccount = "owner-b", origin = "android", shiftId = "persisted-b", vehicleId = vehicle,
            timezone = "America/Cuiaba", state = "RUNNING", version = 1,
            lastBootId = boot, lastElapsedMillis = elapsed, lastUtcMillis = now,
            activeMillis = 1234, manuallyReviewedMillis = 0, createdAtMillis = now, updatedAtMillis = now,
        )
        runBlocking { store.startShift(b) }
        val vm = model()
        render(vm)
        assertNull(vm.uiState.value.shift)
        sessions.save(TestSessions.session("owner-b"))
        compose.runOnIdle { vm.load(vehicle) }
        waitFor(vm) { it.shift?.state == "RECOVERY_REQUIRED" && it.shift?.ownerAccount == "owner-b" }
        assertEquals(1234L, vm.uiState.value.shift!!.activeMillis)
        assertEquals("process-recovery", vm.uiState.value.gaps.single().reason)
        assertNull(vm.uiState.value.gaps.single().reviewedWorkMillis)
        assertEquals("RECOVERY_REQUIRED", runBlocking { store.findShift("owner-b", "android", "persisted-b") }?.state)
    }

    @Test fun legacyReviewedFlagWithoutWorkAmountStillRequiresExplicitReview() {
        val now = System.currentTimeMillis()
        val elapsed = SystemClock.elapsedRealtime()
        val boot = "boot-" + Settings.Global.getInt(context.contentResolver, Settings.Global.BOOT_COUNT, 0)
        val initial = DriverShiftEntity(
            ownerAccount = "owner-a", origin = "android", shiftId = "legacy", vehicleId = vehicle,
            timezone = "America/Cuiaba", state = "RUNNING", version = 1,
            lastBootId = boot, lastElapsedMillis = elapsed, lastUtcMillis = now,
            activeMillis = 1234, manuallyReviewedMillis = 0, startOdometerMetres = 100_000,
            createdAtMillis = now, updatedAtMillis = now,
        )
        val gap = DriverShiftGapEntity(
            ownerAccount = "owner-a", origin = "android", shiftId = "legacy", gapId = "old-gap",
            reason = "process-recovery", previousBootId = boot, previousElapsedMillis = elapsed,
            previousUtcMillis = now, nextBootId = boot, nextElapsedMillis = elapsed,
            nextUtcMillis = now, reviewed = true, reviewedWorkMillis = null, createdAtMillis = now,
        )
        runBlocking {
            store.startShift(initial)
            store.updateShift(initial.copy(state = "RECOVERY_REQUIRED", version = 2), listOf(gap), 1)
        }
        val vm = model()
        render(vm)
        assertEquals(true, vm.uiState.value.gaps.single().reviewed)
        assertNull(vm.uiState.value.gaps.single().reviewedWorkMillis)
        compose.onNodeWithText("Lacuna pendente:", substring = true).performScrollTo().assertExists()
        compose.onNodeWithText("Hodômetro final (km)").performScrollTo().performTextInput("101")
        compose.onNodeWithText("Encerrar turno").performScrollTo().performClick()
        waitFor(vm) { it.message?.contains("Revise todas as lacunas") == true }
        assertEquals("RECOVERY_REQUIRED", runBlocking { store.findShift("owner-a", "android", "legacy") }?.state)
        compose.onNodeWithText("Minutos trabalhados confirmados").performScrollTo().performTextInput("0")
        compose.onNodeWithText("Confirmar revisão da lacuna").performScrollTo().performClick()
        waitFor(vm) { it.shift?.state == "PAUSED" && it.gaps.singleOrNull()?.reviewedWorkMillis == 0L }
        assertEquals(1234L, vm.uiState.value.shift!!.activeMillis)
        assertEquals(0L, runBlocking { store.listShiftGaps("owner-a", "android", "legacy").single().reviewedWorkMillis })
    }


    @Test fun checkpointUsesCurrentVehicleAfterSameViewModelChangesVehicle() {
        val vm = model()
        render(vm)
        compose.runOnIdle { vm.start(vehicle, "100") }
        waitFor(vm) { it.shift?.state == "RUNNING" }
        compose.runOnIdle { vm.end(vehicle, "101") }
        waitFor(vm) { it.shift == null && !it.loading }

        val now = System.currentTimeMillis()
        val elapsed = SystemClock.elapsedRealtime()
        val boot = "boot-" + Settings.Global.getInt(context.contentResolver, Settings.Global.BOOT_COUNT, 0)
        val otherVehicle = "synthetic-car-b"
        val other = DriverShiftEntity(
            ownerAccount = "owner-a", origin = "android", shiftId = "vehicle-b-shift", vehicleId = otherVehicle,
            timezone = "America/Cuiaba", state = "RUNNING", version = 1, lastBootId = boot,
            lastElapsedMillis = elapsed, lastUtcMillis = now, activeMillis = 0, manuallyReviewedMillis = 0,
            createdAtMillis = now, updatedAtMillis = now,
        )
        runBlocking {
            store.startShift(other)
            store.updateShift(other.copy(state = "PAUSED", version = 2), emptyList(), 1)
        }
        compose.runOnIdle { vm.load(otherVehicle) }
        waitFor(vm) { it.shift?.shiftId == "vehicle-b-shift" && it.shift?.state == "PAUSED" }
        compose.runOnIdle { vm.resume(otherVehicle) }
        waitFor(vm) { it.shift?.state == "RUNNING" && it.shift?.vehicleId == otherVehicle }
        val versionBefore = vm.uiState.value.shift!!.version
        compose.runOnIdle { vm.checkpointActiveShift() }
        waitFor(vm) { (it.shift?.version ?: 0) > versionBefore || it.shiftBlocked }
        assertFalse(vm.uiState.value.shiftBlocked)
        assertEquals(otherVehicle, vm.uiState.value.shift?.vehicleId)
        assertEquals("RUNNING", vm.uiState.value.shift?.state)
        assertTrue((vm.uiState.value.shift?.version ?: 0) > versionBefore)
    }

}
