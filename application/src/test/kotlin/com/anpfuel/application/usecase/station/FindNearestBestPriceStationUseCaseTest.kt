package com.anpfuel.application.usecase.station

import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.price.StationPricesOutcome
import com.anpfuel.domain.event.StationNavigationRequested
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.AddressGeocodeOutcome
import com.anpfuel.domain.repository.AddressGeocodeRepository
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.GeocodeRequest
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.NearestBestPriceStationRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.GeoCoordinates
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class FindNearestBestPriceStationUseCaseTest {

    private val getStationPricesUseCase = mockk<GetStationPricesUseCase>()
    private val addressGeocodeRepository = mockk<AddressGeocodeRepository>()
    private val userPreferencesRepository = mockk<UserPreferencesRepository>()
    private val eventPublisher = mockk<DomainEventPublisher>()

    private lateinit var useCase: FindNearestBestPriceStationUseCase

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val deviceLocation = GeoCoordinates.of(-25.4284, -49.2733)
    private val nearCoordinates = GeoCoordinates.of(-25.4294, -49.2733)
    private val midCoordinates = GeoCoordinates.of(-25.4464, -49.2733)
    private val farCoordinates = GeoCoordinates.of(-25.5000, -49.2733)

    @BeforeEach
    fun setUp() {
        useCase = FindNearestBestPriceStationUseCase(
            getStationPricesUseCase = getStationPricesUseCase,
            addressGeocodeRepository = addressGeocodeRepository,
            buildStationNavigationQueryUseCase = BuildStationNavigationQueryUseCase(
                userPreferencesRepository = userPreferencesRepository,
                eventPublisher = eventPublisher,
            ),
            userPreferencesRepository = userPreferencesRepository,
        )
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()
        coEvery { eventPublisher.publish(any()) } returns Unit
    }

    @Test
    fun recommendsNearestStationInsidePriceToleranceAndPublishesNavigation() = runTest {
        stubStationPrices(
            stationPrice("5.89", tradeName = "POSTO A", address = "RUA A, 100"),
            stationPrice("6.00", tradeName = "POSTO B", address = "RUA B, 200"),
        )
        coEvery { addressGeocodeRepository.geocode(match<List<GeocodeRequest>> { it.containsFreeText("RUA A, 100") }) } returns
            AddressGeocodeOutcome.Success(midCoordinates)
        coEvery { addressGeocodeRepository.geocode(match<List<GeocodeRequest>> { it.containsFreeText("RUA B, 200") }) } returns
            AddressGeocodeOutcome.Success(nearCoordinates)
        val eventSlot = slot<StationNavigationRequested>()

        val outcome = useCase(FuelProduct.GASOLINE_REGULAR, deviceLocation)

        assertTrue(outcome is FindNearestStationOutcome.Success)
        val success = outcome as FindNearestStationOutcome.Success
        assertEquals("POSTO B", success.station.displayName())
        assertEquals("6.00", success.price.value.toPlainString())
        assertTrue(success.distanceMeters in 100.0..120.0, "Expected ~111 m, got ${success.distanceMeters}")
        assertEquals("POSTO B, RUA B, 200, Curitiba - PR, Brazil", success.navigationQuery)

        coVerify(exactly = 1) { eventPublisher.publish(capture(eventSlot)) }
        assertEquals(success.navigationQuery, eventSlot.captured.payload.navigationQuery)
    }

    @Test
    fun recommendsCheapestStationWhenOthersAreOutsideTolerance() = runTest {
        stubStationPrices(
            stationPrice("5.89", tradeName = "POSTO A", address = "RUA A, 100"),
            stationPrice("6.50", tradeName = "POSTO B", address = "RUA B, 200"),
        )
        coEvery { addressGeocodeRepository.geocode(any<List<GeocodeRequest>>()) } returns
            AddressGeocodeOutcome.Success(nearCoordinates)

        val outcome = useCase(FuelProduct.GASOLINE_REGULAR, GeoCoordinates.of(-25.4290, -49.2733))

        assertTrue(outcome is FindNearestStationOutcome.Success)
        assertEquals("POSTO A", (outcome as FindNearestStationOutcome.Success).station.displayName())
    }

    @Test
    fun geocodesAtMostTheCheapestCandidates() = runTest {
        val stations = (1..10).map { index ->
            stationPrice(
                value = "5.${index.toString().padStart(2, '0')}",
                tradeName = "POSTO $index",
                address = "RUA $index, $index",
                cnpj = "000000000000${index.toString().padStart(2, '0')}",
            )
        }
        stubStationPrices(*stations.toTypedArray())
        coEvery { addressGeocodeRepository.geocode(any<List<GeocodeRequest>>()) } returns
            AddressGeocodeOutcome.Success(nearCoordinates)

        useCase(FuelProduct.GASOLINE_REGULAR, deviceLocation)

        coVerify(exactly = NearestBestPriceStationRule.MAX_GEOCODE_CANDIDATES) {
            addressGeocodeRepository.geocode(any<List<GeocodeRequest>>())
        }
    }

    @Test
    fun abortsImmediatelyOnNetworkError() = runTest {
        stubStationPrices(
            stationPrice("5.89", tradeName = "POSTO A", address = "RUA A, 100"),
            stationPrice("6.00", tradeName = "POSTO B", address = "RUA B, 200"),
        )
        coEvery { addressGeocodeRepository.geocode(any<List<GeocodeRequest>>()) } returns AddressGeocodeOutcome.NetworkError

        val outcome = useCase(FuelProduct.GASOLINE_REGULAR, deviceLocation)

        assertEquals(FindNearestStationOutcome.GeocodingFailed, outcome)
        coVerify(exactly = 1) { addressGeocodeRepository.geocode(any<List<GeocodeRequest>>()) }
    }

    @Test
    fun returnsNoStationWithinRadiusWhenEveryCandidateIsBeyondTheRadius() = runTest {
        stubStationPrices(stationPrice("5.89", tradeName = "POSTO A", address = "RUA A, 100"))
        coEvery { addressGeocodeRepository.geocode(any<List<GeocodeRequest>>()) } returns
            AddressGeocodeOutcome.Success(farCoordinates)

        val outcome = useCase(FuelProduct.GASOLINE_REGULAR, deviceLocation)

        assertEquals(FindNearestStationOutcome.NoStationWithinRadius, outcome)
    }

    @Test
    fun returnsSuccessWhenCandidateIsInsideTheConfiguredRadius() = runTest {
        stubStationPrices(stationPrice("5.89", tradeName = "POSTO A", address = "RUA A, 100"))
        coEvery { addressGeocodeRepository.geocode(any<List<GeocodeRequest>>()) } returns
            AddressGeocodeOutcome.Success(farCoordinates)
        coEvery { userPreferencesRepository.getPreferences() } returns
            UserPreferences(nearestStationRadiusKm = 15)

        val outcome = useCase(FuelProduct.GASOLINE_REGULAR, deviceLocation)

        assertTrue(outcome is FindNearestStationOutcome.Success)
        assertEquals("POSTO A", (outcome as FindNearestStationOutcome.Success).station.displayName())
    }

    @Test
    fun returnsGeocodingFailedWhenNoAddressResolves() = runTest {
        stubStationPrices(stationPrice("5.89", tradeName = "POSTO A", address = "RUA A, 100"))
        coEvery { addressGeocodeRepository.geocode(any<List<GeocodeRequest>>()) } returns AddressGeocodeOutcome.NotFound

        val outcome = useCase(FuelProduct.GASOLINE_REGULAR, deviceLocation)

        assertEquals(FindNearestStationOutcome.GeocodingFailed, outcome)
    }

    @Test
    fun returnsNoStationsWhenListIsEmpty() = runTest {
        stubStationPrices()

        val outcome = useCase(FuelProduct.GASOLINE_REGULAR, deviceLocation)

        assertEquals(FindNearestStationOutcome.NoStations, outcome)
    }

    @Test
    fun returnsStationDetailMissingWhenStationDataWasNotImported() = runTest {
        coEvery { getStationPricesUseCase(fuelProduct = FuelProduct.GASOLINE_REGULAR) } returns
            StationPricesOutcome.StationDetailMissing(requiresOnDemandDownload = true)

        val outcome = useCase(FuelProduct.GASOLINE_REGULAR, deviceLocation)

        assertEquals(FindNearestStationOutcome.StationDetailMissing, outcome)
    }

    private fun List<GeocodeRequest>.containsFreeText(snippet: String): Boolean =
        any { it is GeocodeRequest.FreeText && it.query.contains(snippet) }

    private fun stubStationPrices(vararg stationPrices: StationPrice) {
        coEvery { getStationPricesUseCase(fuelProduct = FuelProduct.GASOLINE_REGULAR) } returns
            StationPricesOutcome.Success(
                surveyWeek = surveyWeek,
                state = BrazilianState.PARANA,
                municipality = "Curitiba",
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                stations = stationPrices.toList(),
            )
    }

    private fun stationPrice(
        value: String,
        tradeName: String,
        address: String,
        cnpj: String = "61602199002409",
    ): StationPrice = StationPrice.create(
        priceSurveyId = DomainId.forSurveyWeek(surveyWeek),
        surveyWeek = surveyWeek,
        station = RetailStation.create(
            cnpj = Cnpj.parse(cnpj),
            legalName = tradeName,
            tradeName = tradeName,
            address = address,
            municipality = "Curitiba",
            state = BrazilianState.PARANA,
            brand = "BR",
        ),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        price = PriceAmount.of(value),
    )
}
