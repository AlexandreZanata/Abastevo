package com.anpfuel.domain.rule

import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class StationPriceOrderingRuleTest {

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val station = RetailStation.create(
        cnpj = Cnpj.parse("61602199002409"),
        legalName = "COMPANHIA ULTRAGAZ S A",
        tradeName = "ULTRAGAZ",
        address = "RUA AMARO CASTRO LIMA",
        municipality = "São Paulo",
        state = BrazilianState.SAO_PAULO,
        brand = "ULTRAGAZ",
    )

    @Test
    fun ordersByAscendingPrice() {
        val expensive = stationPrice("5.99")
        val cheapest = stationPrice("5.19")
        val middle = stationPrice("5.49")

        val ordered = StationPriceOrderingRule.orderByPriceAscending(
            listOf(expensive, cheapest, middle),
        )

        assertEquals(listOf("5.19", "5.49", "5.99"), ordered.map { it.price.value.toPlainString() })
    }

    private fun stationPrice(value: String): StationPrice = StationPrice.create(
        priceSurveyId = DomainId.forSurveyWeek(surveyWeek),
        surveyWeek = surveyWeek,
        station = station,
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        price = PriceAmount.of(value),
    )
}
