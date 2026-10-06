package com.anpfuel.app.capture

import android.content.Context
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.port.CaptureOcrFlagProvider
import com.anpfuel.application.port.OcrPort
import com.anpfuel.application.portable.PhotoFlow
import com.anpfuel.application.usecase.capture.ConfirmPriceCaptureUseCase
import com.anpfuel.application.usecase.contribution.EnqueueContributionUseCase
import com.anpfuel.application.usecase.directory.GetNearbyServerStationsUseCase
import com.anpfuel.application.usecase.directory.NearbyServerStationsOutcome
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.portable.PortablePriceOcr
import com.anpfuel.domain.valueobject.FuelProduct
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
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/**
 * P10-T04: ViewModel gates on flag/permission/cancel and never
 * auto-confirms or uploads.
 */
class CaptureOcrViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val enqueue = mockk<EnqueueContributionUseCase>()

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private fun viewModel(
        enabled: Boolean,
        hasPermission: Boolean,
        ocrText: (String) -> List<PortablePriceOcr.OcrCandidate> =
            PortablePriceOcr::parseCandidates,
        location: com.anpfuel.domain.valueobject.DeviceLocation? = null,
        nearby: NearbyServerStationsOutcome =
            NearbyServerStationsOutcome.Fresh(emptyList()),
        photo: PhotoFlow.PhotoResult = PhotoFlow.PhotoResult.Ready("photo-1", 120_000, 1),
    ): CaptureOcrViewModel {
        val flags = object : CaptureOcrFlagProvider {
            override fun isEnabled(): Boolean = enabled
        }
        val ocr = object : OcrPort {
            override fun candidatesFromText(text: String) = ocrText(text)
        }
        val useCase = ConfirmPriceCaptureUseCase(flags, ocr)
        val context = mockk<Context>(relaxed = true)
        val permissions = CameraPermissionHandler(context)
        val handler = mockk<CameraPermissionHandler>()
        every { handler.hasCameraPermission() } returns hasPermission
        val locations = mockk<LocationPermissionHandler>()
        every { locations.getLastKnownLocation() } returns location
        val nearbyUseCase = mockk<GetNearbyServerStationsUseCase>()
        io.mockk.coEvery { nearbyUseCase.invoke(any(), any(), any(), any()) } returns nearby
        val photos = mockk<PhotoFlow>()
        every { photos.prepare(any(), any()) } returns photo
        return CaptureOcrViewModel(useCase, handler, flags, enqueue, locations, nearbyUseCase, photos)
    }

    @Test
    fun `denied permission surfaces PermissionDenied`() {
        val vm = viewModel(enabled = true, hasPermission = false)
        vm.onCaptureResult(cancelled = false, ocrText = "R$ 5,89")
        assertTrue(vm.state.value is CaptureOcrUiState.PermissionDenied)
    }

    @Test
    fun `cancelled capture surfaces Cancelled`() {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.onCaptureResult(cancelled = true, ocrText = "R$ 5,89")
        assertTrue(vm.state.value is CaptureOcrUiState.Cancelled)
    }

    @Test
    fun `multi-price capture needs human choice and confirms explicitly`() {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.onCaptureResult(cancelled = false, ocrText = "R$ 6,19\nR$ 5,89")
        val pending = vm.state.value
        assertTrue(pending is CaptureOcrUiState.NeedsConfirmation)
        assertTrue((pending as CaptureOcrUiState.NeedsConfirmation).candidates.size == 2)

        vm.onConfirm(pending.candidates.first(), null, "STANDARD", true)
        assertTrue(vm.state.value is CaptureOcrUiState.NeedsConfirmation)

        vm.onConfirm(pending.candidates[1], FuelProduct.GASOLINE_REGULAR, null, true)
        assertTrue(vm.state.value is CaptureOcrUiState.NeedsConfirmation)

        vm.onConfirm(pending.candidates[1], FuelProduct.GASOLINE_REGULAR, "STANDARD", true)
        val done = vm.state.value
        assertTrue(done is CaptureOcrUiState.Confirmed)
        assertTrue((done as CaptureOcrUiState.Confirmed).candidate.priceMilli == 5890L)
        assertTrue(done.conditionKind == "STANDARD")
    }

    @Test
    fun `manual price confirms and invalid input keeps candidates`() {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.onCaptureResult(cancelled = false, ocrText = "5,89")
        val pending = vm.state.value
        assertTrue(pending is CaptureOcrUiState.NeedsConfirmation)

        vm.onConfirmManual("nove", FuelProduct.ETHANOL, "APP", true)
        val kept = vm.state.value
        assertTrue(kept is CaptureOcrUiState.NeedsConfirmation)
        assertTrue((kept as CaptureOcrUiState.NeedsConfirmation).candidates.size == 1)

        vm.onConfirmManual("5,89", FuelProduct.ETHANOL, "APP", true)
        val done = vm.state.value
        assertTrue(done is CaptureOcrUiState.Confirmed)
        val confirmed = done as CaptureOcrUiState.Confirmed
        assertTrue(confirmed.candidate.priceMilli == 5890L)
        assertTrue(confirmed.candidate.manualEntry)
        assertTrue(confirmed.product == FuelProduct.ETHANOL)
        assertTrue(confirmed.conditionKind == "APP")
    }

    @Test
    fun `valid target binds and legacy cnpj is invalid`() {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.bindTarget("d6c74c23-63db-4c24-a2e5-408cb23bad26", "GASOLINE_REGULAR")
        assertTrue(vm.target.value?.stationId == "d6c74c23-63db-4c24-a2e5-408cb23bad26")
        assertTrue(!vm.targetInvalid.value)

        vm.bindTarget("04218406000104", "GASOLINE_REGULAR")
        assertTrue(vm.target.value == null)
        assertTrue(vm.targetInvalid.value)

        vm.bindTarget(null, null)
        assertTrue(vm.target.value == null)
        assertTrue(!vm.targetInvalid.value)
    }

    @OptIn(ExperimentalCoroutinesApi::class)
    @Test
    fun `submit without target or confirm stays NoTarget`() = runTest(dispatcher) {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.submitConfirmed()
        assertTrue(vm.submit.value is CaptureOcrViewModel.SubmitState.NoTarget)

        vm.onCaptureResult(cancelled = false, ocrText = "R$ 5,89")
        vm.submitConfirmed()
        assertTrue(vm.submit.value is CaptureOcrViewModel.SubmitState.NoTarget)
    }

    @OptIn(ExperimentalCoroutinesApi::class)
    @Test
    fun `submit enqueues matching target and rejects fuel mismatch`() = runTest(dispatcher) {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.onCaptureResult(cancelled = false, ocrText = "R$ 5,89")
        val pending = vm.state.value as CaptureOcrUiState.NeedsConfirmation
        vm.onConfirm(pending.candidates.first(), FuelProduct.GASOLINE_REGULAR, "STANDARD", true)
        assertTrue(vm.state.value is CaptureOcrUiState.Confirmed)

        vm.bindTarget("d6c74c23-63db-4c24-a2e5-408cb23bad26", "ETHANOL")
        vm.submitConfirmed()
        assertTrue(vm.submit.value is CaptureOcrViewModel.SubmitState.FuelMismatch)

        io.mockk.coEvery { enqueue.invoke(any()) } returns
            com.anpfuel.application.usecase.contribution.EnqueueContributionOutcome.Queued(
                command = mockk(relaxed = true),
                historical = false,
            )
        vm.bindTarget("d6c74c23-63db-4c24-a2e5-408cb23bad26", "GASOLINE_REGULAR")
        vm.submitConfirmed()
        advanceUntilIdle()
        val submitted = vm.submit.value
        assertTrue(submitted is CaptureOcrViewModel.SubmitState.Queued)
        assertTrue(!(submitted as CaptureOcrViewModel.SubmitState.Queued).historical)
    }

    private fun nearbyStation(id: String, name: String, meters: Double) =
        NearbyServerStation(
            station = ServerStation.create(
                stationId = id,
                displayName = name,
                locationQuality = StationLocationQuality.UNKNOWN,
                latitude = null,
                longitude = null,
                cnpjNormalized = null,
                municipalityCode = null,
                state = null,
                currentRevisionId = null,
            ),
            distanceMeters = meters,
        )

    @OptIn(ExperimentalCoroutinesApi::class)
    @Test
    fun `entry with location loads nearby stations for picking`() = runTest(dispatcher) {
        val rows = listOf(
            nearbyStation("d6c74c23-63db-4c24-a2e5-408cb23bad26", "Posto A", 120.0),
            nearbyStation("e7d85d34-74ec-5d35-b3f6-519dc44ce370", "Posto B", 340.0),
        )
        val vm = viewModel(
            enabled = true,
            hasPermission = true,
            location = com.anpfuel.domain.valueobject.DeviceLocation.of(-12.55, -55.72),
            nearby = NearbyServerStationsOutcome.Fresh(rows),
        )
        vm.onEntryPermissions(cameraGranted = true, locationGranted = true)
        advanceUntilIdle()
        org.junit.jupiter.api.Assertions.assertEquals(2, vm.nearby.value.size)
        vm.pickStation("d6c74c23-63db-4c24-a2e5-408cb23bad26")
        org.junit.jupiter.api.Assertions.assertEquals(
            "d6c74c23-63db-4c24-a2e5-408cb23bad26", vm.pickedStationId.value)
    }

    @OptIn(ExperimentalCoroutinesApi::class)
    @Test
    fun `entry without location marks picker denied`() = runTest(dispatcher) {
        val vm = viewModel(enabled = true, hasPermission = true, location = null)
        vm.onEntryPermissions(cameraGranted = true, locationGranted = false)
        advanceUntilIdle()
        assertTrue(vm.locationDenied.value)
        assertTrue(vm.nearby.value.isEmpty())
    }

    @Test
    fun `entry without camera stays PermissionDenied`() {
        val vm = viewModel(enabled = true, hasPermission = false)
        vm.onEntryPermissions(cameraGranted = false, locationGranted = true)
        assertTrue(vm.state.value is CaptureOcrUiState.PermissionDenied)
    }

    @Test
    fun `ready photo attaches id and refused photo keeps code with review`() {
        val ready = viewModel(enabled = true, hasPermission = true)
        ready.preparePhoto(byteArrayOf(1, 2, 3), "image/jpeg")
        org.junit.jupiter.api.Assertions.assertEquals("photo-1", ready.photoId.value)
        assertTrue(ready.state.value is CaptureOcrUiState.NeedsConfirmation)

        val refused = viewModel(
            enabled = true,
            hasPermission = true,
            photo = PhotoFlow.PhotoResult.Refused("OVER_BUDGET"),
        )
        refused.preparePhoto(byteArrayOf(1, 2, 3), "image/jpeg")
        org.junit.jupiter.api.Assertions.assertEquals("OVER_BUDGET", refused.photoRefused.value)
        assertTrue(refused.photoId.value == null)
        assertTrue(refused.state.value is CaptureOcrUiState.NeedsConfirmation)
    }

    @OptIn(ExperimentalCoroutinesApi::class)
    @Test
    fun `picked nearby station plus photo submit to outbox`() = runTest(dispatcher) {
        val vm = viewModel(
            enabled = true,
            hasPermission = true,
            location = com.anpfuel.domain.valueobject.DeviceLocation.of(-12.55, -55.72),
            nearby = NearbyServerStationsOutcome.Fresh(
                listOf(nearbyStation("d6c74c23-63db-4c24-a2e5-408cb23bad26", "Posto A", 120.0))),
        )
        vm.onEntryPermissions(cameraGranted = true, locationGranted = true)
        advanceUntilIdle()
        vm.pickStation("d6c74c23-63db-4c24-a2e5-408cb23bad26")
        vm.onConfirmManual("5,89", FuelProduct.GASOLINE_REGULAR, "STANDARD", true)
        assertTrue(vm.state.value is CaptureOcrUiState.Confirmed)
        val slot = io.mockk.slot<EnqueueContributionUseCase.Request>()
        io.mockk.coEvery { enqueue.invoke(capture(slot)) } returns
            com.anpfuel.application.usecase.contribution.EnqueueContributionOutcome.Queued(
                command = mockk(relaxed = true),
                historical = false,
            )
        vm.submitConfirmed()
        advanceUntilIdle()
        val submitted = vm.submit.value
        assertTrue(submitted is CaptureOcrViewModel.SubmitState.Queued)
        org.junit.jupiter.api.Assertions.assertEquals(
            "d6c74c23-63db-4c24-a2e5-408cb23bad26", slot.captured.stationId)
        org.junit.jupiter.api.Assertions.assertEquals(
            FuelProduct.GASOLINE_REGULAR, slot.captured.fuelProduct)
    }

    @Test
    fun `geofence admits only the minimum station area`() {
        org.junit.jupiter.api.Assertions.assertTrue(
            CaptureOcrViewModel.isInsideStationArea(0.0))
        org.junit.jupiter.api.Assertions.assertTrue(
            CaptureOcrViewModel.isInsideStationArea(
                CaptureOcrViewModel.MIN_STATION_AREA_METERS))
        org.junit.jupiter.api.Assertions.assertFalse(
            CaptureOcrViewModel.isInsideStationArea(
                CaptureOcrViewModel.MIN_STATION_AREA_METERS + 0.5))
        org.junit.jupiter.api.Assertions.assertFalse(
            CaptureOcrViewModel.isInsideStationArea(null))
    }

    @Test
    fun `fuel rows remove restore and blank means unsent`() {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR, "6,59")
        vm.setFuelAmount(FuelProduct.ETHANOL, "")
        org.junit.jupiter.api.Assertions.assertEquals(
            "6,59", vm.fuelAmounts.value[FuelProduct.GASOLINE_REGULAR])
        vm.removeFuel(FuelProduct.ETHANOL)
        org.junit.jupiter.api.Assertions.assertTrue(
            FuelProduct.ETHANOL in vm.removedFuels.value)
        vm.restoreFuels()
        org.junit.jupiter.api.Assertions.assertTrue(vm.removedFuels.value.isEmpty())
    }

    @OptIn(ExperimentalCoroutinesApi::class)
    @Test
    fun `submit sends one contribution per filled fuel`() = runTest(dispatcher) {
        val vm = viewModel(
            enabled = true,
            hasPermission = true,
            location = com.anpfuel.domain.valueobject.DeviceLocation.of(-12.55, -55.72),
            nearby = NearbyServerStationsOutcome.Fresh(
                listOf(nearbyStation("d6c74c23-63db-4c24-a2e5-408cb23bad26", "Posto A", 120.0))),
        )
        vm.onEntryPermissions(cameraGranted = true, locationGranted = true)
        advanceUntilIdle()
        vm.pickStation("d6c74c23-63db-4c24-a2e5-408cb23bad26")
        vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR, "6,59")
        vm.setFuelAmount(FuelProduct.ETHANOL, "4,32")
        vm.removeFuel(FuelProduct.DIESEL_S500)
        val fuels = mutableListOf<FuelProduct>()
        io.mockk.coEvery { enqueue.invoke(any()) } answers {
            fuels += firstArg<EnqueueContributionUseCase.Request>().fuelProduct
            com.anpfuel.application.usecase.contribution.EnqueueContributionOutcome.Queued(
                command = mockk(relaxed = true),
                historical = false,
            )
        }
        vm.submitContributions()
        advanceUntilIdle()
        val submitted = vm.submit.value
        assertTrue(submitted is CaptureOcrViewModel.SubmitState.Queued)
        org.junit.jupiter.api.Assertions.assertEquals(
            2, (submitted as CaptureOcrViewModel.SubmitState.Queued).count)
        org.junit.jupiter.api.Assertions.assertEquals(
            setOf(FuelProduct.GASOLINE_REGULAR, FuelProduct.ETHANOL), fuels.toSet())
    }

    @OptIn(ExperimentalCoroutinesApi::class)
    @Test
    fun `submit marks invalid rows without sending`() = runTest(dispatcher) {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR, "nove")
        vm.submitContributions()
        advanceUntilIdle()
        assertTrue(vm.submit.value is CaptureOcrViewModel.SubmitState.NoTarget)
        org.junit.jupiter.api.Assertions.assertTrue(
            FuelProduct.GASOLINE_REGULAR in vm.fuelErrors.value)
        io.mockk.coVerify(exactly = 0) { enqueue.invoke(any()) }
    }
}
