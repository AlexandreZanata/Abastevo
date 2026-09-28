package com.anpfuel.domain.model

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.GeographicScope
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class AveragePriceTest {

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val priceSurveyId = DomainId.forSurveyWeek(surveyWeek)

    @Test
    fun createGeneratesUniqueDomainIds() {
        val first = createAveragePrice()
        val second = createAveragePrice()

        assertNotEquals(first.id, second.id)
    }

    @Test
    fun isForProductMatchesFuelProduct() {
        val averagePrice = createAveragePrice(fuelProduct = FuelProduct.ETHANOL)

        assertTrue(averagePrice.isForProduct(FuelProduct.ETHANOL))
        assertFalse(averagePrice.isForProduct(FuelProduct.CNG))
    }

    @Test
    fun matchesLocationUsesStateAndMunicipality() {
        val averagePrice = createAveragePrice(
            state = BrazilianState.SAO_PAULO,
            municipality = "ADAMANTINA",
        )

        assertTrue(averagePrice.matchesLocation(BrazilianState.SAO_PAULO, "adamantina"))
        assertFalse(averagePrice.matchesLocation(BrazilianState.PARANA, "ADAMANTINA"))
    }

    @Test
    fun hasPriceStatisticsWhenAnyPriceFieldIsPresent() {
        val withAverage = createAveragePrice(average = PriceAmount.of("3.42"))
        val withoutStatistics = createAveragePrice()

        assertTrue(withAverage.hasPriceStatistics())
        assertFalse(withoutStatistics.hasPriceStatistics())
    }

    @Test
    fun linksSurveyWeekStateMunicipalityAndFuelProduct() {
        val averagePrice = createAveragePrice(
            state = BrazilianState.SAO_PAULO,
            municipality = "ADAMANTINA",
            fuelProduct = FuelProduct.ETHANOL,
        )

        assertEquals(surveyWeek, averagePrice.surveyWeek)
        assertEquals(BrazilianState.SAO_PAULO, averagePrice.state)
        assertEquals("ADAMANTINA", averagePrice.municipality)
        assertEquals(FuelProduct.ETHANOL, averagePrice.fuelProduct)
        assertEquals(GeographicScope.MUNICIPALITY, averagePrice.geographicScope)
        assertEquals(priceSurveyId, averagePrice.priceSurveyId)
    }

    private fun createAveragePrice(
        state: BrazilianState = BrazilianState.SAO_PAULO,
        municipality: String = "ADAMANTINA",
        fuelProduct: FuelProduct = FuelProduct.GASOLINE_REGULAR,
        average: PriceAmount? = null,
    ): AveragePrice = AveragePrice.create(
        priceSurveyId = priceSurveyId,
        surveyWeek = surveyWeek,
        state = state,
        municipality = municipality,
        fuelProduct = fuelProduct,
        average = average,
    )
}
