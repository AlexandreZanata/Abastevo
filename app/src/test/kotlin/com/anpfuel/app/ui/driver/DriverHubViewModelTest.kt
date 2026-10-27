package com.anpfuel.app.ui.driver

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.premium.GetPremiumStatusUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusException
import com.anpfuel.domain.premium.PremiumStatusSnapshot
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class DriverHubViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val vehicles = mockk<ListVehiclesUseCase>()
    private val premium = mockk<GetPremiumStatusUseCase>()
    private val sessions = mockk<AuthSessionStore>()
    private val session = PortableAuth.Session("fam", "acct", "token", "refresh", Long.MAX_VALUE, Long.MAX_VALUE)
    private val active = PremiumStatus(
        policyVersion = 1, state = PremiumState.ACTIVE, reason = "verified",
        capabilities = setOf("professional.write", "professional.report"), validUntil = "2026-11-30T23:59:59Z",
        retainedDataAccess = true, paymentsConfigured = false, managementUrl = PremiumStatus.MANAGEMENT_URL,
    )
    private val expired = active.copy(state = PremiumState.EXPIRED, capabilities = emptySet())
    private val car = Vehicle.create(
        displayName = "Gol", tankCapacity = TankCapacity.of(50.0), fuelProduct = FuelProduct.GASOLINE_REGULAR,
        priceSource = VehiclePriceSource.cheapest(),
    )

    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun teardown() { Dispatchers.resetMain() }

    private fun model() = DriverHubViewModel(vehicles, premium, sessions)

    @Test fun `verified owner sees real vehicle actions`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { vehicles.invoke() } returns listOf(car)
        coEvery { premium.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(DriverHubAccess.VERIFIED, vm.uiState.value.access)
        assertEquals(1, vm.uiState.value.vehicles.size)
        assertFalse(vm.uiState.value.loading)
    }

    @Test fun `lapse keeps retained data state without local grant`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { vehicles.invoke() } returns listOf(car)
        coEvery { premium.invoke() } returns PremiumStatusSnapshot("acct", expired)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(DriverHubAccess.LAPSED, vm.uiState.value.access)
        assertEquals(1, vm.uiState.value.vehicles.size)
    }

    @Test fun `offline unavailable signed-out and vehicle failure are distinct`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { vehicles.invoke() } returns emptyList()
        coEvery { premium.invoke() } throws PremiumStatusException.Unavailable("down")
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(DriverHubAccess.OFFLINE, vm.uiState.value.access)

        coEvery { premium.invoke() } throws PremiumStatusException.Invalid("bad")
        vm.refresh()
        advanceUntilIdle()
        assertEquals(DriverHubAccess.UNAVAILABLE, vm.uiState.value.access)

        every { sessions.load() } returns null
        vm.refresh()
        advanceUntilIdle()
        assertEquals(DriverHubAccess.SIGNED_OUT, vm.uiState.value.access)

        every { sessions.load() } returns session
        coEvery { vehicles.invoke() } throws IllegalStateException("db")
        coEvery { premium.invoke() } returns PremiumStatusSnapshot("acct", active)
        vm.refresh()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.vehicleLoadFailed)
        assertTrue(vm.uiState.value.vehicles.isEmpty())
    }
}
