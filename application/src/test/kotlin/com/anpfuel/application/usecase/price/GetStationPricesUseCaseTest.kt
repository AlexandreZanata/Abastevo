package com.anpfuel.application.usecase.price

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class GetStationPricesUseCaseTest {

    private val stationPriceRepository = mockk<StationPriceRepository>()
    private val priceTableRepository = mockk<PriceTableRepository>()
    private val userPreferencesRepository = mockk<UserPreferencesRepository>()

    private lateinit var useCase: GetStationPricesUseCase

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val state = BrazilianState.SAO_PAULO
    private val municipality = "São Paulo"
    private val fuelProduct = FuelProduct.GASOLINE_REGULAR
    private val station = RetailStation.create(
        cnpj = Cnpj.parse("61602199002409"),
        legalName = "Posto Legal",
        tradeName = "Posto Centro",
        address = "Rua A",
        municipality = municipality,
        state = state,
        brand = "BR",
    )

    @BeforeEach
    fun setUp() {
        useCase = GetStationPricesUseCase(
            stationPriceRepository = stationPriceRepository,
            priceTableRepository = priceTableRepository,
            userPreferencesRepository = userPreferencesRepository,
        )

        coEvery { priceTableRepository.countImportedSurveyWeeks() } returns 1
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns listOf(
            PriceSurvey.restore(
                id = DomainId.forSurveyWeek(surveyWeek),
                surveyWeek = surveyWeek,
                summaryImportedAt = java.time.Instant.parse("2026-06-14T10:00:00Z"),
                stationImportedAt = null,
            ),
        )
        coEvery { priceTableRepository.findPriceSurveyByWeek(surveyWeek) } returns null
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = state,
            preferredMunicipality = municipality,
            preferredFuelProduct = fuelProduct,
        )
    }

    @Test
    fun br008ReturnsTypedErrorWhenStationDetailMissingAndOptedOut() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = state,
            preferredMunicipality = municipality,
            preferredFuelProduct = fuelProduct,
            syncStationDetail = false,
        )
        coEvery {
            stationPriceRepository.hasStationData(surveyWeek, state, municipality)
        } returns false

        val outcome = useCase.invoke()

        val missing = assertInstanceOf(StationPricesOutcome.StationDetailMissing::class.java, outcome)
        assertEquals(AppError.StationDetailNotSynced, missing.error)
        assertTrue(missing.requiresOnDemandDownload)
        coVerify(exactly = 0) {
            stationPriceRepository.getStationPrices(any(), any(), any(), any())
        }
    }

    @Test
    fun sortsStationPricesByAscendingPrice() = runTest {
        coEvery {
            stationPriceRepository.hasStationData(surveyWeek, state, municipality)
        } returns true
        coEvery {
            stationPriceRepository.getStationPrices(state, municipality, fuelProduct, surveyWeek)
        } returns listOf(
            stationPrice("5.99"),
            stationPrice("5.19"),
            stationPrice("5.49"),
        )

        val outcome = useCase.invoke() as StationPricesOutcome.Success

        assertEquals(listOf("5.19", "5.49", "5.99"), outcome.stations.map { it.price.value.toPlainString() })
    }

    @Test
    fun br010EmptyFuelResultIsSuccessNotError() = runTest {
        coEvery {
            stationPriceRepository.hasStationData(surveyWeek, state, municipality)
        } returns true
        coEvery {
            stationPriceRepository.getStationPrices(state, municipality, fuelProduct, surveyWeek)
        } returns emptyList()

        val outcome = useCase.invoke() as StationPricesOutcome.Success

        assertTrue(outcome.isEmpty)
    }

    @Test
    fun returnsEmptySuccessWhenStationDetailImportedButMunicipalityHasNoStations() = runTest {
        coEvery { priceTableRepository.findPriceSurveyByWeek(surveyWeek) } returns PriceSurvey.restore(
            id = DomainId.forSurveyWeek(surveyWeek),
            surveyWeek = surveyWeek,
            summaryImportedAt = java.time.Instant.parse("2026-06-14T10:00:00Z"),
            stationImportedAt = java.time.Instant.parse("2026-06-14T10:05:00Z"),
        )
        coEvery {
            stationPriceRepository.hasStationData(surveyWeek, state, municipality)
        } returns false

        val outcome = useCase.invoke() as StationPricesOutcome.Success

        assertTrue(outcome.isEmpty)
        coVerify(exactly = 0) {
            stationPriceRepository.getStationPrices(any(), any(), any(), any())
        }
    }

    @Test
    fun doesNotRequireOnDemandDownloadWhenAutoSyncEnabled() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = state,
            preferredMunicipality = municipality,
            preferredFuelProduct = fuelProduct,
            syncStationDetail = true,
        )
        coEvery {
            stationPriceRepository.hasStationData(surveyWeek, state, municipality)
        } returns false

        val outcome = useCase.invoke() as StationPricesOutcome.StationDetailMissing

        assertEquals(false, outcome.requiresOnDemandDownload)
    }

    private fun stationPrice(value: String): StationPrice = StationPrice.create(
        priceSurveyId = DomainId.forSurveyWeek(surveyWeek),
        surveyWeek = surveyWeek,
        station = station,
        fuelProduct = fuelProduct,
        price = PriceAmount.of(value),
    )
}
