package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.GeographicScope
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Statistical summary for a [FuelProduct] at a [GeographicScope].
 */
class AveragePrice private constructor(
    val id: DomainId,
    val priceSurveyId: DomainId,
    val surveyWeek: SurveyWeek,
    val state: BrazilianState,
    val municipality: String,
    val fuelProduct: FuelProduct,
    val geographicScope: GeographicScope,
    val stationCount: Int?,
    val unit: String?,
    val average: PriceAmount?,
    val minimum: PriceAmount?,
    val maximum: PriceAmount?,
    val standardDeviation: PriceAmount?,
) {
    init {
        if (municipality.isBlank()) {
            throw DomainException("AveragePrice municipality must not be blank")
        }
        stationCount?.let { count ->
            if (count < 0) {
                throw DomainException("AveragePrice stationCount must be non-negative")
            }
        }
    }

    fun isForProduct(product: FuelProduct): Boolean = fuelProduct == product

    fun matchesLocation(state: BrazilianState, municipality: String): Boolean =
        this.state == state && this.municipality.equals(municipality, ignoreCase = true)

    fun hasPriceStatistics(): Boolean = average != null || minimum != null || maximum != null

    companion object {
        fun create(
            priceSurveyId: DomainId,
            surveyWeek: SurveyWeek,
            state: BrazilianState,
            municipality: String,
            fuelProduct: FuelProduct,
            geographicScope: GeographicScope = GeographicScope.MUNICIPALITY,
            stationCount: Int? = null,
            unit: String? = null,
            average: PriceAmount? = null,
            minimum: PriceAmount? = null,
            maximum: PriceAmount? = null,
            standardDeviation: PriceAmount? = null,
            id: DomainId = DomainId.generate(),
        ): AveragePrice = AveragePrice(
            id = id,
            priceSurveyId = priceSurveyId,
            surveyWeek = surveyWeek,
            state = state,
            municipality = municipality.trim(),
            fuelProduct = fuelProduct,
            geographicScope = geographicScope,
            stationCount = stationCount,
            unit = unit?.trim(),
            average = average,
            minimum = minimum,
            maximum = maximum,
            standardDeviation = standardDeviation,
        )
    }
}
