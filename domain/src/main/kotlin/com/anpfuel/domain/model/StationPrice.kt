package com.anpfuel.domain.model

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import java.time.LocalDate

/**
 * Price charged by a [RetailStation] for a [FuelProduct] on a collection date.
 */
class StationPrice private constructor(
    val id: DomainId,
    val priceSurveyId: DomainId,
    val surveyWeek: SurveyWeek,
    val station: RetailStation,
    val fuelProduct: FuelProduct,
    val price: PriceAmount,
    val collectedAt: LocalDate?,
) {
    fun isCheaperThan(other: StationPrice): Boolean =
        price.value.compareTo(other.price.value) < 0

    fun matchesLocation(state: BrazilianState, municipality: String): Boolean =
        station.matchesLocation(state, municipality)

    fun isForProduct(product: FuelProduct): Boolean = fuelProduct == product

    companion object {
        fun create(
            priceSurveyId: DomainId,
            surveyWeek: SurveyWeek,
            station: RetailStation,
            fuelProduct: FuelProduct,
            price: PriceAmount,
            collectedAt: LocalDate? = null,
            id: DomainId = DomainId.generate(),
        ): StationPrice = StationPrice(
            id = id,
            priceSurveyId = priceSurveyId,
            surveyWeek = surveyWeek,
            station = station,
            fuelProduct = fuelProduct,
            price = price,
            collectedAt = collectedAt,
        )
    }
}
