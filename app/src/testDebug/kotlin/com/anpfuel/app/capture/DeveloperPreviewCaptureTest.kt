package com.anpfuel.app.capture

import android.content.Context
import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.port.CaptureLocationSource
import com.anpfuel.application.port.ImagePriceOcr
import com.anpfuel.application.port.PhotoCaptureGate
import com.anpfuel.application.port.PhotoCapturePermission
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.port.CaptureOcrFlagProvider
import com.anpfuel.application.port.OcrPort
import com.anpfuel.application.portable.PhotoFlow
import com.anpfuel.application.usecase.capture.ConfirmPriceCaptureUseCase
import com.anpfuel.application.usecase.contribution.EnqueueContributionUseCase
import com.anpfuel.application.usecase.contribution.EnqueueReviewOutcome
import com.anpfuel.application.usecase.contribution.GetOwnedContributionsUseCase
import com.anpfuel.application.usecase.contribution.OwnedContributionStatus
import com.anpfuel.domain.contribution.ContributionState
import com.anpfuel.domain.repository.QueuedContribution
import com.anpfuel.application.usecase.directory.GetNearbyServerStationsUseCase
import com.anpfuel.application.usecase.directory.NearbyServerStationsOutcome
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.portable.FuelBoardOcr
import com.anpfuel.domain.portable.PortablePriceOcr
import com.anpfuel.domain.valueobject.FuelProduct
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/**
 * Debug-only developer preview capture: `setPreviewEnabled` needs the
 * debug [DeveloperCaptureMode] (compiled off in release, where
 * `ReleaseDevelopmentCaptureTest` asserts refusal). These tests live in
 * `testDebug` because CI runs `./gradlew test` across both variants and
 * shared `src/test` must stay variant-agnostic.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class DeveloperPreviewCaptureTest {

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
    fun `preview off by default needs canonical station and signed server authorization without GPS`() = runTest(dispatcher) {
        val gate = developmentGate()
        val vm = viewModel(enabled = true, hasPermission = false, gate = gate)
        assertFalse(vm.previewEnabled.value)
        assertFalse(vm.acceptPreviewPhoto(byteArrayOf(1), "image/jpeg", "private"))
        vm.setPreviewEnabled(true); advanceUntilIdle()
        var opens = 0
        vm.authorizeCamera { opens++ }; advanceUntilIdle(); assertEquals(0, opens)
        vm.pickPreviewStation("unknown"); assertEquals(null, vm.pickedStationId.value)
        prepareDevelopment(vm)
        vm.authorizeCamera { opens++ }; advanceUntilIdle()
        assertEquals(1, opens); assertTrue(vm.beginCamera())
        io.mockk.coVerify(exactly = 1) { gate.authorizeDevelopmentPreview(any(), any()) }
        io.mockk.coVerify(exactly = 0) { gate.authorize(any(), any(), any()) }
    }

    @Test
    fun `enabling preview lists every city station and single character waits for more input`() = runTest(dispatcher) {
        val vm = viewModel(enabled = true, hasPermission = false, gate = developmentGate())
        vm.setPreviewEnabled(true); advanceUntilIdle()
        val listed = kotlinx.coroutines.withContext(Dispatchers.Default) {
            kotlinx.coroutines.withTimeout(5000) { vm.previewStations.first { it.isNotEmpty() } }
        }
        assertEquals("123e4567-e89b-12d3-a456-426614174000", listed.first().stationId)
        assertEquals("", vm.previewQuery.value)
        vm.searchPreviewStation("a"); advanceUntilIdle()
        assertTrue(vm.previewStations.value.isEmpty())
    }

    @Test
    fun `server receipt issued seconds in the future still authorizes preview gallery`() = runTest(dispatcher) {
        val gate = mockk<PhotoCaptureGate>()
        val now = System.currentTimeMillis()
        io.mockk.coEvery { gate.authorizeDevelopmentPreview(any(), any()) } answers {
            PhotoCapturePermission("10000000-0000-4000-8000-000000000001", firstArg(), now + 5000, now + 125000, now + 86405000, "owner", "https://teste.abastevo.com.br", true)
        }
        every { gate.isCurrent(any()) } returns true
        val vm = viewModel(enabled = true, hasPermission = false, gate = gate)
        prepareDevelopment(vm)
        var opens = 0
        vm.authorizeCamera { opens++ }; advanceUntilIdle()
        assertEquals(1, opens)
        assertEquals(null, vm.gateFailure.value)
    }

    @Test
    fun `unrecognized OCR values wait for manual fuel choice`() = runTest(dispatcher) {
        val pixels = object : ImagePriceOcr {
            override suspend fun recognize(bytes: ByteArray) =
                com.anpfuel.domain.portable.FuelBoardOcr.Result(emptyList(), false, true, listOf(6150L, 4350L))
        }
        val vm = viewModel(enabled = true, hasPermission = false, gate = developmentGate(), pixels = pixels)
        prepareDevelopment(vm)
        vm.authorizeCamera { }; advanceUntilIdle()
        assertTrue(vm.acceptPreviewPhoto(byteArrayOf(1), "image/jpeg", "private")); advanceUntilIdle()
        kotlinx.coroutines.withContext(Dispatchers.Default) { kotlinx.coroutines.withTimeout(5000) { vm.processing.first { !it } } }
        assertEquals(listOf("6,150", "4,350"), vm.unassignedAmounts.value)
        vm.dismissUnassigned(0); assertEquals(listOf("4,350"), vm.unassignedAmounts.value)
        vm.assignUnassigned(0, FuelProduct.ETHANOL)
        assertEquals(emptyList<String>(), vm.unassignedAmounts.value)
        assertEquals("4,350", vm.fuelAmounts.value[FuelProduct.ETHANOL])
    }

    @Test
    fun `late normal location response cannot clear the developer station selection`() = runTest(dispatcher) {
        val delayed = CompletableDeferred<NearbyServerStationsOutcome>()
        val vm = viewModel(true, false, location = com.anpfuel.domain.valueobject.DeviceLocation.of(0.0, 0.0), nearbyDeferred = delayed)
        vm.loadNearby(); runCurrent()
        assertTrue(vm.nearbyLoading.value)
        prepareDevelopment(vm)
        val selected = "123e4567-e89b-12d3-a456-426614174000"
        assertEquals(selected, vm.pickedStationId.value)
        delayed.complete(NearbyServerStationsOutcome.Fresh(emptyList()))
        advanceUntilIdle()
        assertEquals(selected, vm.pickedStationId.value)
        vm.loadNearby(); advanceUntilIdle()
        assertEquals(selected, vm.pickedStationId.value)
    }

    @Test
    fun `gallery preview requires signed receipt and queues real reviewed subset without camera permission`() = runTest(dispatcher) {
        val vm = viewModel(enabled = true, hasPermission = false, gate = developmentGate())
        prepareDevelopment(vm)
        assertFalse(vm.acceptPreviewPhoto(byteArrayOf(1), "image/jpeg", "private"))
        vm.authorizeCamera { }; advanceUntilIdle()
        assertTrue(vm.acceptPreviewPhoto(byteArrayOf(1), "image/jpeg", "private")); advanceUntilIdle()
        kotlinx.coroutines.withContext(Dispatchers.Default) { kotlinx.coroutines.withTimeout(5000) { vm.processing.first { !it } } }
        vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR, "5,899")
        vm.setFuelAmount(FuelProduct.ETHANOL, "")
        io.mockk.coEvery { enqueue.invokeReview(any()) } returns com.anpfuel.application.usecase.contribution.EnqueueReviewOutcome.Queued(emptyList(), false)
        vm.submitContributions(); advanceUntilIdle()
        assertTrue(vm.submit.value is CaptureOcrViewModel.SubmitState.Queued)
        io.mockk.coVerify(exactly = 1) { enqueue.invokeReview(match { it.size == 1 && it[0].photoContext?.captureId == "10000000-0000-4000-8000-000000000001" && it[0].stationId == "123e4567-e89b-12d3-a456-426614174000" }) }
        vm.submitConfirmed()
        io.mockk.coVerify(exactly = 0) { enqueue.invoke(any()) }
    }

    @Test
    fun `validated queue shows sent feedback`() = runTest(dispatcher) {
        val owned = mockk<GetOwnedContributionsUseCase>()
        val vm = viewModel(enabled = true, hasPermission = false, gate = developmentGate(), owned = owned)
        prepareDevelopment(vm)
        vm.authorizeCamera { }; advanceUntilIdle()
        assertTrue(vm.acceptPreviewPhoto(byteArrayOf(1), "image/jpeg", "private")); advanceUntilIdle()
        kotlinx.coroutines.withContext(Dispatchers.Default) { kotlinx.coroutines.withTimeout(5000) { vm.processing.first { !it } } }
        vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR, "5,899")
        val cmd = QueuedContribution("cmd-1", 1, "{}", false)
        io.mockk.coEvery { enqueue.invokeReview(any()) } returns EnqueueReviewOutcome.Queued(listOf(cmd), false)
        io.mockk.coEvery { owned.invoke() } returns listOf(OwnedContributionStatus("cmd-1", 1, 1, ContributionState.Accepted))
        vm.submitContributions(); advanceUntilIdle()
        val sent = vm.submit.value as? CaptureOcrViewModel.SubmitState.Sent
        assertEquals(1, sent?.count)
    }

    @Test
    fun `partially rejected queue surfaces partial feedback`() = runTest(dispatcher) {
        val owned = mockk<GetOwnedContributionsUseCase>()
        val vm = viewModel(enabled = true, hasPermission = false, gate = developmentGate(), owned = owned)
        prepareDevelopment(vm)
        vm.authorizeCamera { }; advanceUntilIdle()
        assertTrue(vm.acceptPreviewPhoto(byteArrayOf(1), "image/jpeg", "private")); advanceUntilIdle()
        kotlinx.coroutines.withContext(Dispatchers.Default) { kotlinx.coroutines.withTimeout(5000) { vm.processing.first { !it } } }
        vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR, "5,899")
        vm.setFuelAmount(FuelProduct.ETHANOL, "4,350")
        val cmds = listOf(QueuedContribution("cmd-1", 1, "{}", false), QueuedContribution("cmd-2", 1, "{}", false))
        io.mockk.coEvery { enqueue.invokeReview(any()) } returns EnqueueReviewOutcome.Queued(cmds, false)
        io.mockk.coEvery { owned.invoke() } returns listOf(
            OwnedContributionStatus("cmd-1", 1, 1, ContributionState.Accepted),
            OwnedContributionStatus("cmd-2", 1, 3, ContributionState.Rejected, "contribution.rejected"))
        vm.submitContributions(); advanceUntilIdle()
        val partial = vm.submit.value as? CaptureOcrViewModel.SubmitState.Partial
        assertEquals(1, partial?.sent)
    }

    @Test
    fun `preview overprecision refused and toggling invalidates imported photo and proof`() = runTest(dispatcher) {
        val vm = viewModel(enabled = true, hasPermission = false, gate = developmentGate())
        prepareDevelopment(vm); vm.authorizeCamera { }; advanceUntilIdle()
        vm.acceptPreviewPhoto(byteArrayOf(1), "image/jpeg", "private"); advanceUntilIdle()
        kotlinx.coroutines.withContext(Dispatchers.Default) { kotlinx.coroutines.withTimeout(5000) { vm.processing.first { !it } } }
        vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR, "5,8999"); vm.submitContributions()
        assertTrue(FuelProduct.GASOLINE_REGULAR in vm.fuelErrors.value)
        vm.setPreviewEnabled(false)
        assertEquals(null, vm.reviewUri.value); assertEquals(null, vm.photoId.value)
        vm.submitContributions(); io.mockk.coVerify(exactly = 0) { enqueue.invokeReview(any()) }
    }
}
