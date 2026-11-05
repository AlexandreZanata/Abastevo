package com.anpfuel.app.ui.stations

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.usecase.directory.*
import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.price.StationPricesOutcome
import com.anpfuel.application.usecase.community.GetCityCommunityFeedUseCase
import com.anpfuel.application.usecase.community.GetCommunityPriceGroupsUseCase
import com.anpfuel.application.usecase.community.CommunityPriceGroupsOutcome
import com.anpfuel.application.usecase.profile.GetStationProfileUseCase
import com.anpfuel.application.usecase.profile.StationProfileOutcome
import com.anpfuel.data.repository.ChargingDiscoveryRepository
import com.anpfuel.data.repository.PlaceDetail
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.valueobject.FuelProduct
import io.mockk.*
import java.io.IOException
import java.util.Locale
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.*
import org.junit.jupiter.api.Assertions.*

@OptIn(ExperimentalCoroutinesApi::class)
class StationPageViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val local = mockk<GetStationPricesUseCase>()
    private val resolve = mockk<ResolveStationByCnpjUseCase>()
    private val detail = mockk<GetServerStationDetailUseCase>()
    private val prices = mockk<GetCommunityPriceGroupsUseCase>()
    private val profile = mockk<GetStationProfileUseCase>()
    private val cityFeed = mockk<GetCityCommunityFeedUseCase>()
    private val auth = mockk<AuthFlow>()
    private val charging = mockk<ChargingDiscoveryRepository>()
    private val id = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private val station = ServerStation.create(id, "Central", StationLocationQuality.UNKNOWN,
        null, null, "04218406000104", null, "SP", null)
    @BeforeEach fun before() {
        Dispatchers.setMain(dispatcher)
        coEvery { local(any(), any(), any(), any()) } returns StationPricesOutcome.StationDetailMissing(requiresOnDemandDownload = true)
        coEvery { prices(any(), any()) } returns CommunityPriceGroupsOutcome.Fresh(
            com.anpfuel.domain.model.BackendPriceGroups.create(id, null, emptyList(), 1L, 2L))
        coEvery { profile(any()) } returns StationProfileOutcome.Unclaimed
        coEvery { cityFeed.city() } returns null
        coEvery { charging.linkedToStation(any()) } returns emptyList()
        every { auth.refreshSession() } returns AuthApiResult.Ok(null)
    }
    @AfterEach fun after() { Dispatchers.resetMain() }
    private fun vm(key: String = id, fuel: String = "ETHANOL") = StationPageViewModel(
        SavedStateHandle(mapOf("stationKey" to key, "fuelProduct" to fuel)), local, resolve, detail,
        prices, profile, cityFeed, auth, charging).also { it.ioDispatcher = dispatcher }

    @Test fun malformedRouteDoesNotReadOrInventStation() = runTest(dispatcher) {
        val vm = vm("../auth"); vm.load(Locale.US); advanceUntilIdle()
        assertTrue(vm.state.value.unavailable)
        coVerify(exactly = 0) { detail(any()) }
        coVerify(exactly = 0) { resolve(any()) }
    }
    @Test fun legacyCnpjResolvesExactlyAndSurvivesRouteRestoration() = runTest(dispatcher) {
        coEvery { resolve("04218406000104") } returns ServerStationDetailOutcome.Fresh(station)
        val vm = vm("04218406000104"); vm.load(Locale.US); advanceUntilIdle()
        assertEquals(id, vm.state.value.canonical?.stationId)
        assertEquals(FuelProduct.ETHANOL, vm.state.value.fuel)
        assertFalse(vm.state.value.unavailable)
        coVerify(exactly = 1) { charging.linkedToStation(id) }
    }
    @Test fun canonicalRouteShowsCachedLinksWhenDirectoryIsOffline() = runTest(dispatcher) {
        coEvery { detail(id) } throws IOException()
        val place = ChargingPlace(
            id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", entityKey = "a".repeat(64),
            externalId = "synthetic", name = "Synthetic Charge", country = "BR", latitude = null, longitude = null,
            publicAccess = ChargingAccess.PUBLIC, reportedPoints = 0, groups = emptyList(),
        )
        val linked = PlaceDetail(place, ChargingDiscoveryRepository.NATIONAL_PROFILE, "synthetic", 1, 1)
        coEvery { charging.linkedToStation(id) } returns listOf(linked)
        val vm = vm(); vm.load(Locale.US); advanceUntilIdle()
        assertEquals(listOf(linked), vm.state.value.linkedChargers)
        assertFalse(vm.state.value.canParticipate)
        coVerify(exactly = 1) { charging.linkedToStation(id) }
    }

    @Test fun unresolvedLegacyRouteNeverQueriesLinks() = runTest(dispatcher) {
        coEvery { resolve("04218406000104") } throws IOException()
        val vm = vm("04218406000104"); vm.load(Locale.US); advanceUntilIdle()
        assertTrue(vm.state.value.linkedChargers.isEmpty())
        coVerify(exactly = 0) { charging.linkedToStation(any()) }
    }

    @Test fun staleLegacyIdentityCannotEnableParticipation() = runTest(dispatcher) {
        coEvery { resolve(any()) } returns ServerStationDetailOutcome.StaleCache(station, IOException())
        val vm = vm("04218406000104"); vm.load(Locale.US); advanceUntilIdle()
        assertTrue(vm.state.value.identityStale)
        assertFalse(vm.state.value.canParticipate)
    }
    @Test fun selectingFuelRejectsLateResponseForPreviousFuel() = runTest(dispatcher) {
        coEvery { detail(id) } returns ServerStationDetailOutcome.Fresh(station)
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        coEvery { prices(id, FuelProduct.ETHANOL) } coAnswers {
            entered.complete(Unit)
            withContext(NonCancellable) { release.await() }
            CommunityPriceGroupsOutcome.Unavailable(IOException())
        }
        val vm = vm(); vm.load(Locale.US); runCurrent(); assertTrue(entered.isCompleted)
        vm.selectFuel(FuelProduct.DIESEL_S10, Locale.US); runCurrent()
        release.complete(Unit); advanceUntilIdle()
        assertEquals(FuelProduct.DIESEL_S10, vm.state.value.fuel)
        assertFalse(vm.state.value.priceUnavailable)
    }
    @Test fun foreignDirectoryIdentityCannotEnableRatingsOrPrices() = runTest(dispatcher) {
        coEvery { detail("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa") } returns ServerStationDetailOutcome.Fresh(station)
        val vm = vm("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"); vm.load(Locale.US); advanceUntilIdle()
        assertNull(vm.state.value.canonical)
        assertFalse(vm.state.value.canParticipate)
        coVerify(exactly = 0) { prices(any(), any()) }
    }
    @Test fun directoryExceptionLeavesRecoverableUnavailableState() = runTest(dispatcher) {
        coEvery { detail(id) } throws IOException()
        val vm = vm(); vm.load(Locale.US); advanceUntilIdle()
        assertTrue(vm.state.value.unavailable)
        assertFalse(vm.state.value.loading)
    }
    @Test fun communityFeedPriceAppearsWhenStationMatches() = runTest(dispatcher) {
        coEvery { detail(id) } returns ServerStationDetailOutcome.Fresh(station)
        val city = com.anpfuel.domain.community.FeedCity("3550308",
            com.anpfuel.domain.valueobject.BrazilianState.SAO_PAULO, "São Paulo")
        val now = java.time.Instant.parse("2026-10-06T12:00:00Z")
        val item = com.anpfuel.domain.community.CommunityFeedItem(id, "Central",
            FuelProduct.ETHANOL, 4440L, now, now.plusSeconds(3600), 3, 2, "LOW", 1L)
        coEvery { cityFeed.city() } returns city
        coEvery { cityFeed(any(), any()) } returns com.anpfuel.domain.community.CommunityFeedPage(
            listOf(item), null, now)
        val vm = vm(); vm.load(Locale.US); advanceUntilIdle()
        assertEquals(4440L, vm.state.value.communityItem?.amountMilliBrl)
        assertEquals(3, vm.state.value.communityItem?.supporters)
    }
    @Test fun communityFeedMissKeepsHonestEmptyState() = runTest(dispatcher) {
        coEvery { detail(id) } returns ServerStationDetailOutcome.Fresh(station)
        val city = com.anpfuel.domain.community.FeedCity("3550308",
            com.anpfuel.domain.valueobject.BrazilianState.SAO_PAULO, "São Paulo")
        val now = java.time.Instant.parse("2026-10-06T12:00:00Z")
        coEvery { cityFeed.city() } returns city
        coEvery { cityFeed(any(), any()) } returns com.anpfuel.domain.community.CommunityFeedPage(
            emptyList(), null, now)
        val vm = vm(); vm.load(Locale.US); advanceUntilIdle()
        assertNull(vm.state.value.communityItem)
    }
    @Test fun offlineResolutionPreservesDatedAnpButNeverEnablesSocialWrites() = runTest(dispatcher) {
        val week = com.anpfuel.domain.valueobject.SurveyWeek.fromIsoDates("2026-09-27", "2026-10-03")
        val row = com.anpfuel.domain.model.StationPrice.create(
            priceSurveyId = com.anpfuel.domain.valueobject.DomainId.forSurveyWeek(week), surveyWeek = week,
            station = com.anpfuel.domain.model.RetailStation.create(
                com.anpfuel.domain.valueobject.Cnpj.parse("04218406000104"), "Example Legal", "Example",
                "Example Avenue, 10", "Curitiba", com.anpfuel.domain.valueobject.BrazilianState.PARANA, "BR"),
            fuelProduct = FuelProduct.ETHANOL, price = com.anpfuel.domain.valueobject.PriceAmount.of("3.85"),
            collectedAt = java.time.LocalDate.of(2026, 9, 29))
        coEvery { local(FuelProduct.ETHANOL, any(), any(), any()) } returns StationPricesOutcome.Success(
            week, com.anpfuel.domain.valueobject.BrazilianState.PARANA, "Curitiba", FuelProduct.ETHANOL, listOf(row))
        coEvery { resolve(any()) } returns ServerStationDetailOutcome.Unavailable(IOException())
        val vm = vm("04218406000104"); vm.load(Locale.US); advanceUntilIdle()
        assertEquals("Example", vm.state.value.name)
        assertNotNull(vm.state.value.localDetail)
        val datedDetail = vm.state.value.localDetail
        assertNotNull(vm.state.value.navigationQuery)
        assertFalse(vm.state.value.canParticipate)
        vm.selectFuel(FuelProduct.DIESEL_S10, Locale.US); advanceUntilIdle()
        assertNull(vm.state.value.localDetail)
        assertEquals("Example", vm.state.value.name)
        assertNotNull(vm.state.value.navigationQuery)

        // Resolving current identity must replace the historical price heading
        // without changing the dated ANP price or its collection period.
        coEvery { resolve(any()) } returns ServerStationDetailOutcome.Fresh(
            ServerStation.create(id, "Current Trade", StationLocationQuality.UNKNOWN,
                null, null, "04218406000104", null, "PR", null,
                legalName = "Current Legal", address = mapOf("street" to "Current Street", "number" to "12")))
        vm.selectFuel(FuelProduct.ETHANOL, Locale.US); advanceUntilIdle()
        assertEquals("Current Trade", vm.state.value.name)
        assertEquals("Current Legal", vm.state.value.canonical?.secondaryLegalName)
        assertEquals("Current Street, 12", vm.state.value.address)
        assertEquals(datedDetail?.surveyWeekLabel, vm.state.value.localDetail?.surveyWeekLabel)
        assertEquals(datedDetail?.station?.priceFormatted, vm.state.value.localDetail?.station?.priceFormatted)
    }

}
