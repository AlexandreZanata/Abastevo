package com.anpfuel.application.usecase.price

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.EmptyMunicipalityResultRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class GetMunicipalityPricesUseCaseTest {

    private val averagePriceRepository = mockk<AveragePriceRepository>()
    private val municipalityCatalogRepository = mockk<MunicipalityCatalogRepository>()
    private val priceTableRepository = mockk<PriceTableRepository>()
    private val userPreferencesRepository = mockk<UserPreferencesRepository>()

    private lateinit var useCase: GetMunicipalityPricesUseCase

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val state = BrazilianState.SAO_PAULO
    private val municipality = "São Paulo"

    @BeforeEach
    fun setUp() {
        useCase = GetMunicipalityPricesUseCase(
            averagePriceRepository = averagePriceRepository,
            municipalityCatalogRepository = municipalityCatalogRepository,
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
        coEvery { averagePriceRepository.getLatestImportedSurveyWeek() } returns surveyWeek
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = state,
            preferredMunicipality = municipality,
        )
        coEvery { municipalityCatalogRepository.getOperationalNote(any()) } returns null
    }

    @Test
    fun br006FallsBackToLatestImportedSurveyWeekWhenNoActivePreference() = runTest {
        val prices = listOf(createAveragePrice(FuelProduct.ETHANOL))
        coEvery {
            averagePriceRepository.getPricesByMunicipality(state, municipality, surveyWeek)
        } returns prices

        val result = useCase.invoke()

        assertEquals(surveyWeek, result.surveyWeek)
        assertEquals(prices, result.prices)
        assertEquals(DataAvailability.HAS_DATA, result.dataAvailability)
        coVerify(exactly = 1) { priceTableRepository.getImportedPriceSurveys() }
        coVerify(exactly = 0) { averagePriceRepository.getLatestImportedSurveyWeek() }
    }

    @Test
    fun br019UsesActiveSurveyWeekWhenImportedLocally() = runTest {
        val olderWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
        val latestWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = state,
            preferredMunicipality = municipality,
            activeSurveyWeek = olderWeek,
        )
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns listOf(
            PriceSurvey.restore(
                id = DomainId.forSurveyWeek(olderWeek),
                surveyWeek = olderWeek,
                summaryImportedAt = java.time.Instant.parse("2026-06-07T10:00:00Z"),
                stationImportedAt = null,
            ),
            PriceSurvey.restore(
                id = DomainId.forSurveyWeek(latestWeek),
                surveyWeek = latestWeek,
                summaryImportedAt = java.time.Instant.parse("2026-06-14T10:00:00Z"),
                stationImportedAt = null,
            ),
        )
        val prices = listOf(createAveragePrice(FuelProduct.ETHANOL, week = olderWeek))
        coEvery {
            averagePriceRepository.getPricesByMunicipality(state, municipality, olderWeek)
        } returns prices

        val result = useCase.invoke()

        assertEquals(olderWeek, result.surveyWeek)
        assertEquals(prices, result.prices)
    }

    @Test
    fun br010EmptyMunicipalityReturnsEmptyListNotError() = runTest {
        coEvery {
            averagePriceRepository.getPricesByMunicipality(state, municipality, surveyWeek)
        } returns emptyList()
        coEvery {
            municipalityCatalogRepository.resolveDataAvailability(state, municipality, surveyWeek)
        } returns DataAvailability.NO_DATA_THIS_WEEK

        val result = useCase.invoke()

        assertTrue(result.isEmpty)
        assertTrue(result.prices.isEmpty())
        assertEquals(DataAvailability.NO_DATA_THIS_WEEK, result.dataAvailability)
        assertFalse(EmptyMunicipalityResultRule.isError(result.prices.size))
    }

    @Test
    fun br010DistinguishesNeverInAnpFromNoDataThisWeek() = runTest {
        coEvery {
            averagePriceRepository.getPricesByMunicipality(
                BrazilianState.ACRE,
                "ACRELÂNDIA",
                surveyWeek,
            )
        } returns emptyList()
        coEvery {
            municipalityCatalogRepository.resolveDataAvailability(
                BrazilianState.ACRE,
                "ACRELÂNDIA",
                surveyWeek,
            )
        } returns DataAvailability.NEVER_IN_ANP

        val result = useCase.invoke(
            state = BrazilianState.ACRE,
            municipality = "ACRELÂNDIA",
        )

        assertTrue(result.isEmpty)
        assertEquals(DataAvailability.NEVER_IN_ANP, result.dataAvailability)
        assertNull(result.operationalNote)
    }

    @Test
    fun br004ReadsPricesFromLocalCacheOnly() = runTest {
        val cachedPrices = listOf(
            createAveragePrice(FuelProduct.GASOLINE_REGULAR, PriceAmount.of("5.89")),
            createAveragePrice(FuelProduct.ETHANOL, PriceAmount.of("3.42")),
        )
        coEvery {
            averagePriceRepository.getPricesByMunicipality(state, municipality, surveyWeek)
        } returns cachedPrices

        val result = useCase.invoke()

        assertEquals(cachedPrices, result.prices)
        coVerify(exactly = 1) {
            averagePriceRepository.getPricesByMunicipality(state, municipality, surveyWeek)
        }
    }

    @Test
    fun acceptsExplicitLocationAndSurveyWeek() = runTest {
        val olderWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
        val explicitPrices = listOf(createAveragePrice(FuelProduct.DIESEL_S10, week = olderWeek))
        coEvery {
            averagePriceRepository.getPricesByMunicipality(
                BrazilianState.PARANA,
                "Curitiba",
                olderWeek,
            )
        } returns explicitPrices

        val result = useCase.invoke(
            state = BrazilianState.PARANA,
            municipality = "Curitiba",
            surveyWeek = olderWeek,
        )

        assertEquals(olderWeek, result.surveyWeek)
        assertEquals(explicitPrices, result.prices)
        assertEquals(DataAvailability.HAS_DATA, result.dataAvailability)
        coVerify(exactly = 0) { averagePriceRepository.getLatestImportedSurveyWeek() }
    }

    @Test
    fun rejectsWhenNoImportedData() = runTest {
        coEvery { priceTableRepository.countImportedSurveyWeeks() } returns 0

        assertThrows<DomainException> {
            useCase.invoke()
        }
    }

    @Test
    fun rejectsWhenPreferredLocationIsMissing() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()

        assertThrows<DomainException> {
            useCase.invoke()
        }
    }

    private fun createAveragePrice(
        fuelProduct: FuelProduct,
        average: PriceAmount? = null,
        week: SurveyWeek = surveyWeek,
    ): AveragePrice = AveragePrice.create(
        priceSurveyId = DomainId.forSurveyWeek(week),
        surveyWeek = week,
        state = state,
        municipality = municipality,
        fuelProduct = fuelProduct,
        average = average,
    )
}
