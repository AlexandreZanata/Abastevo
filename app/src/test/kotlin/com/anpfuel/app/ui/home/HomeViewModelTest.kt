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
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
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
    private val observeNetworkConnectivityUseCase = mockk<ObserveNetworkConnectivityUseCase>()

    private val communitySummary = mockk<com.anpfuel.application.usecase.community.GetCityCommunitySummaryUseCase>()
    private lateinit var viewModel: HomeViewModel

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val state = BrazilianState.PARANA
    private val municipality = "Curitiba"

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        every { observeNetworkConnectivityUseCase.invoke() } returns flowOf(true)
        every { homeVehiclePresenceStore.hasVehicle() } returns false
        every { homeVehiclePresenceStore.setHasVehicle(any()) } returns Unit
        every { homeBenefitsVisibilityStore.isVisible() } returns true
        every { homeBenefitsVisibilityStore.setVisible(any()) } returns Unit
        viewModel = HomeViewModel(
            getDataReadinessUseCase = getDataReadinessUseCase,
            getMunicipalityPricesUseCase = getMunicipalityPricesUseCase,
            selectLocationUseCase = selectLocationUseCase,
            syncPriceTablesUseCase = syncPriceTablesUseCase,
            listVehiclesUseCase = listVehiclesUseCase,
            getTankFillCostEstimatesUseCase = getTankFillCostEstimatesUseCase,
            homeVehiclePresenceStore = homeVehiclePresenceStore,
            homeBenefitsVisibilityStore = homeBenefitsVisibilityStore,
            observeNetworkConnectivityUseCase = observeNetworkConnectivityUseCase,
            communitySummary = communitySummary,
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
            observeNetworkConnectivityUseCase = observeNetworkConnectivityUseCase,
            communitySummary = communitySummary,
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
            observeNetworkConnectivityUseCase = observeNetworkConnectivityUseCase,
            communitySummary = communitySummary,
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
        assertTrue(viewModel.uiState.value.communityFailed);assertTrue(viewModel.uiState.value.communityPrices.isEmpty())
        assertEquals(null,viewModel.uiState.value.communityGeneratedAt)
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

}
