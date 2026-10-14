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

/** Ordinary camera and durable review: shared by debug and release. */
@OptIn(ExperimentalCoroutinesApi::class)
class NormalCameraContributionTest {

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

    private fun normalGate(): PhotoCaptureGate {
        val gate = mockk<PhotoCaptureGate>()
        val now = System.currentTimeMillis()
        io.mockk.coEvery { gate.authorizeCommunity(any(), any()) } answers {
            PhotoCapturePermission("10000000-0000-4000-8000-000000000001", firstArg(), now - 1000, now + 119000, now + 86399000, "owner", "https://teste.abastevo.com.br", false)
        }
        every { gate.isCurrent(any()) } returns true
        return gate
    }

    private suspend fun prepareNormal(vm: CaptureOcrViewModel) {
        vm.loadStationSearch()
        kotlinx.coroutines.test.TestScope(dispatcher).advanceUntilIdle()
        vm.searchStations("Test")
        kotlinx.coroutines.test.TestScope(dispatcher).advanceUntilIdle()
        kotlinx.coroutines.withContext(Dispatchers.Default) { kotlinx.coroutines.withTimeout(5000) { vm.stationResults.first { it.isNotEmpty() } } }
        vm.selectStation("123e4567-e89b-12d3-a456-426614174000")
    }

    @Test
    fun `ordinary camera requires signed receipt and queues real reviewed subset without GPS`() = runTest(dispatcher) {
        val vm = viewModel(enabled = true, hasPermission = true, gate = normalGate())
        prepareNormal(vm)
        assertFalse(vm.acceptCameraPhoto(byteArrayOf(1), "image/jpeg", "private"))
        vm.authorizeCamera { }; advanceUntilIdle()
        assertTrue(vm.acceptCameraPhoto(byteArrayOf(1), "image/jpeg", "private")); advanceUntilIdle()
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
        val vm = viewModel(enabled = true, hasPermission = true, gate = normalGate(), owned = owned)
        prepareNormal(vm)
        vm.authorizeCamera { }; advanceUntilIdle()
        assertTrue(vm.acceptCameraPhoto(byteArrayOf(1), "image/jpeg", "private")); advanceUntilIdle()
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
        val vm = viewModel(enabled = true, hasPermission = true, gate = normalGate(), owned = owned)
        prepareNormal(vm)
        vm.authorizeCamera { }; advanceUntilIdle()
        assertTrue(vm.acceptCameraPhoto(byteArrayOf(1), "image/jpeg", "private")); advanceUntilIdle()
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

    @Test fun `overprecision is refused and changed station invalidates ordinary proof`() = runTest(dispatcher) {
        val vm = viewModel(true, true, gate = normalGate())
        prepareNormal(vm); vm.authorizeCamera { }; advanceUntilIdle()
        assertTrue(vm.acceptCameraPhoto(byteArrayOf(1), "image/jpeg", "private"))
        kotlinx.coroutines.withContext(Dispatchers.Default) { kotlinx.coroutines.withTimeout(5000) { vm.processing.first { !it } } }
        vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR, "5,8999"); vm.submitContributions()
        assertTrue(FuelProduct.GASOLINE_REGULAR in vm.fuelErrors.value)
        vm.pickStation(null); vm.setFuelAmount(FuelProduct.GASOLINE_REGULAR, "5,899"); vm.submitContributions()
        io.mockk.coVerify(exactly = 0) { enqueue.invokeReview(any()) }
    }
}
