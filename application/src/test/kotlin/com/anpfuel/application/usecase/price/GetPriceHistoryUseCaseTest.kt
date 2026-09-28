package com.anpfuel.application.usecase.price

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class GetPriceHistoryUseCaseTest {

    private val averagePriceRepository = mockk<AveragePriceRepository>()
    private val priceTableRepository = mockk<PriceTableRepository>()
    private val userPreferencesRepository = mockk<UserPreferencesRepository>()

    private lateinit var useCase: GetPriceHistoryUseCase

    private val state = BrazilianState.SAO_PAULO
    private val municipality = "São Paulo"
    private val fuelProduct = FuelProduct.ETHANOL
    private val weekA = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
    private val weekB = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val weekC = SurveyWeek.fromIsoDates("2026-06-14", "2026-06-20")

    @BeforeEach
    fun setUp() {
        useCase = GetPriceHistoryUseCase(
            averagePriceRepository = averagePriceRepository,
            priceTableRepository = priceTableRepository,
            userPreferencesRepository = userPreferencesRepository,
        )

        coEvery { priceTableRepository.countImportedSurveyWeeks() } returns 2
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = state,
            preferredMunicipality = municipality,
            preferredFuelProduct = fuelProduct,
            showPriceHistory = true,
        )
    }

    @Test
    fun returnsOrderedHistoryWhenAtLeastTwoWeeksExist() = runTest {
        val unorderedHistory = listOf(
            averagePrice(weekC),
            averagePrice(weekA),
            averagePrice(weekB),
        )
        coEvery {
            averagePriceRepository.getPriceHistory(state, municipality, fuelProduct)
        } returns unorderedHistory

        val outcome = useCase.invoke()

        val success = assertInstanceOf(PriceHistoryOutcome.Success::class.java, outcome)
        assertEquals(listOf(weekC, weekB, weekA), success.entries.map { it.surveyWeek })
    }

    @Test
    fun returnsInsufficientDataWhenOnlyOneWeekAvailable() = runTest {
        coEvery {
            averagePriceRepository.getPriceHistory(state, municipality, fuelProduct)
        } returns listOf(averagePrice(weekA), averagePrice(weekA))

        val outcome = useCase.invoke()

        assertEquals(PriceHistoryOutcome.InsufficientData, outcome)
    }

    @Test
    fun returnsHistoryDisabledWhenPreferenceIsOff() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = state,
            preferredMunicipality = municipality,
            preferredFuelProduct = fuelProduct,
            showPriceHistory = false,
        )

        val outcome = useCase.invoke()

        assertEquals(PriceHistoryOutcome.HistoryDisabled, outcome)
        coVerify(exactly = 0) {
            averagePriceRepository.getPriceHistory(any(), any(), any())
        }
    }

    @Test
    fun rejectsWhenNoImportedData() = runTest {
        coEvery { priceTableRepository.countImportedSurveyWeeks() } returns 0

        assertThrows<DomainException> {
            useCase.invoke()
        }
    }

    @Test
    fun acceptsExplicitLocationAndFuelProduct() = runTest {
        coEvery {
            averagePriceRepository.getPriceHistory(
                BrazilianState.PARANA,
                "Curitiba",
                FuelProduct.DIESEL_S10,
            )
        } returns listOf(
            averagePrice(weekA, BrazilianState.PARANA, "Curitiba", FuelProduct.DIESEL_S10),
            averagePrice(weekB, BrazilianState.PARANA, "Curitiba", FuelProduct.DIESEL_S10),
        )

        val outcome = useCase.invoke(
            fuelProduct = FuelProduct.DIESEL_S10,
            state = BrazilianState.PARANA,
            municipality = "Curitiba",
        )

        assertInstanceOf(PriceHistoryOutcome.Success::class.java, outcome)
    }

    private fun averagePrice(
        surveyWeek: SurveyWeek,
        state: BrazilianState = this.state,
        municipality: String = this.municipality,
        fuelProduct: FuelProduct = this.fuelProduct,
    ): AveragePrice = AveragePrice.create(
        priceSurveyId = DomainId.forSurveyWeek(surveyWeek),
        surveyWeek = surveyWeek,
        state = state,
        municipality = municipality,
        fuelProduct = fuelProduct,
    )
}
