package com.anpfuel.app.capture

import android.content.Context
import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.port.CaptureFix
import com.anpfuel.application.port.CaptureLocationSource
import com.anpfuel.application.port.PhotoCaptureGate
import com.anpfuel.application.port.PhotoCapturePermission
import com.anpfuel.application.port.ImagePriceOcr
import com.anpfuel.domain.portable.FuelBoardOcr
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.first
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.port.CaptureOcrFlagProvider
import com.anpfuel.application.port.OcrPort
import com.anpfuel.application.portable.PhotoFlow
import com.anpfuel.application.usecase.capture.ConfirmPriceCaptureUseCase
import com.anpfuel.application.usecase.contribution.EnqueueContributionUseCase
import com.anpfuel.application.usecase.contribution.GetOwnedContributionsUseCase
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
import kotlinx.coroutines.test.runCurrent
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
@OptIn(ExperimentalCoroutinesApi::class)
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
        locationSource: CaptureLocationSource = CaptureLocationSource { null },
        gate: PhotoCaptureGate = mockk(relaxed = true),
        pixels: ImagePriceOcr = object : ImagePriceOcr {
            override suspend fun recognize(bytes: ByteArray) = FuelBoardOcr.Result(emptyList(), false, false)
        },
        saved: SavedStateHandle = SavedStateHandle(),
        owned: GetOwnedContributionsUseCase = mockk(relaxed = true),
        nearbyDeferred: CompletableDeferred<NearbyServerStationsOutcome>? = null,
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
        io.mockk.coEvery { nearbyUseCase.invoke(any(), any(), any(), any()) } coAnswers { nearbyDeferred?.await() ?: nearby }
        val photos = mockk<PhotoFlow>()
        every { photos.prepareAt(any(), any(), any()) } returns photo
        every { photos.discard(any()) } returns Unit
        val city = mockk<com.anpfuel.application.usecase.community.GetCityCommunityFeedUseCase>()
        io.mockk.coEvery { city.city() } returns com.anpfuel.domain.community.FeedCity("5103403", com.anpfuel.domain.valueobject.BrazilianState.MATO_GROSSO, "Test City")
        val stationGateway = mockk<com.anpfuel.domain.repository.ServerStationGateway>()
        io.mockk.coEvery { stationGateway.search(any(), any(), any()) } returns com.anpfuel.domain.discovery.ServerStationPage(listOf(
            ServerStation.create("123e4567-e89b-12d3-a456-426614174000", "Test Station", StationLocationQuality.UNKNOWN, null, null, null, "5103403", "MT", null)), null)
        return CaptureOcrViewModel(useCase, handler, flags, enqueue, locations, nearbyUseCase, photos, locationSource, gate, pixels, saved, city, stationGateway, owned)
    }

    private fun developmentGate(): PhotoCaptureGate {
        val gate = mockk<PhotoCaptureGate>()
        val now = System.currentTimeMillis()
        io.mockk.coEvery { gate.authorizeDevelopmentPreview(any(), any()) } answers {
            PhotoCapturePermission("10000000-0000-4000-8000-000000000001", firstArg(), now - 1000, now + 119000, now + 86399000, "owner", "https://teste.abastevo.com.br", true)
        }
        every { gate.isCurrent(any()) } returns true
        return gate
    }

    private suspend fun prepareDevelopment(vm: CaptureOcrViewModel) {
        vm.setPreviewEnabled(true)
        kotlinx.coroutines.test.TestScope(dispatcher).advanceUntilIdle()
        vm.searchPreviewStation("Test")
        kotlinx.coroutines.test.TestScope(dispatcher).advanceUntilIdle()
        kotlinx.coroutines.withContext(Dispatchers.Default) { kotlinx.coroutines.withTimeout(5000) { vm.previewStations.first { it.isNotEmpty() } } }
        vm.pickPreviewStation("123e4567-e89b-12d3-a456-426614174000")
    }

    @Test
    fun `restored preview draft cannot use normal submission or forged receipt`() = runTest(dispatcher) {
        val saved = SavedStateHandle(mapOf("previewPhoto" to true, "photoId" to "local-photo",
            "capturedAt" to System.currentTimeMillis(), "amount.GASOLINE_REGULAR" to "5,899"))
        val vm = viewModel(enabled = true, hasPermission = true, saved = saved)
        vm.bindTarget("123e4567-e89b-12d3-a456-426614174000", "GASOLINE_REGULAR")
        vm.submitContributions(); vm.submitConfirmed()
        io.mockk.coVerify(exactly = 0) { enqueue.invokeReview(any()) }
        io.mockk.coVerify(exactly = 0) { enqueue.invoke(any()) }
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
    fun `ready photo attaches id and refused photo keeps code with review`() = runTest(dispatcher) {
        val ready = viewModel(enabled = true, hasPermission = true)
        ready.preparePhoto(byteArrayOf(1, 2, 3), "image/jpeg")
        ready.processing.first { !it }
        org.junit.jupiter.api.Assertions.assertEquals("photo-1", ready.photoId.value)
        assertTrue(ready.state.value is CaptureOcrUiState.NeedsConfirmation)

        val refused = viewModel(
            enabled = true,
            hasPermission = true,
            photo = PhotoFlow.PhotoResult.Refused("OVER_BUDGET"),
        )
        refused.preparePhoto(byteArrayOf(1, 2, 3), "image/jpeg")
        refused.processing.first { !it }
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
        listOf(-1.0, Double.NaN, Double.NEGATIVE_INFINITY, Double.POSITIVE_INFINITY).forEach {
            assertFalse(CaptureOcrViewModel.isInsideStationArea(it))
        }
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

    private fun reviewedState(): SavedStateHandle {
        val permission = validReceipt()
        return SavedStateHandle(mapOf("receipt" to arrayListOf(permission.captureId, permission.stationId,
            permission.issuedAtMillis.toString(), permission.cameraExpiresAtMillis.toString(), permission.expiresAtMillis.toString(),
            permission.ownerScope, permission.origin), "capturedAt" to (permission.issuedAtMillis + 500), "photoId" to "photo-1"))
    }

    private fun queuedState() = SavedStateHandle(mapOf(
        "queuedCount" to 1, "submittedIds" to arrayListOf("mine"), "reviewUri" to "private-review"))

    @Test fun `restored server received review confirms sent immediately without waiting for validation`() = runTest(dispatcher) {
        val owned = mockk<GetOwnedContributionsUseCase>()
        io.mockk.coEvery { owned.invoke() } returns listOf(
            com.anpfuel.application.usecase.contribution.OwnedContributionStatus("mine",1,1,
                com.anpfuel.domain.contribution.ContributionState.Pending))
        val vm = viewModel(true,true,saved=queuedState(),owned=owned)
        runCurrent()
        assertTrue(vm.submit.value is CaptureOcrViewModel.SubmitState.Sent)
        assertEquals(1,(vm.submit.value as CaptureOcrViewModel.SubmitState.Sent).count)
        io.mockk.coVerify(exactly=0) { enqueue.invokeReview(any()) }
    }

    @Test fun `retrying and missing owner receipts never confirm sent or enqueue again`() = runTest(dispatcher) {
        for (statuses in listOf(emptyList(),listOf(
            com.anpfuel.application.usecase.contribution.OwnedContributionStatus("mine",1,3,
                com.anpfuel.domain.contribution.ContributionState.Queued(true))),listOf(
            com.anpfuel.application.usecase.contribution.OwnedContributionStatus("foreign",1,1,
                com.anpfuel.domain.contribution.ContributionState.Accepted)))) {
            val owned = mockk<GetOwnedContributionsUseCase>()
            io.mockk.coEvery { owned.invoke() } returns statuses
            val vm = viewModel(true,true,saved=queuedState(),owned=owned)
            advanceUntilIdle()
            assertTrue(vm.submit.value is CaptureOcrViewModel.SubmitState.Queued)
        }
        io.mockk.coVerify(exactly=0) { enqueue.invokeReview(any()) }
    }
    @Test fun `review queues one atomic subset with stable IDs and original time even on double tap`() = runTest(dispatcher) {
        val saved = reviewedState()
        val gate = mockk<PhotoCaptureGate>()
        every { gate.isCurrent(any()) } returns true
        val vm = viewModel(true,true,gate=gate,saved=saved)
        vm.bindTarget(stationId,"ETHANOL")
        vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR,"6,59")
        vm.setFuelAmount(FuelProduct.ETHANOL,"4,32")
        vm.setFuelAmount(FuelProduct.DIESEL_S10,"")
        vm.setFuelAmount(FuelProduct.DIESEL_S500,"5,99")
        vm.removeFuel(FuelProduct.DIESEL_S500)
        val requests = io.mockk.slot<List<EnqueueContributionUseCase.Request>>()
        io.mockk.coEvery { enqueue.invokeReview(capture(requests)) } answers {
            com.anpfuel.application.usecase.contribution.EnqueueReviewOutcome.Queued(firstArg<List<EnqueueContributionUseCase.Request>>().map {
                com.anpfuel.domain.repository.QueuedContribution(it.clientSubmissionId,1,"synthetic",false)
            },false)
        }
        vm.submitContributions(); vm.submitContributions()
        advanceUntilIdle()
        assertEquals(2,(vm.submit.value as CaptureOcrViewModel.SubmitState.Queued).count)
        assertEquals(setOf(FuelProduct.GASOLINE_REGULAR,FuelProduct.ETHANOL),requests.captured.map { it.fuelProduct }.toSet())
        assertTrue(requests.captured.all { it.capturedAtMillis == saved.get<Long>("capturedAt") && it.photoId == "photo-1" && it.unit == "L" })
        assertTrue(requests.captured.all { it.clientSubmissionId.startsWith(validReceipt().captureId+":") })
        vm.submitContributions()
        val restored = viewModel(true,true,gate=gate,saved=saved)
        restored.bindTarget(stationId,"ETHANOL"); restored.submitContributions()
        assertEquals(2,(restored.submit.value as CaptureOcrViewModel.SubmitState.Queued).count)
        io.mockk.coVerify(exactly=1) { enqueue.invokeReview(any()) }
        io.mockk.coVerify(exactly=0) { enqueue.invoke(any()) }
    }
    @Test fun `disabled and failed review do not claim queued and invalid sibling queues nothing`() = runTest(dispatcher) {
        val gate=mockk<PhotoCaptureGate>(); every { gate.isCurrent(any()) } returns true
        val vm=viewModel(true,true,gate=gate,saved=reviewedState()); vm.bindTarget(stationId,"ETHANOL")
        vm.setFuelAmount(FuelProduct.ETHANOL,"4,32"); vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR,"invalid")
        vm.submitContributions(); advanceUntilIdle()
        io.mockk.coVerify(exactly=0) { enqueue.invokeReview(any()) }
        assertTrue(vm.submit.value is CaptureOcrViewModel.SubmitState.NoTarget)
        vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR,"")
        io.mockk.coEvery { enqueue.invokeReview(any()) } returns com.anpfuel.application.usecase.contribution.EnqueueReviewOutcome.Disabled
        vm.submitContributions(); advanceUntilIdle()
        assertTrue(vm.submit.value is CaptureOcrViewModel.SubmitState.Disabled)
        io.mockk.coEvery { enqueue.invokeReview(any()) } throws java.io.IOException("synthetic")
        vm.submitContributions(); advanceUntilIdle()
        assertTrue(vm.submit.value is CaptureOcrViewModel.SubmitState.Failed)
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
    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private fun validReceipt() = System.currentTimeMillis().let { now ->
        PhotoCapturePermission("589a13ba-1f78-41dc-a4ab-d63a511c73da", stationId, now - 1000, now + 119000,
            now + 86399000, "synthetic-owner", "https://example.invalid")
    }

    @Test fun `location denial and server refusal never launch camera`() = runTest(dispatcher) {
        val gate = mockk<PhotoCaptureGate>()
        val missing = viewModel(true, true, gate = gate)
        missing.bindTarget(stationId, "ETHANOL")
        var launches = 0
        missing.authorizeCamera { launches++ }
        advanceUntilIdle()
        assertEquals(0, launches)
        io.mockk.coVerify(exactly = 0) { gate.authorize(any(), any(), any()) }
        val fix = CaptureFix(-12.5, -55.7, 10.0, System.currentTimeMillis(), true, true, false)
        val refused = viewModel(true, true, gate = gate, locationSource = CaptureLocationSource { fix })
        refused.bindTarget(stationId, "ETHANOL")
        io.mockk.coEvery { gate.authorize(any(), any(), any()) } throws java.io.IOException("refused")
        refused.authorizeCamera { launches++ }
        advanceUntilIdle()
        assertEquals(0, launches)
        assertFalse(refused.gateBusy.value)
        assertEquals("photo.authorization-unavailable", refused.gateFailure.value)
    }

    @Test fun `duplicate taps and changed station cannot launch stale camera`() = runTest(dispatcher) {
        val result = CompletableDeferred<PhotoCapturePermission>()
        val gate = mockk<PhotoCaptureGate>()
        io.mockk.coEvery { gate.authorize(any(), any(), any()) } coAnswers { result.await() }
        every { gate.isCurrent(any()) } returns true
        val fix = CaptureFix(-12.5, -55.7, 10.0, System.currentTimeMillis(), true, true, false)
        val vm = viewModel(true, true, gate = gate, locationSource = CaptureLocationSource { fix })
        vm.bindTarget(stationId, "ETHANOL")
        var launches = 0
        vm.authorizeCamera { launches++ }
        vm.authorizeCamera { launches++ }
        runCurrent()
        vm.bindTarget("e7d85d34-74ec-5d35-b3f6-519dc44ce370", "ETHANOL")
        result.complete(validReceipt())
        advanceUntilIdle()
        assertEquals(0, launches)
        io.mockk.coVerify(exactly = 1) { gate.authorize(any(), any(), any()) }
    }

    @Test fun `receipt is checked again after camera permission and survives saved state`() = runTest(dispatcher) {
        val saved = SavedStateHandle()
        val gate = mockk<PhotoCaptureGate>()
        val receipt = validReceipt()
        io.mockk.coEvery { gate.authorize(any(), any(), any()) } returns receipt
        every { gate.isCurrent(any()) } returns true
        val fix = CaptureFix(-12.5, -55.7, 10.0, System.currentTimeMillis(), true, true, false)
        val vm = viewModel(true, true, gate = gate, locationSource = CaptureLocationSource { fix }, saved = saved)
        vm.bindTarget(stationId, "ETHANOL")
        var launches = 0
        vm.authorizeCamera { launches++ }
        advanceUntilIdle()
        assertEquals(1, launches)
        val recovered = viewModel(true, true, gate = gate, saved = saved)
        recovered.bindTarget(stationId, "ETHANOL")
        assertTrue(recovered.beginCamera())
        every { gate.isCurrent(any()) } returns false
        assertFalse(recovered.beginCamera())
        assertFalse(saved.keys().any { it.contains("latitude") || it.contains("longitude") })
    }

    @Test fun `pixel OCR edits removals crop conditions and draft survive recovery`() = runTest(dispatcher) {
        val saved = SavedStateHandle()
        var calls = 0
        val delayed = CompletableDeferred<FuelBoardOcr.Result>()
        val pixels = object : ImagePriceOcr {
            override suspend fun recognize(bytes: ByteArray): FuelBoardOcr.Result {
                calls++
                return if (calls == 1) delayed.await() else FuelBoardOcr.Result(
                    listOf(FuelBoardOcr.Row(FuelProduct.ETHANOL, 4390), FuelBoardOcr.Row(FuelProduct.GASOLINE_REGULAR, 6990)), false, false)
            }
        }
        val vm = viewModel(true, true, pixels = pixels, saved = saved)
        vm.preparePhoto(byteArrayOf(1), "image/jpeg")
        vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR, "6,70")
        vm.removeFuel(FuelProduct.ETHANOL)
        delayed.complete(FuelBoardOcr.Result(listOf(FuelBoardOcr.Row(FuelProduct.GASOLINE_REGULAR, 6590), FuelBoardOcr.Row(FuelProduct.ETHANOL, 4320)), true, false))
        vm.processing.first { !it }
        assertEquals(mapOf(FuelProduct.GASOLINE_REGULAR to "6,70"), vm.fuelAmounts.value)
        assertTrue(vm.conditional.value)
        vm.replacePhoto(byteArrayOf(2), "image/jpeg")
        vm.processing.first { !it }
        assertEquals(mapOf(FuelProduct.GASOLINE_REGULAR to "6,70"), vm.fuelAmounts.value)
        assertTrue(vm.conditional.value, "cropping must not erase original payment ambiguity")
        val recovered = viewModel(true, true, saved = saved)
        assertEquals(vm.fuelAmounts.value, recovered.fuelAmounts.value)
        assertEquals(vm.removedFuels.value, recovered.removedFuels.value)
        assertEquals(vm.originalCapturedAtMillis, recovered.originalCapturedAtMillis)
    }

    @Test fun `cancelled late OCR cannot restore rows or enqueue`() = runTest(dispatcher) {
        val delayed = CompletableDeferred<FuelBoardOcr.Result>()
        val pixels = object : ImagePriceOcr { override suspend fun recognize(bytes: ByteArray) = delayed.await() }
        val vm = viewModel(true, true, pixels = pixels)
        vm.preparePhoto(byteArrayOf(1), "image/jpeg")
        vm.cameraCancelled()
        delayed.complete(FuelBoardOcr.Result(listOf(FuelBoardOcr.Row(FuelProduct.ETHANOL, 4320)), false, false))
        advanceUntilIdle()
        assertTrue(vm.fuelAmounts.value.isEmpty())
        assertTrue(vm.state.value is CaptureOcrUiState.Cancelled)
        io.mockk.coVerify(exactly = 0) { enqueue.invoke(any()) }
    }

}
