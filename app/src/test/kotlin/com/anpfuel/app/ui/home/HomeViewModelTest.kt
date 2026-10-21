package com.anpfuel.app.ui.home

import com.anpfuel.application.usecase.location.PreferredLocation
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.price.GetMunicipalityPricesUseCase
import com.anpfuel.application.usecase.price.MunicipalityPricesResult
import com.anpfuel.application.usecase.readiness.DataReadinessResult
import com.anpfuel.application.usecase.readiness.GetDataReadinessUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
import com.anpfuel.application.usecase.vehicle.GetTankFillCostEstimatesUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.application.usecase.vehicle.TankFillCostEstimatesResult
import com.anpfuel.app.mapper.PriceFormatter
import com.anpfuel.application.usecase.vehicle.VehicleTankFillCostEstimate
import com.anpfuel.domain.model.TankFillCostEstimate
import com.anpfuel.domain.model.TankFillCostUnitPriceSource
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import java.util.Locale
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.runCurrent
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
class HomeViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val getDataReadinessUseCase = mockk<GetDataReadinessUseCase>()
    private val getMunicipalityPricesUseCase = mockk<GetMunicipalityPricesUseCase>()
    private val selectLocationUseCase = mockk<SelectLocationUseCase>()
    private val syncPriceTablesUseCase = mockk<SyncPriceTablesUseCase>()
    private val listVehiclesUseCase = mockk<ListVehiclesUseCase>()
    private val getTankFillCostEstimatesUseCase = mockk<GetTankFillCostEstimatesUseCase>()
    private val homeVehiclePresenceStore = mockk<com.anpfuel.data.local.preferences.HomeVehiclePresenceStore>()
    private val homeBenefitsVisibilityStore = mockk<com.anpfuel.data.local.preferences.HomeBenefitsVisibilityStore>()
    private val manualSyncVerificationStore =
        mockk<com.anpfuel.data.local.preferences.ManualSyncVerificationStore>(relaxed = true)
    private val observeNetworkConnectivityUseCase = mockk<ObserveNetworkConnectivityUseCase>()
    private val connected = MutableStateFlow(true)

    private val communitySummary = mockk<com.anpfuel.application.usecase.community.GetCityCommunitySummaryUseCase>()
    private val communitySnapshot = mockk<com.anpfuel.domain.community.CommunityFeedLocalSnapshot>()
    private lateinit var viewModel: HomeViewModel

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val state = BrazilianState.PARANA
    private val municipality = "Curitiba"

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        connected.value = true
        every { observeNetworkConnectivityUseCase.invoke() } returns connected
        every { homeVehiclePresenceStore.hasVehicle() } returns false
        every { homeVehiclePresenceStore.setHasVehicle(any()) } returns Unit
        every { homeBenefitsVisibilityStore.isVisible() } returns true
        every { homeBenefitsVisibilityStore.setVisible(any()) } returns Unit
        coEvery { communitySnapshot.snapshotSummary(any()) } returns null
        every { manualSyncVerificationStore.get() } returns null
        viewModel = HomeViewModel(
            getDataReadinessUseCase = getDataReadinessUseCase,
            getMunicipalityPricesUseCase = getMunicipalityPricesUseCase,
            selectLocationUseCase = selectLocationUseCase,
            syncPriceTablesUseCase = syncPriceTablesUseCase,
            listVehiclesUseCase = listVehiclesUseCase,
            getTankFillCostEstimatesUseCase = getTankFillCostEstimatesUseCase,
            homeVehiclePresenceStore = homeVehiclePresenceStore,
            homeBenefitsVisibilityStore = homeBenefitsVisibilityStore,
            manualSyncVerificationStore = manualSyncVerificationStore,
            observeNetworkConnectivityUseCase = observeNetworkConnectivityUseCase,
            communitySummary = communitySummary,
            communitySnapshot = communitySnapshot,
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun loadLeavesTankFillEstimatesEmptyWhenNoVehicles() = runTest(dispatcher) {
        stubReadyHome(vehicles = emptyList(), tankItems = emptyList())

        viewModel.load(Locale.US)
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value.tankFillCostEstimates.isEmpty())
    }

    @Test
    fun loadMapsTankFillEstimatesForRegisteredVehicles() = runTest(dispatcher) {
        val vehicle = sampleVehicle("Gol")
        val estimate = TankFillCostEstimate(
            vehicleId = vehicle.id,
            displayName = vehicle.displayName,
            tankCapacity = vehicle.tankCapacity,
            fuelProduct = vehicle.fuelProduct,
            unitPrice = PriceAmount.of("5.49"),
            totalCost = PriceAmount.of("274.50"),
            unitPriceSource = TankFillCostUnitPriceSource.CHEAPEST_STATION,
            stationDisplayName = "Cheap",
        )
        stubReadyHome(
            vehicles = listOf(vehicle),
            tankItems = listOf(VehicleTankFillCostEstimate(vehicle = vehicle, estimate = estimate)),
        )

        viewModel.load(Locale.US)
        advanceUntilIdle()

        assertEquals(1, viewModel.uiState.value.tankFillCostEstimates.size)
        assertEquals("Gol", viewModel.uiState.value.tankFillCostEstimates.single().displayName)
        assertEquals(
            PriceFormatter.formatAmount(PriceAmount.of("274.50"), Locale.US),
            viewModel.uiState.value.tankFillCostEstimates.single().totalCostFormatted,
        )
    }

    @Test
    fun loadDoesNotShowLoadingIndicatorWhenContentAlreadyPresent() = runTest(dispatcher) {
        stubReadyHome(vehicles = emptyList(), tankItems = emptyList())

        viewModel.load(Locale.US)
        advanceUntilIdle()
        assertEquals(false, viewModel.uiState.value.isLoading)

        viewModel.load(Locale.US)
        advanceUntilIdle()

        assertEquals(false, viewModel.uiState.value.isLoading)
    }

    @Test
    fun priceFilterKeepsFuelOrderAndStateWithoutReloading() = runTest(dispatcher) {
        stubReadyHome(
            vehicles = emptyList(),
            tankItems = emptyList(),
            prices = listOf(
                sampleAveragePrice(FuelProduct.GASOLINE_REGULAR, "6.83"),
                sampleAveragePrice(FuelProduct.ETHANOL, "3.96"),
            ),
        )

        viewModel.load(Locale.US)
        advanceUntilIdle()

        viewModel.onPriceSortChanged(HomePriceSortOrder.PRICE_ASC)
        advanceUntilIdle()
        viewModel.onPriceSortChanged(HomePriceSortOrder.PRICE_DESC)
        advanceUntilIdle()

        assertEquals(HomePriceSortOrder.PRICE_DESC, viewModel.uiState.value.priceSort)
        assertEquals(
            listOf(FuelProduct.ETHANOL, FuelProduct.GASOLINE_REGULAR),
            viewModel.uiState.value.prices.map { it.fuelProduct },
        )
        coVerify(exactly = 1) { getMunicipalityPricesUseCase.invoke() }
    }

    @Test
    fun loadMapsMultipleTankFillCardsInOrder() = runTest(dispatcher) {
        val first = sampleVehicle("Gol", sortOrder = 0)
        val second = sampleVehicle("Onix", sortOrder = 1)
        stubReadyHome(
            vehicles = listOf(first, second),
            tankItems = listOf(
                VehicleTankFillCostEstimate(
                    vehicle = first,
                    estimate = tankEstimate(first, "274.50"),
                ),
                VehicleTankFillCostEstimate(
                    vehicle = second,
                    estimate = tankEstimate(second, "220.00"),
                ),
            ),
        )

        viewModel.load(Locale.US)
        advanceUntilIdle()

        assertEquals(
            listOf("Gol", "Onix"),
            viewModel.uiState.value.tankFillCostEstimates.map { it.displayName },
        )
    }

    @Test
    fun cachedVehiclePresenceShowsBeforeFirstLoad() {
        every { homeVehiclePresenceStore.hasVehicle() } returns true
        viewModel = HomeViewModel(
            getDataReadinessUseCase = getDataReadinessUseCase,
            getMunicipalityPricesUseCase = getMunicipalityPricesUseCase,
            selectLocationUseCase = selectLocationUseCase,
            syncPriceTablesUseCase = syncPriceTablesUseCase,
            listVehiclesUseCase = listVehiclesUseCase,
            getTankFillCostEstimatesUseCase = getTankFillCostEstimatesUseCase,
            homeVehiclePresenceStore = homeVehiclePresenceStore,
            homeBenefitsVisibilityStore = homeBenefitsVisibilityStore,
            manualSyncVerificationStore = manualSyncVerificationStore,
            observeNetworkConnectivityUseCase = observeNetworkConnectivityUseCase,
            communitySummary = communitySummary,
            communitySnapshot = communitySnapshot,
        )

        assertTrue(viewModel.uiState.value.hasVehicle)
    }

    @Test
    fun cachedBenefitsVisibilityShowsBeforeFirstLoad() {
        every { homeBenefitsVisibilityStore.isVisible() } returns false
        viewModel = HomeViewModel(
            getDataReadinessUseCase = getDataReadinessUseCase,
            getMunicipalityPricesUseCase = getMunicipalityPricesUseCase,
            selectLocationUseCase = selectLocationUseCase,
            syncPriceTablesUseCase = syncPriceTablesUseCase,
            listVehiclesUseCase = listVehiclesUseCase,
            getTankFillCostEstimatesUseCase = getTankFillCostEstimatesUseCase,
            homeVehiclePresenceStore = homeVehiclePresenceStore,
            homeBenefitsVisibilityStore = homeBenefitsVisibilityStore,
            manualSyncVerificationStore = manualSyncVerificationStore,
            observeNetworkConnectivityUseCase = observeNetworkConnectivityUseCase,
            communitySummary = communitySummary,
            communitySnapshot = communitySnapshot,
        )

        assertEquals(false, viewModel.uiState.value.showBenefits)
    }

    @Test
    fun dismissBenefitsPersistsAndHides() {
        assertEquals(true, viewModel.uiState.value.showBenefits)

        viewModel.dismissBenefits()

        assertEquals(false, viewModel.uiState.value.showBenefits)
        verify { homeBenefitsVisibilityStore.setVisible(false) }
    }

    @Test
    fun loadRefreshesVehiclePresenceCache() = runTest(dispatcher) {
        stubReadyHome(
            vehicles = listOf(sampleVehicle("Gol")),
            tankItems = emptyList(),
        )

        viewModel.load(Locale.US)
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value.hasVehicle)
        coVerify { homeVehiclePresenceStore.setHasVehicle(true) }
    }

    @Test
    fun refreshWithNoNewDataKeepsStaleButMarksVerified() = runTest(dispatcher) {
        stubStaleHome(displayedWeek = surveyWeek, latestWeek = surveyWeek)
        coEvery {
            syncPriceTablesUseCase.invoke(com.anpfuel.domain.event.SyncRequestSource.MANUAL)
        } returns com.anpfuel.application.usecase.sync.SyncPriceTablesResult(
            outcome = com.anpfuel.domain.event.SyncJobOutcome.NO_NEW_DATA,
        )
        var stored: com.anpfuel.data.local.preferences.ManualSyncVerificationStore.Verification? = null
        every { manualSyncVerificationStore.get() } answers { stored }
        every {
            manualSyncVerificationStore.saveVerified(any(), any(), any())
        } answers {
            stored = com.anpfuel.data.local.preferences.ManualSyncVerificationStore.Verification(
                weekStartIso = firstArg(),
                weekEndIso = secondArg(),
                verifiedAtMillis = thirdArg(),
            )
        }

        viewModel.load(Locale.US)
        advanceUntilIdle()
        viewModel.refresh(Locale.US)
        advanceUntilIdle()

        coVerify {
            manualSyncVerificationStore.saveVerified(
                weekStartIso = surveyWeek.startDate.toString(),
                weekEndIso = surveyWeek.endDate.toString(),
                verifiedAtMillis = any(),
            )
        }
        assertEquals(
            com.anpfuel.domain.event.SyncJobOutcome.NO_NEW_DATA,
            viewModel.uiState.value.lastSyncOutcome,
        )
        assertTrue(viewModel.uiState.value.isStaleVerified)
    }

    @Test
    fun refreshWithSuccessClearsStaleVerification() = runTest(dispatcher) {
        stubReadyHome(vehicles = emptyList(), tankItems = emptyList())
        coEvery {
            syncPriceTablesUseCase.invoke(com.anpfuel.domain.event.SyncRequestSource.MANUAL)
        } returns com.anpfuel.application.usecase.sync.SyncPriceTablesResult(
            outcome = com.anpfuel.domain.event.SyncJobOutcome.SUCCESS,
        )

        viewModel.load(Locale.US)
        advanceUntilIdle()
        viewModel.refresh(Locale.US)
        advanceUntilIdle()

        coVerify { manualSyncVerificationStore.clear() }
        assertEquals(false, viewModel.uiState.value.isStaleVerified)
    }

    @Test
    fun loadHidesVerificationWhenPinnedToOldWeek() = runTest(dispatcher) {
        val oldWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
        val latestWeek = SurveyWeek.fromIsoDates("2026-06-14", "2026-06-20")
        stubStaleHome(displayedWeek = oldWeek, latestWeek = latestWeek)
        every { manualSyncVerificationStore.get() } returns com.anpfuel.data.local.preferences.ManualSyncVerificationStore.Verification(
            weekStartIso = oldWeek.startDate.toString(),
            weekEndIso = oldWeek.endDate.toString(),
            verifiedAtMillis = 123L,
        )

        viewModel.load(Locale.US)
        advanceUntilIdle()

        assertEquals(false, viewModel.uiState.value.isStaleVerified)
    }

    private fun stubStaleHome(
        displayedWeek: SurveyWeek,
        latestWeek: SurveyWeek,
        vehicles: List<Vehicle> = emptyList(),
        tankItems: List<VehicleTankFillCostEstimate> = emptyList(),
    ) {
        coEvery { getDataReadinessUseCase.invoke() } returns DataReadinessResult(
            readiness = DataReadinessState.STALE,
            hasCachedData = true,
            latestSurveyWeek = latestWeek,
            lastSummarySyncAt = null,
        )
        coEvery { selectLocationUseCase.getPreferredLocation() } returns PreferredLocation(
            municipality = municipality,
            state = state,
        )
        coEvery { listVehiclesUseCase.invoke() } returns vehicles
        coEvery { getMunicipalityPricesUseCase.invoke() } returns MunicipalityPricesResult(
            surveyWeek = displayedWeek,
            state = state,
            municipality = municipality,
            prices = emptyList(),
            dataAvailability = DataAvailability.HAS_DATA,
        )
        coEvery {
            getTankFillCostEstimatesUseCase.invoke(
                vehicles = vehicles,
                state = state,
                municipality = municipality,
                surveyWeek = displayedWeek,
            )
        } returns TankFillCostEstimatesResult(
            surveyWeek = displayedWeek,
            state = state,
            municipality = municipality,
            items = tankItems,
        )
    }

    private fun stubReadyHome(
        vehicles: List<Vehicle>,
        tankItems: List<VehicleTankFillCostEstimate>,
        prices: List<com.anpfuel.domain.model.AveragePrice> = emptyList(),
    ) {
        coEvery { getDataReadinessUseCase.invoke() } returns DataReadinessResult(
            readiness = DataReadinessState.READY,
            hasCachedData = true,
            latestSurveyWeek = surveyWeek,
            lastSummarySyncAt = null,
        )
        coEvery { selectLocationUseCase.getPreferredLocation() } returns PreferredLocation(
            municipality = municipality,
            state = state,
        )
        coEvery { listVehiclesUseCase.invoke() } returns vehicles
        coEvery { getMunicipalityPricesUseCase.invoke() } returns MunicipalityPricesResult(
            surveyWeek = surveyWeek,
            state = state,
            municipality = municipality,
            prices = prices,
            dataAvailability = DataAvailability.HAS_DATA,
        )
        coEvery {
            getTankFillCostEstimatesUseCase.invoke(
                vehicles = vehicles,
                state = state,
                municipality = municipality,
                surveyWeek = surveyWeek,
            )
        } returns TankFillCostEstimatesResult(
            surveyWeek = surveyWeek,
            state = state,
            municipality = municipality,
            items = tankItems,
        )
    }

    private fun sampleVehicle(name: String, sortOrder: Int = 0): Vehicle =
        Vehicle.create(
            displayName = name,
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
            sortOrder = sortOrder,
        )

    private fun sampleAveragePrice(product: FuelProduct, average: String?) =
        com.anpfuel.domain.model.AveragePrice.create(
            priceSurveyId = com.anpfuel.domain.valueobject.DomainId.generate(),
            surveyWeek = surveyWeek,
            state = state,
            municipality = municipality,
            fuelProduct = product,
            average = average?.let { PriceAmount.of(it) },
        )

    private fun tankEstimate(vehicle: Vehicle, total: String): TankFillCostEstimate =
        TankFillCostEstimate(
            vehicleId = vehicle.id,
            displayName = vehicle.displayName,
            tankCapacity = vehicle.tankCapacity,
            fuelProduct = vehicle.fuelProduct,
            unitPrice = PriceAmount.of("5.49"),
            totalCost = PriceAmount.of(total),
            unitPriceSource = TankFillCostUnitPriceSource.CHEAPEST_STATION,
        )
    private fun summaryCity(code:String="5103403",name:String="Synthetic city") =
        com.anpfuel.domain.community.FeedCity(code,BrazilianState.MATO_GROSSO,name)
    private fun summary(city:com.anpfuel.domain.community.FeedCity,amount:Long=4500):com.anpfuel.domain.community.CityCommunitySummary {
        val now=java.time.Instant.now()
        return com.anpfuel.domain.community.CityCommunitySummary(city,listOf(
            com.anpfuel.domain.community.CommunitySummaryPrice(FuelProduct.ETHANOL,amount,4000,5000,65,now.minusSeconds(60),now.plusSeconds(3600))),now)
    }
    @Test fun `community source uses all three statistics without ANP cache GPS or sync`()=runTest(dispatcher) {
        val city=summaryCity();coEvery {communitySummary.city()} returns city;coEvery {communitySummary(city)} returns summary(city)
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY,Locale.US);viewModel.refreshCommunity();advanceUntilIdle()
        assertEquals(HomePriceSource.COMMUNITY,viewModel.uiState.value.priceSource)
        val price=viewModel.uiState.value.communityPrices.single()
        assertEquals("R$ 4,500",price.averageFormatted);assertEquals("R$ 4,000",price.minimumFormatted);assertEquals("R$ 5,000",price.maximumFormatted);assertEquals(65,price.stationCount)
        HomePriceSortOrder.entries.forEach {viewModel.onPriceSortChanged(it);assertEquals(it,viewModel.uiState.value.priceSort);assertEquals(price,viewModel.uiState.value.communityPrices.single())}
        assertEquals("R$ 4,500",HomePriceSortOrder.DEFAULT.featuredPrice(price))
        assertEquals("R$ 4,000",HomePriceSortOrder.PRICE_ASC.featuredPrice(price))
        assertEquals("R$ 5,000",HomePriceSortOrder.PRICE_DESC.featuredPrice(price))
        assertTrue(viewModel.uiState.value.communityUpdatedAt!! < viewModel.uiState.value.communityGeneratedAt!!)
        coVerify(exactly=0){getMunicipalityPricesUseCase()};coVerify(exactly=0){syncPriceTablesUseCase(any())};coVerify(exactly=0){getDataReadinessUseCase()}
        coEvery {communitySummary(city)} throws java.io.IOException("private details")
        viewModel.refresh(Locale.US);advanceUntilIdle()
        // Failure preserves the last validated prices with an explicit stale mark.
        assertTrue(viewModel.uiState.value.communityFailed);assertTrue(viewModel.uiState.value.communityStale)
        assertEquals("R$ 4,500",viewModel.uiState.value.communityPrices.single().averageFormatted)
        assertTrue(viewModel.uiState.value.communityGeneratedAt != null)
    }
    @Test fun `late old city cannot replace current city and an old lifecycle cannot cancel the new request`()=runTest(dispatcher) {
        val first=summaryCity();val second=summaryCity("5107925","Synthetic second city")
        val delayed=kotlinx.coroutines.CompletableDeferred<com.anpfuel.domain.community.CityCommunitySummary>()
        coEvery {communitySummary.city()} returnsMany listOf(first,second)
        coEvery {communitySummary(first)} coAnswers {kotlinx.coroutines.withContext(kotlinx.coroutines.NonCancellable){delayed.await()}}
        coEvery {communitySummary(second)} returns summary(second,4900)
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY,Locale.US)
        val oldLease=viewModel.startCommunitySession();viewModel.refreshCommunity();runCurrent()
        viewModel.startCommunitySession();viewModel.refreshCommunity();viewModel.stopCommunityRefresh(oldLease);advanceUntilIdle()
        assertEquals(second,viewModel.uiState.value.communityCity);assertEquals("R$ 4,900",viewModel.uiState.value.communityPrices.single().averageFormatted)
        delayed.complete(summary(first,4100));advanceUntilIdle()
        assertEquals(second,viewModel.uiState.value.communityCity);assertEquals("R$ 4,900",viewModel.uiState.value.communityPrices.single().averageFormatted)
    }
    @Test fun `no manually selected city gives no summary call or fabricated price`()=runTest(dispatcher) {
        coEvery {communitySummary.city()} returns null
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY,Locale.US);viewModel.refreshCommunity();advanceUntilIdle()
        assertTrue(!viewModel.uiState.value.hasLocation);assertTrue(viewModel.uiState.value.communityPrices.isEmpty())
        coVerify(exactly=0){communitySummary(any())}
    }

    @Test fun `periodic refresh preserves a slow city lookup and summary until they finish`() = runTest(dispatcher) {
        val city = summaryCity()
        val selectedCity = kotlinx.coroutines.CompletableDeferred<com.anpfuel.domain.community.FeedCity>()
        val response = kotlinx.coroutines.CompletableDeferred<com.anpfuel.domain.community.CityCommunitySummary>()
        coEvery { communitySummary.city() } coAnswers { selectedCity.await() }
        coEvery { communitySummary(city) } coAnswers { response.await() }
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.refreshCommunity(periodic = true)
        runCurrent()
        testScheduler.advanceTimeBy(15_000)
        viewModel.refreshCommunity(periodic = true)
        runCurrent()
        coVerify(exactly = 1) { communitySummary.city() }
        selectedCity.complete(city)
        runCurrent()
        testScheduler.advanceTimeBy(15_000)
        viewModel.refreshCommunity(periodic = true)
        runCurrent()
        coVerify(exactly = 1) { communitySummary(city) }
        response.complete(summary(city))
        runCurrent()
        assertEquals("R$ 4,500", viewModel.uiState.value.communityPrices.single().averageFormatted)
        assertEquals(false, viewModel.uiState.value.communityLoading)
        viewModel.communityNow = { java.time.Instant.now().plusSeconds(61) }
        viewModel.refreshCommunity(periodic = true)
        runCurrent()
        coVerify(exactly = 2) { communitySummary.city() }
        coVerify(exactly = 2) { communitySummary(city) }
    }

    @Test fun `manual refresh replaces an in flight periodic request for a changed city`() = runTest(dispatcher) {
        val first = summaryCity()
        val second = summaryCity("5107925", "Synthetic second city")
        val pending = kotlinx.coroutines.CompletableDeferred<com.anpfuel.domain.community.CityCommunitySummary>()
        var cancelled = false
        coEvery { communitySummary.city() } returnsMany listOf(first, second)
        coEvery { communitySummary(first) } coAnswers {
            try { pending.await() }
            finally { cancelled = true }
        }
        coEvery { communitySummary(second) } returns summary(second, 4900)
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.refreshCommunity(periodic = true)
        runCurrent()
        viewModel.refresh(Locale.US)
        runCurrent()
        assertTrue(cancelled)
        assertEquals(second, viewModel.uiState.value.communityCity)
        assertEquals("R$ 4,900", viewModel.uiState.value.communityPrices.single().averageFormatted)
    }

    private fun fixedSummary(city: com.anpfuel.domain.community.FeedCity, generated: java.time.Instant, amount: Long = 4500) =
        com.anpfuel.domain.community.CityCommunitySummary(city, listOf(
            com.anpfuel.domain.community.CommunitySummaryPrice(FuelProduct.ETHANOL, amount, 4000, 5000, 65,
                generated.minusSeconds(60), generated.plusSeconds(3600))), generated)

    @Test fun `fresh community snapshot paints before the network answers`() = runTest(dispatcher) {
        val city = summaryCity()
        val at = java.time.Instant.parse("2026-10-06T12:00:00Z")
        viewModel.communityNow = { at }
        val pending = kotlinx.coroutines.CompletableDeferred<com.anpfuel.domain.community.CityCommunitySummary>()
        coEvery { communitySummary.city() } returns city
        coEvery { communitySnapshot.snapshotSummary(city) } returns fixedSummary(city, at)
        coEvery { communitySummary(city) } coAnswers { pending.await() }
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.refreshCommunity(); runCurrent()
        // Painted from the snapshot while the network fetch is still suspended.
        assertEquals("R$ 4,500", viewModel.uiState.value.communityPrices.single().averageFormatted)
        assertFalse(viewModel.uiState.value.communityFailed)
        assertFalse(viewModel.uiState.value.communityStale)
        pending.complete(fixedSummary(city, at.plusSeconds(61), 4900)); advanceUntilIdle()
        assertEquals("R$ 4,900", viewModel.uiState.value.communityPrices.single().averageFormatted)
        // Once the data ages out, the next periodic evaluation refreshes from the network.
        viewModel.communityNow = { at.plusSeconds(122) }
        coEvery { communitySummary(city) } returns fixedSummary(city, at.plusSeconds(122), 4950)
        viewModel.refreshCommunity(periodic = true); advanceUntilIdle()
        assertEquals("R$ 4,950", viewModel.uiState.value.communityPrices.single().averageFormatted)
        coVerify(exactly = 2) { communitySummary(city) }
    }

    @Test fun `aged community snapshot marks stale without failing`() = runTest(dispatcher) {
        val city = summaryCity()
        val at = java.time.Instant.parse("2026-10-06T12:00:00Z")
        viewModel.communityNow = { at }
        val snap = com.anpfuel.domain.community.CityCommunitySummary(city, listOf(
            com.anpfuel.domain.community.CommunitySummaryPrice(FuelProduct.ETHANOL, 4500, 4000, 5000, 65, at.minusSeconds(180), at.plusSeconds(3600))), at.minusSeconds(120))
        val pending = kotlinx.coroutines.CompletableDeferred<com.anpfuel.domain.community.CityCommunitySummary>()
        coEvery { communitySummary.city() } returns city
        coEvery { communitySnapshot.snapshotSummary(city) } returns snap
        coEvery { communitySummary(city) } coAnswers { pending.await() }
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.refreshCommunity(); runCurrent()
        assertEquals(1, viewModel.uiState.value.communityPrices.size)
        assertTrue(viewModel.uiState.value.communityStale)
        assertFalse(viewModel.uiState.value.communityFailed)
        pending.complete(summary(city, 4900))
        advanceUntilIdle()
        assertFalse(viewModel.uiState.value.communityStale)
    }

    @Test fun `offline community refresh keeps the snapshot and marks failed without a call`() = runTest(dispatcher) {
        connected.value = false
        val city = summaryCity()
        val snap = summary(city)
        coEvery { communitySummary.city() } returns city
        coEvery { communitySnapshot.snapshotSummary(city) } returns snap
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.refreshCommunity(); advanceUntilIdle()
        assertEquals(1, viewModel.uiState.value.communityPrices.size)
        assertTrue(viewModel.uiState.value.communityFailed)
        coVerify(exactly = 0) { communitySummary(city) }
    }

    @Test fun `reconnect refreshes a stale home community`() = runTest(dispatcher) {
        connected.value = false
        val city = summaryCity()
        val at = java.time.Instant.parse("2026-10-06T12:00:00Z")
        viewModel.communityNow = { at }
        coEvery { communitySummary.city() } returns city
        coEvery { communitySnapshot.snapshotSummary(city) } returns null
        coEvery { communitySummary(city) } returns summary(city, 4900)
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.startCommunitySession()
        viewModel.refreshCommunity(); advanceUntilIdle()
        assertTrue(viewModel.uiState.value.communityFailed)
        assertTrue(viewModel.uiState.value.communityPrices.isEmpty())
        viewModel.communityNow = { at.plusSeconds(61) }
        connected.value = true; advanceUntilIdle()
        assertEquals("R$ 4,900", viewModel.uiState.value.communityPrices.single().averageFormatted)
        assertFalse(viewModel.uiState.value.communityFailed)
    }

    @Test fun `community retries back off and stop after the budget until manual refresh`() = runTest(dispatcher) {
        val city = summaryCity()
        val at = java.time.Instant.parse("2026-10-06T12:00:00Z")
        viewModel.communityNow = { at }
        coEvery { communitySummary.city() } returns city
        coEvery { communitySummary(city) } throws java.io.IOException("unavailable")
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.refreshCommunity(); advanceUntilIdle()
        assertTrue(viewModel.uiState.value.communityFailed)
        coVerify(exactly = 1) { communitySummary(city) }
        var clock = at
        suspend fun tick(seconds: Long) {
            clock = clock.plusSeconds(seconds)
            val now = clock
            viewModel.communityNow = { now }
            viewModel.refreshCommunity(periodic = true); advanceUntilIdle()
        }
        tick(15); coVerify(exactly = 2) { communitySummary(city) }
        tick(30); coVerify(exactly = 3) { communitySummary(city) }
        tick(60); coVerify(exactly = 4) { communitySummary(city) }
        tick(300); coVerify(exactly = 5) { communitySummary(city) }
        tick(600); coVerify(exactly = 5) { communitySummary(city) }
        coEvery { communitySummary(city) } returns summary(city)
        viewModel.refreshCommunity(); advanceUntilIdle()
        coVerify(exactly = 6) { communitySummary(city) }
        assertFalse(viewModel.uiState.value.communityFailed)
    }

    @Test fun `community expiry clears prices while slow refresh continues and stale expiry cannot clear fresh prices`() = runTest(dispatcher) {
        val city = summaryCity("5107925", "Sorriso")
        coEvery { communitySummary.city() } returns city
        coEvery { communitySummary(city) } returns summary(city, 4500)
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.refreshCommunity()
        runCurrent()
        val expiry = requireNotNull(viewModel.uiState.value.communityExpiresAt)
        viewModel.expireCommunity(expiry.minusSeconds(1))
        assertEquals(1, viewModel.uiState.value.communityPrices.size)
        val pending = kotlinx.coroutines.CompletableDeferred<com.anpfuel.domain.community.CityCommunitySummary>()
        coEvery { communitySummary(city) } coAnswers { pending.await() }
        viewModel.refreshCommunity()
        runCurrent()
        viewModel.expireCommunity(expiry)
        assertTrue(viewModel.uiState.value.communityPrices.isEmpty())
        assertTrue(viewModel.uiState.value.communityLoading)
        pending.complete(summary(city, 4900))
        runCurrent()
        assertEquals("R$ 4,900", viewModel.uiState.value.communityPrices.single().averageFormatted)
    }

    @Test fun `scheduled community loop always yields with no city or suspended lookup`() = runTest(dispatcher) {
        val selected = kotlinx.coroutines.CompletableDeferred<com.anpfuel.domain.community.FeedCity?>()
        coEvery { communitySummary.city() } coAnswers { selected.await() }
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.startCommunitySession()
        viewModel.refreshCommunity(periodic = true); runCurrent()
        val pendingDelay = viewModel.communityAutoRefreshDelayMillis()
        selected.complete(null); runCurrent()
        val missingDelay = viewModel.communityAutoRefreshDelayMillis()
        viewModel.stopCommunityRefresh()
        assertTrue(pendingDelay >= 5_000, "pending lookup must yield")
        assertTrue(missingDelay >= 5_000, "missing city must yield")
    }

    @Test fun `returning Home retains same city memory while the city lookup is suspended`() = runTest(dispatcher) {
        val city = summaryCity()
        coEvery { communitySummary.city() } returns city
        coEvery { communitySummary(city) } returns summary(city)
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.startCommunitySession(); viewModel.refreshCommunity(); runCurrent()
        viewModel.stopCommunityRefresh()
        val selected = kotlinx.coroutines.CompletableDeferred<com.anpfuel.domain.community.FeedCity?>()
        coEvery { communitySummary.city() } coAnswers { selected.await() }
        viewModel.startCommunitySession(); viewModel.refreshCommunity(periodic = true); runCurrent()
        val retained = viewModel.uiState.value.communityPrices.size
        selected.complete(city); runCurrent()
        viewModel.stopCommunityRefresh()
        assertEquals(1, retained)
        coVerify(exactly = 1) { communitySummary(city) }
        viewModel.stopCommunityRefresh()
    }

    @Test fun `reconnecting while Home is stopped makes no community query`() = runTest(dispatcher) {
        connected.value = false
        val city = summaryCity()
        coEvery { communitySummary.city() } returns city
        coEvery { communitySummary(city) } returns summary(city)
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.startCommunitySession(); viewModel.refreshCommunity(); runCurrent()
        viewModel.stopCommunityRefresh()
        connected.value = true; runCurrent()
        coVerify(exactly = 0) { communitySummary(city) }
    }

    @Test fun `periodic evaluation retains Home failure until a successful check`() = runTest(dispatcher) {
        val city = summaryCity()
        val at = java.time.Instant.parse("2026-10-06T12:00:00Z")
        viewModel.communityNow = { at }
        coEvery { communitySummary.city() } returns city
        coEvery { communitySummary(city) } throws java.io.IOException("unavailable")
        viewModel.selectPriceSource(HomePriceSource.COMMUNITY, Locale.US)
        viewModel.startCommunitySession(); viewModel.refreshCommunity(); runCurrent()
        viewModel.communityNow = { at.plusSeconds(5) }
        viewModel.refreshCommunity(periodic = true); runCurrent()
        assertTrue(viewModel.uiState.value.communityFailed)
        coVerify(exactly = 1) { communitySummary(city) }
        viewModel.stopCommunityRefresh()
    }

}
