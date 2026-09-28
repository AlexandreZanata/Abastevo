package com.anpfuel.domain.rule

import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PriceHistoryAvailabilityRuleTest {

    @Test
    fun requiresAtLeastTwoDistinctSurveyWeeks() {
        val weekA = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
        val weekB = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

        val singleWeekHistory = listOf(
            averagePrice(weekA),
            averagePrice(weekA),
        )
        val twoWeekHistory = listOf(
            averagePrice(weekA),
            averagePrice(weekB),
        )

        assertFalse(PriceHistoryAvailabilityRule.hasSufficientHistory(singleWeekHistory))
        assertTrue(PriceHistoryAvailabilityRule.hasSufficientHistory(twoWeekHistory))
        assertEquals(2, PriceHistoryAvailabilityRule.distinctSurveyWeekCount(twoWeekHistory))
    }

    private fun averagePrice(surveyWeek: SurveyWeek): AveragePrice = AveragePrice.create(
        priceSurveyId = DomainId.forSurveyWeek(surveyWeek),
        surveyWeek = surveyWeek,
        state = BrazilianState.SAO_PAULO,
        municipality = "São Paulo",
        fuelProduct = FuelProduct.ETHANOL,
    )
}
