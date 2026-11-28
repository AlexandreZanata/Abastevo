package com.anpfuel.app.capture

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.app.location.DeviceCityFollowGate
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.port.CaptureLocationSource
import com.anpfuel.application.port.CaptureOcrFlagProvider
import com.anpfuel.application.port.ImagePriceOcr
import com.anpfuel.application.port.OcrPort
import com.anpfuel.application.portable.PhotoFlow
import com.anpfuel.application.usecase.capture.ConfirmPriceCaptureUseCase
import com.anpfuel.application.usecase.community.GetCityCommunityFeedUseCase
import com.anpfuel.application.usecase.directory.GetNearbyServerStationsUseCase
import com.anpfuel.application.usecase.location.CityFollowOutcome
import com.anpfuel.application.usecase.location.DeviceSurroundings
import com.anpfuel.application.usecase.location.FollowDeviceCityUseCase
import com.anpfuel.domain.community.FeedCity
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.repository.ServerStationGateway
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DeviceLocation
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/** B-BR-LOC-01/03 in the contribution station picker. */
@OptIn(ExperimentalCoroutinesApi::class)
class StationLocationAssistTest {

    private val dispatcher = StandardTestDispatcher()
    private val fix = DeviceLocation.of(-12.55, -55.72)
    private val home = FeedCity("5103403", BrazilianState.MATO_GROSSO, "CUIABA")
    private val there = FeedCity("5107925", BrazilianState.MATO_GROSSO, "SORRISO")
    private val follow = mockk<FollowDeviceCityUseCase>()
    private val locations = mockk<LocationPermissionHandler>()
    private val gateway = mockk<ServerStationGateway>()
    private val gate = DeviceCityFollowGate()

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        every { locations.hasLocationPermission() } returns true
        coEvery { locations.getCurrentLocation(any()) } returns fix
        coEvery { gateway.search(any(), any(), any()) } returns ServerStationPage(listOf(station(9, "Listed")), null)
        coEvery { follow.follow(any()) } returns CityFollowOutcome.AlreadyThere
        coEvery { gateway.detail(any()) } answers { station(5) }
    }

    private val created = mutableListOf<CaptureOcrViewModel>()

    /**
     * Directory reads hop to Dispatchers.IO and resume on Main: drain them
     * before Main is reset so nothing completes after the test ends.
     */
    @AfterEach
    fun tearDown() {
        kotlinx.coroutines.runBlocking {
            withTimeout(5_000) {
                var rounds = 0
                while (rounds < 5 || created.any { it.stationSearchBusy.value }) {
                    dispatcher.scheduler.advanceUntilIdle()
                    kotlinx.coroutines.delay(10)
                    rounds += 1
                }
                dispatcher.scheduler.advanceUntilIdle()
            }
        }
        Dispatchers.resetMain()
    }

    private fun station(id: Int, name: String = "Posto $id", code: String = home.code) = ServerStation.create(
        stationId = "00000000-0000-4000-8000-%012d".format(id),
        displayName = name,
        locationQuality = StationLocationQuality.REVIEWED,
        latitude = -12.55,
        longitude = -55.72,
        cnpjNormalized = null,
        municipalityCode = code,
        state = "MT",
        currentRevisionId = null,
    )

    private fun viewModel(saved: SavedStateHandle = SavedStateHandle()): CaptureOcrViewModel {
        val flags = object : CaptureOcrFlagProvider {
            override fun isEnabled(): Boolean = true
        }
        val ocr = object : OcrPort {
            override fun candidatesFromText(text: String) = emptyList<com.anpfuel.domain.portable.PortablePriceOcr.OcrCandidate>()
        }
        val camera = mockk<CameraPermissionHandler>()
        every { camera.hasCameraPermission() } returns true
        val city = mockk<GetCityCommunityFeedUseCase>()
        coEvery { city.city() } returns home
        return CaptureOcrViewModel(
            ConfirmPriceCaptureUseCase(flags, ocr), camera, flags, mockk(), locations,
            mockk<GetNearbyServerStationsUseCase>(), mockk<PhotoFlow>(relaxed = true), CaptureLocationSource { null },
            mockk(relaxed = true), mockk<ImagePriceOcr>(), saved, city, gateway, mockk(relaxed = true),
            followDeviceCity = follow, cityFollowGate = gate,
        ).also(created::add)
    }

    private fun idle() = dispatcher.scheduler.advanceUntilIdle()

    private suspend fun awaitListed(vm: CaptureOcrViewModel) = withContext(Dispatchers.Default) {
        withTimeout(5_000) { vm.stationResults.first { it.isNotEmpty() } }
    }

    private suspend fun awaitAssist(vm: CaptureOcrViewModel, predicate: (StationLocationAssist) -> Boolean) =
        withContext(Dispatchers.Default) {
            withTimeout(5_000) {
                while (!predicate(vm.locationAssist.value)) {
                    dispatcher.scheduler.advanceUntilIdle()
                    kotlinx.coroutines.delay(5)
                }
            }
        }

    @Test
    fun `pre-selects the station the driver is at`() = runTest(dispatcher) {
        val atStation = station(1, "Posto Alfa")
        coEvery { follow.surroundings(fix) } returns DeviceSurroundings(listOf(NearbyServerStation(atStation, 42.0)), home)
        val vm = viewModel()
        vm.loadStationSearch()
        idle()

        assertEquals(LocationAssistStart.STARTED, vm.beginLocationAssist())
        awaitAssist(vm) { it is StationLocationAssist.Suggested }

        assertEquals(StationLocationAssist.Suggested(atStation.stationId, 42.0), vm.locationAssist.value)
        assertEquals(atStation.stationId, vm.pickedStationId.value)
        assertEquals(atStation, vm.selectedNominalStation.value)
    }

    @Test
    fun `asks for permission first and keeps manual selection when refused`() = runTest(dispatcher) {
        every { locations.hasLocationPermission() } returns false
        val vm = viewModel()
        vm.loadStationSearch()
        idle()

        assertEquals(LocationAssistStart.REQUEST_PERMISSION, vm.beginLocationAssist())
        vm.onStationLocationPermission(granted = false, canAskAgain = true)

        assertEquals(StationLocationAssist.PermissionDenied(canAskAgain = true), vm.locationAssist.value)
        awaitListed(vm)
        assertNull(vm.pickedStationId.value)
        coVerify(exactly = 0) { follow.surroundings(any()) }
    }

    @Test
    fun `asks only once per visit`() = runTest(dispatcher) {
        every { locations.hasLocationPermission() } returns false
        val vm = viewModel()
        vm.loadStationSearch()

        assertEquals(LocationAssistStart.REQUEST_PERMISSION, vm.beginLocationAssist())
        assertEquals(LocationAssistStart.SKIPPED, vm.beginLocationAssist())
    }

    @Test
    fun `rejecting the suggestion returns to the manual list without suggesting again`() = runTest(dispatcher) {
        val atStation = station(1)
        coEvery { follow.surroundings(fix) } returns DeviceSurroundings(listOf(NearbyServerStation(atStation, 30.0)), home)
        val vm = viewModel()
        vm.loadStationSearch()
        idle()
        vm.beginLocationAssist()
        awaitAssist(vm) { it is StationLocationAssist.Suggested }

        vm.rejectSuggestedStation()

        assertEquals(StationLocationAssist.Dismissed, vm.locationAssist.value)
        assertNull(vm.pickedStationId.value)
        assertNull(vm.selectedNominalStation.value)
        awaitListed(vm)
        vm.locateNearestStation()
        idle()
        assertEquals(StationLocationAssist.Dismissed, vm.locationAssist.value)
        assertNull(vm.pickedStationId.value)
    }

    @Test
    fun `no pinned station close enough leaves the manual list`() = runTest(dispatcher) {
        coEvery { follow.surroundings(fix) } returns
            DeviceSurroundings(listOf(NearbyServerStation(station(1), 900.0)), home)
        val vm = viewModel()
        vm.loadStationSearch()
        idle()
        vm.beginLocationAssist()
        awaitAssist(vm) { it == StationLocationAssist.NoneNearby }

        assertNull(vm.pickedStationId.value)
    }

    @Test
    fun `no fix or a failed lookup is reported as unavailable`() = runTest(dispatcher) {
        coEvery { locations.getCurrentLocation(any()) } returns null
        val vm = viewModel()
        vm.loadStationSearch()
        idle()
        vm.beginLocationAssist()
        awaitAssist(vm) { it == StationLocationAssist.Unavailable }

        coEvery { locations.getCurrentLocation(any()) } returns fix
        coEvery { follow.surroundings(fix) } returns null
        vm.locateNearestStation()
        awaitAssist(vm) { it == StationLocationAssist.Unavailable }
        assertNull(vm.pickedStationId.value)
    }

    @Test
    fun `a station picked by hand is never overridden`() = runTest(dispatcher) {
        val pending = kotlinx.coroutines.CompletableDeferred<DeviceSurroundings?>()
        coEvery { follow.surroundings(fix) } coAnswers { pending.await() }
        val vm = viewModel()
        vm.loadStationSearch()
        idle()
        awaitListed(vm)
        vm.beginLocationAssist()
        idle()
        vm.selectStation(station(9).stationId)

        pending.complete(DeviceSurroundings(listOf(NearbyServerStation(station(1), 20.0)), home))
        awaitAssist(vm) { it !is StationLocationAssist.Locating }

        assertEquals(station(9).stationId, vm.pickedStationId.value)
    }

    @Test
    fun `an explicit station target skips the location help`() = runTest(dispatcher) {
        val vm = viewModel()
        vm.bindEntryTarget(station(5).stationId, "GASOLINE_REGULAR")
        vm.loadStationSearch()

        assertEquals(LocationAssistStart.SKIPPED, vm.beginLocationAssist())
        coVerify(exactly = 0) { locations.getCurrentLocation(any()) }
    }

    @Test
    fun `a driver in another city gets that city and its station`() = runTest(dispatcher) {
        val atStation = station(1, "Posto Sorriso", code = there.code)
        coEvery { follow.surroundings(fix) } returns DeviceSurroundings(listOf(NearbyServerStation(atStation, 15.0)), there)
        coEvery { follow.follow(there) } returns CityFollowOutcome.Switched(null, there)
        val vm = viewModel()
        vm.loadStationSearch()
        idle()

        vm.beginLocationAssist()
        awaitAssist(vm) { it is StationLocationAssist.Suggested }

        assertEquals(there, vm.stationCity.value)
        assertEquals(atStation.stationId, vm.pickedStationId.value)
        coVerify(exactly = 1) { follow.follow(there) }
    }

    @Test
    fun `after a manual city choice the list moves but the preferred city is kept`() = runTest(dispatcher) {
        gate.suppressForSession()
        coEvery { follow.surroundings(fix) } returns DeviceSurroundings(emptyList(), there)
        val vm = viewModel()
        vm.loadStationSearch()
        idle()

        vm.beginLocationAssist()
        awaitAssist(vm) { it == StationLocationAssist.NoneNearby }

        assertEquals(there, vm.stationCity.value)
        coVerify(exactly = 0) { follow.follow(any()) }
    }
}
