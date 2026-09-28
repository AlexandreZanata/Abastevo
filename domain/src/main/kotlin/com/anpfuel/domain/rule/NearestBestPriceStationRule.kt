package com.anpfuel.domain.rule

import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.valueobject.GeoCoordinates
import com.anpfuel.domain.valueobject.PriceAmount
import java.math.BigDecimal
import java.math.RoundingMode

/**
 * BR-028 — Recommends the cheapest station close to the user.
 *
 * Candidates are limited to the cheapest [MAX_GEOCODE_CANDIDATES] stations so that
 * geocoding stays bounded (BR-021). Within the price tolerance band the nearest
 * candidate wins; outside the band price decides. Candidates farther than
 * [MAX_SEARCH_RADIUS_METERS] from the device are never recommended.
 */
object NearestBestPriceStationRule {

    /**
     * Upper bound of stations resolved to coordinates per user request (BR-021 — no bulk geocoding).
     */
    const val MAX_GEOCODE_CANDIDATES = 8

    /**
     * Relative price tolerance: candidates priced up to this ratio above the cheapest
     * compete on distance instead of price.
     */
    const val PRICE_TOLERANCE_RATIO = 0.02

    /**
     * UC-015 / BR-028 — default hard search radius: candidates beyond this distance from
     * the device are never recommended, even when they are the cheapest available. The
     * effective radius is user-configurable (3/5/10/15 km, see
     * [com.anpfuel.domain.model.UserPreferences]).
     */
    const val MAX_SEARCH_RADIUS_METERS = 3_000.0

    data class Candidate(
        val station: RetailStation,
        val price: PriceAmount,
        val coordinates: GeoCoordinates,
    )

    data class Recommendation(
        val station: RetailStation,
        val price: PriceAmount,
        val distanceMeters: Double,
    )

    /**
     * Selects the cheapest stations eligible for geocoding, ordered by ascending price
     * with CNPJ as a stable tie-breaker.
     */
    fun selectCandidates(stationPrices: List<StationPrice>): List<StationPrice> =
        stationPrices
            .sortedWith(compareBy({ it.price.value }, { it.station.cnpj.digits }))
            .take(MAX_GEOCODE_CANDIDATES)

    /**
     * Picks the recommended candidate for [deviceLocation], or `null` when no candidate
     * exists or every candidate lies beyond [searchRadiusMeters].
     */
    fun select(
        candidates: List<Candidate>,
        deviceLocation: GeoCoordinates,
        searchRadiusMeters: Double = MAX_SEARCH_RADIUS_METERS,
    ): Recommendation? {
        val cheapest = candidates.minByOrNull { it.price.value } ?: return null
        val toleranceFloor = cheapest.price.value
            .multiply(BigDecimal.ONE.add(BigDecimal.valueOf(PRICE_TOLERANCE_RATIO)))
            .setScale(2, RoundingMode.HALF_UP)

        return candidates
            .filter { it.price.value <= toleranceFloor }
            .map { candidate ->
                candidate to GeoDistanceRule.distanceMeters(deviceLocation, candidate.coordinates)
            }
            .filter { (_, distanceMeters) -> distanceMeters <= searchRadiusMeters }
            .minWithOrNull(compareBy({ it.second }, { it.first.price.value }))
            ?.let { (candidate, distanceMeters) ->
                Recommendation(
                    station = candidate.station,
                    price = candidate.price,
                    distanceMeters = distanceMeters,
                )
            }
    }
}
