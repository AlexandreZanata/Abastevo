package com.anpfuel.application.usecase.vehicle

import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.model.TankFillCostUnitPriceSource
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.GeographicScope
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class GetTankFillCostEstimatesUseCaseTest {

    private val averagePriceRepository = mockk<AveragePriceRepository>()
    private val stationPriceRepository = mockk<StationPriceRepository>()
    private val priceTableRepository = mockk<PriceTableRepository>()
    private val userPreferencesRepository = mockk<UserPreferencesRepository>()

    private lateinit var useCase: GetTankFillCostEstimatesUseCase

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val priceSurveyId = DomainId.forSurveyWeek(surveyWeek)
    private val state = BrazilianState.PARANA
    private val municipality = "Curitiba"

    @BeforeEach
    fun setUp() {
        useCase = GetTankFillCostEstimatesUseCase(
            averagePriceRepository = averagePriceRepository,
            stationPriceRepository = stationPriceRepository,
            priceTableRepository = priceTableRepository,
            userPreferencesRepository = userPreferencesRepository,
        )

        coEvery { priceTableRepository.countImportedSurveyWeeks() } returns 1
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns listOf(
            PriceSurvey.restore(
                id = priceSurveyId,
                surveyWeek = surveyWeek,
                summaryImportedAt = java.time.Instant.parse("2026-06-14T10:00:00Z"),
                stationImportedAt = java.time.Instant.parse("2026-06-14T11:00:00Z"),
            ),
        )
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = state,
            preferredMunicipality = municipality,
        )
        coEvery {
            averagePriceRepository.getPricesByMunicipality(state, municipality, surveyWeek)
        } returns emptyList()
        coEvery {
            stationPriceRepository.hasStationData(surveyWeek, state, municipality)
        } returns true
    }

    @Test
    fun emptyVehiclesReturnsEmptyItems() = runTest {
        val result = useCase.invoke(vehicles = emptyList())

        assertTrue(result.items.isEmpty())
        coVerify(exactly = 0) { averagePriceRepository.getPricesByMunicipality(any(), any(), any()) }
    }

    @Test
    fun cheapestStationModeUsesMinimumStationPrice() = runTest {
        val vehicle = vehicle(
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
            capacity = 50.0,
            displayName = "Gol",
        )
        coEvery {
            stationPriceRepository.getStationPrices(
                state = state,
                municipality = municipality,
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                surveyWeek = surveyWeek,
            )
        } returns listOf(
            stationPrice(FuelProduct.GASOLINE_REGULAR, "5.99", "Expensive"),
            stationPrice(FuelProduct.GASOLINE_REGULAR, "5.49", "Cheap"),
        )

        val result = useCase.invoke(
            vehicles = listOf(vehicle),
            state = state,
            municipality = municipality,
            surveyWeek = surveyWeek,
        )

        assertEquals(1, result.items.size)
        val estimate = result.items.single().estimate
        assertEquals(PriceAmount.of("5.49"), estimate?.unitPrice)
        assertEquals(PriceAmount.of("274.50"), estimate?.totalCost)
        assertEquals(TankFillCostUnitPriceSource.CHEAPEST_STATION, estimate?.unitPriceSource)
        assertEquals("Cheap", estimate?.stationDisplayName)
    }

    @Test
    fun fallsBackToAverageMinimumWhenStationDataMissing() = runTest {
        val vehicle = vehicle(
            fuelProduct = FuelProduct.ETHANOL,
            priceSource = VehiclePriceSource.cheapest(),
            capacity = 40.0,
            displayName = "Onix",
        )
        coEvery {
            stationPriceRepository.hasStationData(surveyWeek, state, municipality)
        } returns false
        coEvery {
            averagePriceRepository.getPricesByMunicipality(state, municipality, surveyWeek)
        } returns listOf(
            averagePrice(FuelProduct.ETHANOL, minimum = "3.10"),
        )

        val result = useCase.invoke(
            vehicles = listOf(vehicle),
            state = state,
            municipality = municipality,
            surveyWeek = surveyWeek,
        )

        val estimate = result.items.single().estimate
        assertEquals(PriceAmount.of("3.10"), estimate?.unitPrice)
        assertEquals(TankFillCostUnitPriceSource.AVERAGE_MINIMUM, estimate?.unitPriceSource)
        coVerify(exactly = 0) {
            stationPriceRepository.getStationPrices(any(), any(), any(), any())
        }
    }

    @Test
    fun returnsUnavailableEstimateWhenNoPriceDataExists() = runTest {
        val vehicle = vehicle(
            fuelProduct = FuelProduct.DIESEL_S10,
            priceSource = VehiclePriceSource.cheapest(),
            capacity = 60.0,
            displayName = "S10 Truck",
        )
        coEvery {
            stationPriceRepository.getStationPrices(
                state = state,
                municipality = municipality,
                fuelProduct = FuelProduct.DIESEL_S10,
                surveyWeek = surveyWeek,
            )
        } returns emptyList()

        val result = useCase.invoke(
            vehicles = listOf(vehicle),
            state = state,
            municipality = municipality,
            surveyWeek = surveyWeek,
        )

        assertNull(result.items.single().estimate)
    }

    @Test
    fun preservesVehicleSortOrder() = runTest {
        val first = vehicle(
            displayName = "First",
            sortOrder = 0,
            fuelProduct = FuelProduct.ETHANOL,
            priceSource = VehiclePriceSource.cheapest(),
            capacity = 40.0,
        )
        val second = vehicle(
            displayName = "Second",
            sortOrder = 1,
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
            capacity = 50.0,
        )
        coEvery {
            stationPriceRepository.getStationPrices(
                state = state,
                municipality = municipality,
                fuelProduct = FuelProduct.ETHANOL,
                surveyWeek = surveyWeek,
            )
        } returns emptyList()
        coEvery {
            averagePriceRepository.getPricesByMunicipality(state, municipality, surveyWeek)
        } returns listOf(averagePrice(FuelProduct.ETHANOL, minimum = "3.10"))
        coEvery {
            stationPriceRepository.getStationPrices(
                state = state,
                municipality = municipality,
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                surveyWeek = surveyWeek,
            )
        } returns listOf(stationPrice(FuelProduct.GASOLINE_REGULAR, "5.49", "Cheap"))

        val result = useCase.invoke(
            vehicles = listOf(second, first),
            state = state,
            municipality = municipality,
            surveyWeek = surveyWeek,
        )

        assertEquals(listOf("First", "Second"), result.items.map { it.vehicle.displayName })
    }

    private fun vehicle(
        fuelProduct: FuelProduct,
        priceSource: VehiclePriceSource,
        capacity: Double,
        displayName: String,
        sortOrder: Int = 0,
    ): Vehicle = Vehicle.create(
        displayName = displayName,
        tankCapacity = TankCapacity.of(capacity),
        fuelProduct = fuelProduct,
        priceSource = priceSource,
        sortOrder = sortOrder,
    )

    private fun stationPrice(
        fuelProduct: FuelProduct,
        price: String,
        tradeName: String,
        cnpj: Cnpj = Cnpj.parse("61602199002409"),
    ): StationPrice = StationPrice.create(
        priceSurveyId = priceSurveyId,
        surveyWeek = surveyWeek,
        station = RetailStation.create(
            cnpj = cnpj,
            legalName = tradeName,
            tradeName = tradeName,
            address = "RUA A",
            municipality = municipality,
            state = state,
            brand = "BR",
        ),
        fuelProduct = fuelProduct,
        price = PriceAmount.of(price),
    )

    private fun averagePrice(
        fuelProduct: FuelProduct,
        minimum: String,
    ): AveragePrice = AveragePrice.create(
        priceSurveyId = priceSurveyId,
        surveyWeek = surveyWeek,
        state = state,
        municipality = municipality,
        fuelProduct = fuelProduct,
        geographicScope = GeographicScope.MUNICIPALITY,
        minimum = PriceAmount.of(minimum),
    )
}
