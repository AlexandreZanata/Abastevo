package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.model.TankFillCostUnitPriceSource
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
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.math.BigDecimal

class TankFillCostRuleTest {

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val priceSurveyId = DomainId.forSurveyWeek(surveyWeek)

    @Test
    fun multiplyUnitPriceByTankCapacityWithTwoDecimalRounding() {
        val total = TankFillCostRule.multiply(
            unitPrice = PriceAmount.of("5.499"),
            capacity = TankCapacity.of(50.0),
        )

        assertEquals(BigDecimal("275.00"), total.value)
    }

    @Test
    fun cheapestStationModeUsesMinimumStationPrice() {
        val vehicle = vehicle(
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
            capacity = 50.0,
        )
        val estimate = TankFillCostRule.estimate(
            vehicle = vehicle,
            context = TankFillCostRule.PriceContext(
                stationPrices = listOf(
                    stationPrice(FuelProduct.GASOLINE_REGULAR, "5.99", "Expensive"),
                    stationPrice(FuelProduct.GASOLINE_REGULAR, "5.49", "Cheap"),
                ),
                averagePrice = null,
            ),
        )

        assertEquals(PriceAmount.of("5.49"), estimate?.unitPrice)
        assertEquals(PriceAmount.of("274.50"), estimate?.totalCost)
        assertEquals(TankFillCostUnitPriceSource.CHEAPEST_STATION, estimate?.unitPriceSource)
        assertEquals("Cheap", estimate?.stationDisplayName)
    }

    @Test
    fun cheapestStationModeFallsBackToAverageMinimum() {
        val vehicle = vehicle(
            fuelProduct = FuelProduct.ETHANOL,
            priceSource = VehiclePriceSource.cheapest(),
            capacity = 40.0,
        )
        val estimate = TankFillCostRule.estimate(
            vehicle = vehicle,
            context = TankFillCostRule.PriceContext(
                stationPrices = emptyList(),
                averagePrice = averagePrice(minimum = "3.10"),
            ),
        )

        assertEquals(PriceAmount.of("3.10"), estimate?.unitPrice)
        assertEquals(TankFillCostUnitPriceSource.AVERAGE_MINIMUM, estimate?.unitPriceSource)
        assertNull(estimate?.stationDisplayName)
    }

    @Test
    fun specificStationModeUsesMatchingCnpjPrice() {
        val cnpj = Cnpj.parse("12345678000195")
        val vehicle = vehicle(
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.specific(cnpj),
            capacity = 45.0,
        )
        val estimate = TankFillCostRule.estimate(
            vehicle = vehicle,
            context = TankFillCostRule.PriceContext(
                stationPrices = listOf(
                    stationPrice(
                        fuelProduct = FuelProduct.GASOLINE_REGULAR,
                        price = "5.79",
                        tradeName = "Chosen",
                        cnpj = cnpj,
                    ),
                    stationPrice(FuelProduct.GASOLINE_REGULAR, "5.49", "Cheaper"),
                ),
                averagePrice = null,
            ),
        )

        assertEquals(PriceAmount.of("5.79"), estimate?.unitPrice)
        assertEquals(TankFillCostUnitPriceSource.SPECIFIC_STATION, estimate?.unitPriceSource)
        assertEquals("Chosen", estimate?.stationDisplayName)
    }

    @Test
    fun returnsNullWhenNoPriceDataExists() {
        val vehicle = vehicle(
            fuelProduct = FuelProduct.DIESEL_S10,
            priceSource = VehiclePriceSource.cheapest(),
            capacity = 60.0,
        )

        assertNull(
            TankFillCostRule.estimate(
                vehicle = vehicle,
                context = TankFillCostRule.PriceContext(
                    stationPrices = emptyList(),
                    averagePrice = null,
                ),
            ),
        )
    }

    @Test
    fun ignoresStationsForOtherFuelProducts() {
        val vehicle = vehicle(
            fuelProduct = FuelProduct.ETHANOL,
            priceSource = VehiclePriceSource.cheapest(),
            capacity = 50.0,
        )
        val estimate = TankFillCostRule.estimate(
            vehicle = vehicle,
            context = TankFillCostRule.PriceContext(
                stationPrices = listOf(
                    stationPrice(FuelProduct.GASOLINE_REGULAR, "5.49", "Gas"),
                ),
                averagePrice = averagePrice(minimum = "3.20"),
            ),
        )

        assertTrue(estimate != null)
        assertEquals(PriceAmount.of("3.20"), estimate?.unitPrice)
    }

    private fun vehicle(
        fuelProduct: FuelProduct,
        priceSource: VehiclePriceSource,
        capacity: Double,
    ): Vehicle = Vehicle.create(
        displayName = "Test car",
        tankCapacity = TankCapacity.of(capacity),
        fuelProduct = fuelProduct,
        priceSource = priceSource,
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
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = "BR",
        ),
        fuelProduct = fuelProduct,
        price = PriceAmount.of(price),
    )

    private fun averagePrice(minimum: String): AveragePrice = AveragePrice.create(
        priceSurveyId = priceSurveyId,
        surveyWeek = surveyWeek,
        state = BrazilianState.PARANA,
        municipality = "CURITIBA",
        fuelProduct = FuelProduct.ETHANOL,
        geographicScope = GeographicScope.MUNICIPALITY,
        minimum = PriceAmount.of(minimum),
    )
}
