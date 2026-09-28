package com.anpfuel.domain.rule

import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.GeographicScope
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PriceDropDetectionRuleTest {

    private val currentWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val previousWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
    private val currentSurveyId = DomainId.forSurveyWeek(currentWeek)
    private val previousSurveyId = DomainId.forSurveyWeek(previousWeek)
    private val cnpj = Cnpj.parse("12345678000195")

    @Test
    fun shouldNotifyWhenCurrentPriceIsLowerThanPrevious() {
        assertTrue(
            PriceDropDetectionRule.shouldNotify(
                currentPrice = PriceAmount.of("5.40"),
                previousPrice = PriceAmount.of("5.60"),
            ),
        )
    }

    @Test
    fun shouldNotNotifyWhenPricesAreEqual() {
        assertFalse(
            PriceDropDetectionRule.shouldNotify(
                currentPrice = PriceAmount.of("5.40"),
                previousPrice = PriceAmount.of("5.40"),
            ),
        )
    }

    @Test
    fun shouldNotNotifyWhenPreviousPriceMissing() {
        assertFalse(
            PriceDropDetectionRule.shouldNotify(
                currentPrice = PriceAmount.of("5.40"),
                previousPrice = null,
            ),
        )
    }

    @Test
    fun cheapestStationModeComparesMinimumStationPrices() {
        val vehicle = vehicle(VehiclePriceSource.cheapest())
        val (current, previous) = PriceDropDetectionRule.resolveComparisonPrices(
            vehicle = vehicle,
            currentWeek = weekContext(
                surveyWeek = currentWeek,
                surveyId = currentSurveyId,
                stationPrices = listOf(station(currentSurveyId, currentWeek, "5.40")),
                minimum = "5.50",
            ),
            previousWeek = weekContext(
                surveyWeek = previousWeek,
                surveyId = previousSurveyId,
                stationPrices = listOf(station(previousSurveyId, previousWeek, "5.60")),
                minimum = "5.70",
            ),
        )

        assertEquals(PriceAmount.of("5.40"), current)
        assertEquals(PriceAmount.of("5.60"), previous)
        assertTrue(PriceDropDetectionRule.shouldNotify(current, previous))
    }

    @Test
    fun specificStationModeComparesSelectedCnpjPrices() {
        val vehicle = vehicle(VehiclePriceSource.specific(cnpj))
        val (current, previous) = PriceDropDetectionRule.resolveComparisonPrices(
            vehicle = vehicle,
            currentWeek = weekContext(
                surveyWeek = currentWeek,
                surveyId = currentSurveyId,
                stationPrices = listOf(
                    station(currentSurveyId, currentWeek, "5.30", cnpj),
                    station(currentSurveyId, currentWeek, "5.10", Cnpj.parse("61602199002409")),
                ),
                minimum = "5.50",
            ),
            previousWeek = weekContext(
                surveyWeek = previousWeek,
                surveyId = previousSurveyId,
                stationPrices = listOf(
                    station(previousSurveyId, previousWeek, "5.45", cnpj),
                ),
                minimum = "5.70",
            ),
        )

        assertEquals(PriceAmount.of("5.30"), current)
        assertEquals(PriceAmount.of("5.45"), previous)
    }

    private fun vehicle(priceSource: VehiclePriceSource): Vehicle = Vehicle.create(
        displayName = "Test",
        tankCapacity = TankCapacity.of(50.0),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        priceSource = priceSource,
        priceDropAlertEnabled = true,
    )

    private fun weekContext(
        surveyWeek: SurveyWeek,
        surveyId: DomainId,
        stationPrices: List<StationPrice>,
        minimum: String,
    ): PriceDropDetectionRule.WeekPriceContext = PriceDropDetectionRule.WeekPriceContext(
        stationPrices = stationPrices,
        averagePrice = AveragePrice.create(
            priceSurveyId = surveyId,
            surveyWeek = surveyWeek,
            state = BrazilianState.PARANA,
            municipality = "CURITIBA",
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            geographicScope = GeographicScope.MUNICIPALITY,
            minimum = PriceAmount.of(minimum),
        ),
    )

    private fun station(
        surveyId: DomainId,
        surveyWeek: SurveyWeek,
        price: String,
        stationCnpj: Cnpj = cnpj,
    ): StationPrice = StationPrice.create(
        priceSurveyId = surveyId,
        surveyWeek = surveyWeek,
        station = RetailStation.create(
            cnpj = stationCnpj,
            legalName = "POSTO",
            tradeName = "POSTO",
            address = "RUA A",
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = "BR",
        ),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        price = PriceAmount.of(price),
    )
}
