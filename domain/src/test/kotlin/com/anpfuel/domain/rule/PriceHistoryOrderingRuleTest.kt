package com.anpfuel.domain.rule

import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class PriceHistoryOrderingRuleTest {

    @Test
    fun ordersEntriesBySurveyWeekStartDateDescending() {
        val weekA = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
        val weekB = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
        val weekC = SurveyWeek.fromIsoDates("2026-06-14", "2026-06-20")

        val unordered = listOf(
            averagePrice(weekC),
            averagePrice(weekA),
            averagePrice(weekB),
        )

        val ordered = PriceHistoryOrderingRule.orderBySurveyWeekStartDate(unordered)

        assertEquals(listOf(weekC, weekB, weekA), ordered.map { it.surveyWeek })
    }

    private fun averagePrice(surveyWeek: SurveyWeek): AveragePrice = AveragePrice.create(
        priceSurveyId = DomainId.forSurveyWeek(surveyWeek),
        surveyWeek = surveyWeek,
        state = BrazilianState.SAO_PAULO,
        municipality = "São Paulo",
        fuelProduct = FuelProduct.ETHANOL,
    )
}
