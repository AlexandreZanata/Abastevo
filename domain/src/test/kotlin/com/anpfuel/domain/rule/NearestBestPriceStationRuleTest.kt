package com.anpfuel.domain.rule

import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.GeoCoordinates
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class NearestBestPriceStationRuleTest {

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val deviceLocation = GeoCoordinates.of(-25.4284, -49.2733)

    private val nearCoordinates = GeoCoordinates.of(-25.4294, -49.2733)
    private val midCoordinates = GeoCoordinates.of(-25.4464, -49.2733)
    private val farCoordinates = GeoCoordinates.of(-25.5000, -49.2733)

    @Test
    fun selectCandidatesKeepsOnlyTheCheapestStations() {
        val prices = (10..90 step 10).map { cents ->
            val value = "5.${cents.toString().padStart(2, '0')}"
            stationPrice(value, cnpj = "0000000000000${cents / 10}")
        }

        val candidates = NearestBestPriceStationRule.selectCandidates(prices)

        assertEquals(NearestBestPriceStationRule.MAX_GEOCODE_CANDIDATES, candidates.size)
        assertEquals(
            listOf("5.10", "5.20", "5.30", "5.40", "5.50", "5.60", "5.70", "5.80"),
            candidates.map { it.price.value.toPlainString() },
        )
    }

    @Test
    fun selectCandidatesBreaksPriceTiesByCnpj() {
        val prices = listOf(
            stationPrice("5.10", cnpj = "00000000000002"),
            stationPrice("5.10", cnpj = "00000000000001"),
        )

        val candidates = NearestBestPriceStationRule.selectCandidates(prices)

        assertEquals(
            listOf("00000000000001", "00000000000002"),
            candidates.map { it.station.cnpj.digits },
        )
    }

    @Test
    fun selectReturnsNullWithoutCandidates() {
        assertNull(NearestBestPriceStationRule.select(emptyList(), deviceLocation))
    }

    @Test
    fun selectPrefersTheNearestStationInsideThePriceToleranceBand() {
        val cheapestButFar = candidate("5.89", midCoordinates, cnpj = "00000000000001")
        val slightlyDearerButNear = candidate("6.00", nearCoordinates, cnpj = "00000000000002")

        val recommendation = NearestBestPriceStationRule.select(
            candidates = listOf(cheapestButFar, slightlyDearerButNear),
            deviceLocation = deviceLocation,
        )

        assertEquals("00000000000002", recommendation?.station?.cnpj?.digits)
        assertEquals("6.00", recommendation?.price?.value?.toPlainString())
        assertTrue(recommendation != null && recommendation.distanceMeters in 100.0..120.0)
    }

    @Test
    fun selectKeepsTheCheapestStationWhenOthersAreOutsideTheToleranceBand() {
        val cheapestButFar = candidate("5.89", midCoordinates, cnpj = "00000000000001")
        val expensiveButNear = candidate("6.50", nearCoordinates, cnpj = "00000000000002")

        val recommendation = NearestBestPriceStationRule.select(
            candidates = listOf(cheapestButFar, expensiveButNear),
            deviceLocation = deviceLocation,
        )

        assertEquals("00000000000001", recommendation?.station?.cnpj?.digits)
        assertTrue(recommendation != null && recommendation.distanceMeters > 1_500.0)
    }

    @Test
    fun selectBreaksDistanceTiesByLowestPrice() {
        val equalDistance = listOf(
            candidate("6.00", nearCoordinates, cnpj = "00000000000001"),
            candidate("5.95", nearCoordinates, cnpj = "00000000000002"),
        )

        val recommendation = NearestBestPriceStationRule.select(equalDistance, deviceLocation)

        assertEquals("00000000000002", recommendation?.station?.cnpj?.digits)
        assertEquals("5.95", recommendation?.price?.value?.toPlainString())
    }

    @Test
    fun selectReturnsNullWhenTheOnlyCandidateIsBeyondTheSearchRadius() {
        val beyondRadius = candidate("5.89", farCoordinates, cnpj = "00000000000001")

        val recommendation = NearestBestPriceStationRule.select(
            candidates = listOf(beyondRadius),
            deviceLocation = deviceLocation,
        )

        assertNull(recommendation)
    }

    @Test
    fun selectHonoursACustomSearchRadius() {
        val fifteenKmAway = GeoCoordinates.of(-25.5628, -49.2733)
        val candidate = candidate("5.89", fifteenKmAway, cnpj = "00000000000001")

        val recommendation = NearestBestPriceStationRule.select(
            candidates = listOf(candidate),
            deviceLocation = deviceLocation,
            searchRadiusMeters = 15_000.0,
        )

        assertTrue(recommendation != null && recommendation.distanceMeters in 14_900.0..15_100.0)
    }

    @Test
    fun selectKeepsCandidateJustInsideTheSearchRadius() {
        val justInside = GeoCoordinates.of(-25.4553, -49.2733)
        val candidate = candidate("5.89", justInside, cnpj = "00000000000001")

        val recommendation = NearestBestPriceStationRule.select(
            candidates = listOf(candidate),
            deviceLocation = deviceLocation,
        )

        assertTrue(recommendation != null && recommendation.distanceMeters in 2900.0..3000.0)
    }

    private fun candidate(
        value: String,
        coordinates: GeoCoordinates,
        cnpj: String,
    ): NearestBestPriceStationRule.Candidate {
        val stationPrice = stationPrice(value, cnpj)
        return NearestBestPriceStationRule.Candidate(
            station = stationPrice.station,
            price = stationPrice.price,
            coordinates = coordinates,
        )
    }

    private fun stationPrice(value: String, cnpj: String): StationPrice = StationPrice.create(
        priceSurveyId = DomainId.forSurveyWeek(surveyWeek),
        surveyWeek = surveyWeek,
        station = station(cnpj),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        price = PriceAmount.of(value),
    )

    private fun station(cnpj: String): RetailStation = RetailStation.create(
        cnpj = Cnpj.parse(cnpj),
        legalName = "POSTO $cnpj",
        tradeName = "POSTO $cnpj",
        address = "RUA XV DE NOVEMBRO, 1000",
        municipality = "Curitiba",
        state = BrazilianState.PARANA,
        brand = "BR",
    )
}
