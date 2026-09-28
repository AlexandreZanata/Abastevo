package com.anpfuel.domain.model

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.time.LocalDate

class StationPriceTest {

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val priceSurveyId = DomainId.forSurveyWeek(surveyWeek)
    private val station = RetailStation.create(
        cnpj = Cnpj.parse("61602199002409"),
        legalName = "COMPANHIA ULTRAGAZ S A",
        tradeName = "ULTRAGAZ",
        address = "RUA AMARO CASTRO LIMA",
        municipality = "CAMPO GRANDE",
        state = BrazilianState.MATO_GROSSO_DO_SUL,
        brand = "ULTRAGAZ",
    )

    @Test
    fun createGeneratesUniqueDomainIds() {
        val first = createStationPrice(price = PriceAmount.of("125.00"))
        val second = createStationPrice(price = PriceAmount.of("126.00"))

        assertNotEquals(first.id, second.id)
    }

    @Test
    fun isCheaperThanComparesPriceAmounts() {
        val cheaper = createStationPrice(price = PriceAmount.of("5.49"))
        val expensive = createStationPrice(price = PriceAmount.of("5.99"))

        assertTrue(cheaper.isCheaperThan(expensive))
        assertFalse(expensive.isCheaperThan(cheaper))
    }

    @Test
    fun matchesLocationThroughRetailStation() {
        val stationPrice = createStationPrice(price = PriceAmount.of("125.00"))

        assertTrue(stationPrice.matchesLocation(BrazilianState.MATO_GROSSO_DO_SUL, "CAMPO GRANDE"))
        assertFalse(stationPrice.matchesLocation(BrazilianState.SAO_PAULO, "CAMPO GRANDE"))
    }

    @Test
    fun retainsCollectionDateWhenProvided() {
        val stationPrice = createStationPrice(
            price = PriceAmount.of("125.00"),
            collectedAt = LocalDate.parse("2026-06-08"),
        )

        assertTrue(stationPrice.isForProduct(FuelProduct.LPG_P13))
        assertTrue(stationPrice.collectedAt == LocalDate.parse("2026-06-08"))
    }

    private fun createStationPrice(
        price: PriceAmount,
        collectedAt: LocalDate? = null,
    ): StationPrice = StationPrice.create(
        priceSurveyId = priceSurveyId,
        surveyWeek = surveyWeek,
        station = station,
        fuelProduct = FuelProduct.LPG_P13,
        price = price,
        collectedAt = collectedAt,
    )
}
