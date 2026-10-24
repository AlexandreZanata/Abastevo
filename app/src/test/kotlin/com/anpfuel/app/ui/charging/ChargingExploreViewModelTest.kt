package com.anpfuel.app.ui.charging

import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.vehicle.GetVehicleUseCase
import com.anpfuel.data.repository.ChargingDiscoveryRepository
import com.anpfuel.data.repository.DiscoveryRow
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.Compatibility
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.BatteryCapacity
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.Propulsion
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import java.time.Instant
import java.util.Locale
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ChargingExploreViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val repository = mockk<ChargingDiscoveryRepository>()
    private val getVehicle = mockk<GetVehicleUseCase>()
    private val connectivity = mockk<ObserveNetworkConnectivityUseCase>()

    private lateinit var viewModel: ChargingExploreViewModel

    private val vehicleId = DomainId.generate()
    private val placeId = "11111111-1111-4111-8111-111111111111"

    private fun row(id: String = placeId, verdict: Compatibility = Compatibility.COMPATIBLE) = DiscoveryRow(
        place = ChargingPlace(
            id = id, entityKey = "a".repeat(64), externalId = "OCM-1", name = "Eletroposto",
            country = "BR", latitude = null, longitude = null, publicAccess = ChargingAccess.PUBLIC,
            reportedPoints = 1, groups = emptyList(),
        ),
        verdict = verdict, profile = "ocm-br-20261021-v1", sourceRevision = "rev-1",
        generation = 4, savedAtMillis = 1,
    )

    private fun bev() = Vehicle.create(
        displayName = "Dolphin", tankCapacity = null, fuelProduct = null,
        priceSource = VehiclePriceSource.cheapest(), id = vehicleId, propulsion = Propulsion.BEV,
        batteryCapacity = BatteryCapacity.ofWh(38000), chargeConnector = ChargingConnector.CCS2,
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { getVehicle.invoke(vehicleId) } returns bev()
        every { connectivity.invoke() } returns flowOf(true)
        coEvery { repository.generationMeta(any()) } returns null
        coEvery { repository.refresh(any(), any(), any(), any(), any()) } returns com.anpfuel.data.local.charging.StoredPackMeta(
            key = "pack:ocm-br-20261021-v1", generation = 4, profile = "ocm-br-20261021-v1",
            sourceRevision = "rev-1", origin = "https://api.example.test", license = "L",
            schemaVersion = 1, baseGeneration = 0, placeCount = 0, groupCount = 0,
            cursor = null, state = "COMPLETE", savedAtMillis = 1,
        )
        coEvery { repository.list(any(), any(), any(), any(), any(), any(), any()) } returns emptyList()
        viewModel = ChargingExploreViewModel(repository, getVehicle, connectivity)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test fun `first open downloads then pages locally without network`() = runTest(dispatcher) {
        coEvery { repository.list(any(), 4L, any(), any(), any(), 20, 0) } returns listOf(row())
        coEvery { repository.list(any(), 4L, any(), any(), any(), 20, 20) } returns emptyList()
        viewModel.load(vehicleId)
        advanceUntilIdle()
        coVerify(exactly = 1) { repository.refresh("ocm-br-20261021-v1", any(), any(), any(), any()) }
        assertEquals(1, viewModel.uiState.value.rows.size)
        assertEquals(ChargingConnector.CCS2, viewModel.uiState.value.connector)
        viewModel.loadMore()
        advanceUntilIdle()
        assertTrue(viewModel.uiState.value.rows.isNotEmpty())
    }

    @Test fun `filter change resets the window and offline keeps rows`() = runTest(dispatcher) {
        coEvery { repository.list(any(), 4L, any(), any(), any(), 20, 0) } returns listOf(row())
        viewModel.load(vehicleId)
        advanceUntilIdle()
        viewModel.onCurrentChanged(ChargingCurrent.DC)
        advanceUntilIdle()
        coVerify(exactly = 1) { repository.list(any(), 4L, any(), ChargingCurrent.DC, any(), 20, 0) }
        coEvery { repository.refresh(any(), any(), any(), any(), any()) } throws java.io.IOException("offline")
        viewModel.refresh()
        advanceUntilIdle()
        assertTrue(viewModel.uiState.value.isOffline)
        assertEquals(1, viewModel.uiState.value.rows.size)
    }

    @Test fun `snapshot age is dated and never presented as live`() = runTest(dispatcher) {
        val stale = com.anpfuel.data.local.charging.StoredPackMeta(
            key = "pack:ocm-br-20261021-v1", generation = 4, profile = "ocm-br-20261021-v1",
            sourceRevision = "rev-1", origin = "https://api.example.test", license = "L",
            schemaVersion = 1, baseGeneration = 0, placeCount = 0, groupCount = 0,
            cursor = null, state = "COMPLETE", savedAtMillis = 1L,
        )
        coEvery { repository.generationMeta(any()) } returns stale
        coEvery { repository.list(any(), 4L, any(), any(), any(), 20, 0) } returns emptyList()
        viewModel.load(vehicleId)
        advanceUntilIdle()
        assertEquals(com.anpfuel.domain.charging.ChargingSnapshotAge.STALE, viewModel.uiState.value.sourceAge)
    }

    @Test fun `repeated refresh triggers converge on one active request`() = runTest(dispatcher) {
        val meta = com.anpfuel.data.local.charging.StoredPackMeta(
            key = "pack:ocm-br-20261021-v1", generation = 4, profile = "ocm-br-20261021-v1",
            sourceRevision = "rev-1", origin = "https://api.example.test", license = "L",
            schemaVersion = 1, baseGeneration = 0, placeCount = 0, groupCount = 0,
            cursor = null, state = "COMPLETE", savedAtMillis = 1,
        )
        coEvery { repository.generationMeta(any()) } returns meta
        coEvery { repository.list(any(), 4L, any(), any(), any(), 20, 0) } returns emptyList()
        val gate = CompletableDeferred<Unit>()
        coEvery { repository.refresh(any(), any(), any(), any(), any()) } coAnswers {
            gate.await()
            meta
        }
        viewModel.load(vehicleId)
        advanceUntilIdle()
        viewModel.refresh()
        runCurrent()
        viewModel.refresh()
        runCurrent()
        coVerify(exactly = 1) { repository.refresh("ocm-br-20261021-v1", any(), any(), any(), any()) }
        gate.complete(Unit)
        advanceUntilIdle()
        assertEquals(false, viewModel.uiState.value.isRefreshing)
    }

    @Test fun `detail opens offline and unknown verdicts never claim availability`() = runTest(dispatcher) {
        coEvery { repository.list(any(), 4L, any(), any(), any(), 20, 0) } returns listOf(row())
        viewModel.load(vehicleId)
        advanceUntilIdle()
        coEvery { repository.detail(placeId, "ocm-br-20261021-v1", 4L) } returns com.anpfuel.data.repository.PlaceDetail(
            row().place, "ocm-br-20261021-v1", "rev-1", 4L, 1L,
        )
        viewModel.openDetail(placeId)
        advanceUntilIdle()
        assertEquals(placeId, viewModel.uiState.value.detail?.place?.id)
        assertTrue(viewModel.uiState.value.rows.all { it.generation == 4L })
        viewModel.dismissDetail()
        assertEquals(null, viewModel.uiState.value.detail)
    }

}
