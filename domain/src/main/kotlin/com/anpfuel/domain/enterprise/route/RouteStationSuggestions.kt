package com.anpfuel.domain.enterprise.route

import java.math.BigInteger

/*
 * Neutral detour-cost and comparable fuel-station price evaluation along a
 * planned route (EMP-ROUTE-008, BR-ROUTE-020..022, section 10.7).
 * Uses exact BigInteger half-up scaling and keeps commercial status strictly out
 * of math and ranking.
 */

enum class SuggestionPriceSource {
    COMMUNITY, OFFICIAL_ANP, STATION_ANNOUNCED;

    companion object {
        fun parse(raw: String): SuggestionPriceSource =
            entries.firstOrNull { it.name == raw } ?: throw RouteDomainException()
    }
}

enum class SuggestionIncomparableReason {
    NO_CANDIDATE_PRICE,
    NO_REFERENCE_PRICE,
    SOURCE_KIND_MISMATCH,
    PRODUCT_MISMATCH,
    UNIT_MISMATCH,
    PAYMENT_CONDITION_MISMATCH,
    STALE_PRICE,
}

data class SuggestionPriceReference(
    val sourceKind: SuggestionPriceSource,
    val product: String,
    val unit: String,
    val paymentCondition: String,
    val priceMilliBrl: Long,
    val ageSeconds: Long,
) {
    init {
        if (product.isBlank() || unit.isBlank() || paymentCondition.isBlank() || priceMilliBrl <= 0L || ageSeconds < 0L) {
            throw RouteDomainException()
        }
    }

    override fun toString(): String = "SuggestionPriceReference(kind=$sourceKind,product=$product)"
}

data class AssetConsumptionBasis(
    val distanceMetres: Long,
    val volumeMilliLitres: Long,
) {
    init {
        if (distanceMetres <= 0L || volumeMilliLitres <= 0L) throw RouteDomainException()
    }
}

data class RouteStationCandidate(
    val stationId: String,
    val stationName: String,
    val price: SuggestionPriceReference?,
    val routeTotalMetres: Long,
    val routeTotalSeconds: Long,
    val hasPaidPlan: Boolean = false,
    val hasActiveAgreement: Boolean = false,
) {
    override fun toString(): String = "RouteStationCandidate(id=$stationId)"
}

data class RouteStationSuggestion(
    val stationId: String,
    val stationName: String,
    val price: SuggestionPriceReference?,
    val detourMetres: Long,
    val detourSeconds: Long,
    val detourFuelMilliLitres: Long?,
    val detourCostCentavos: Long?,
    val referenceCostCentavos: Long?,
    val candidateCostCentavos: Long?,
    val grossDiffCentavos: Long?,
    val netDiffCentavos: Long?,
    val comparable: Boolean,
    val incomparableReason: SuggestionIncomparableReason?,
    val detourExceedsDiff: Boolean,
)

object RouteStationSuggestions {
    private val TWO = BigInteger.valueOf(2L)
    private val FIVE_THOUSAND = BigInteger.valueOf(5_000L)
    private val TEN_THOUSAND = BigInteger.valueOf(10_000L)

    private fun priceCostCentavos(volumeMl: Long, priceMilliBrl: Long): Long =
        BigInteger.valueOf(volumeMl)
            .multiply(BigInteger.valueOf(priceMilliBrl))
            .add(FIVE_THOUSAND)
            .divide(TEN_THOUSAND)
            .longValueExact()

    private fun detourFuelMl(detourMetres: Long, basis: AssetConsumptionBasis): Long {
        val dist = BigInteger.valueOf(basis.distanceMetres)
        return BigInteger.valueOf(detourMetres)
            .multiply(BigInteger.valueOf(basis.volumeMilliLitres))
            .add(dist.divide(TWO))
            .divide(dist)
            .longValueExact()
    }

    fun evaluate(
        product: String,
        plannedVolumeMl: Long,
        baseMetres: Long,
        baseSeconds: Long,
        consumption: AssetConsumptionBasis?,
        freshnessLimitSeconds: Long,
        reference: SuggestionPriceReference?,
        candidate: RouteStationCandidate,
    ): RouteStationSuggestion {
        if (product.isBlank() || plannedVolumeMl <= 0L || baseMetres < 0L || baseSeconds < 0L ||
            freshnessLimitSeconds <= 0L || candidate.routeTotalMetres < baseMetres || candidate.routeTotalSeconds < baseSeconds
        ) {
            throw RouteDomainException()
        }
        val detourM = candidate.routeTotalMetres - baseMetres
        val detourS = candidate.routeTotalSeconds - baseSeconds

        if (candidate.price == null) {
            return RouteStationSuggestion(
                stationId = candidate.stationId,
                stationName = candidate.stationName,
                price = null,
                detourMetres = detourM,
                detourSeconds = detourS,
                detourFuelMilliLitres = null,
                detourCostCentavos = null,
                referenceCostCentavos = null,
                candidateCostCentavos = null,
                grossDiffCentavos = null,
                netDiffCentavos = null,
                comparable = false,
                incomparableReason = SuggestionIncomparableReason.NO_CANDIDATE_PRICE,
                detourExceedsDiff = false,
            )
        }

        val df = consumption?.let { detourFuelMl(detourM, it) }
        val dc = df?.let { priceCostCentavos(it, candidate.price.priceMilliBrl) }
        val candCost = priceCostCentavos(plannedVolumeMl, candidate.price.priceMilliBrl)
        val refCost = reference?.let { priceCostCentavos(plannedVolumeMl, it.priceMilliBrl) }

        val mismatch = when {
            reference == null -> SuggestionIncomparableReason.NO_REFERENCE_PRICE
            reference.sourceKind != candidate.price.sourceKind -> SuggestionIncomparableReason.SOURCE_KIND_MISMATCH
            reference.product != candidate.price.product || candidate.price.product != product ->
                SuggestionIncomparableReason.PRODUCT_MISMATCH
            reference.unit != candidate.price.unit -> SuggestionIncomparableReason.UNIT_MISMATCH
            reference.paymentCondition != candidate.price.paymentCondition ->
                SuggestionIncomparableReason.PAYMENT_CONDITION_MISMATCH
            reference.ageSeconds > freshnessLimitSeconds || candidate.price.ageSeconds > freshnessLimitSeconds ->
                SuggestionIncomparableReason.STALE_PRICE
            else -> null
        }

        if (mismatch != null) {
            return RouteStationSuggestion(
                stationId = candidate.stationId,
                stationName = candidate.stationName,
                price = candidate.price,
                detourMetres = detourM,
                detourSeconds = detourS,
                detourFuelMilliLitres = df,
                detourCostCentavos = dc,
                referenceCostCentavos = refCost,
                candidateCostCentavos = candCost,
                grossDiffCentavos = null,
                netDiffCentavos = null,
                comparable = false,
                incomparableReason = mismatch,
                detourExceedsDiff = false,
            )
        }

        val gross = refCost!! - candCost
        val net = dc?.let { gross - it }
        val detourExceeds = dc != null && dc > gross

        return RouteStationSuggestion(
            stationId = candidate.stationId,
            stationName = candidate.stationName,
            price = candidate.price,
            detourMetres = detourM,
            detourSeconds = detourS,
            detourFuelMilliLitres = df,
            detourCostCentavos = dc,
            referenceCostCentavos = refCost,
            candidateCostCentavos = candCost,
            grossDiffCentavos = gross,
            netDiffCentavos = net,
            comparable = true,
            incomparableReason = null,
            detourExceedsDiff = detourExceeds,
        )
    }

    /** Neutral ranking: paid-plan/agreement status is never part of comparator inputs (BR-ROUTE-022). */
    fun rank(items: List<RouteStationSuggestion>): List<RouteStationSuggestion> = items.sortedWith { a, b ->
        when {
            a.comparable != b.comparable -> if (a.comparable) -1 else 1
            a.netDiffCentavos != null && b.netDiffCentavos != null && a.netDiffCentavos != b.netDiffCentavos ->
                b.netDiffCentavos.compareTo(a.netDiffCentavos)
            a.comparable && b.comparable && a.grossDiffCentavos != null && b.grossDiffCentavos != null && a.grossDiffCentavos != b.grossDiffCentavos ->
                b.grossDiffCentavos.compareTo(a.grossDiffCentavos)
            a.detourMetres != b.detourMetres -> a.detourMetres.compareTo(b.detourMetres)
            a.price != null && b.price != null && a.price.priceMilliBrl != b.price.priceMilliBrl ->
                a.price.priceMilliBrl.compareTo(b.price.priceMilliBrl)
            else -> a.stationId.compareTo(b.stationId)
        }
    }
}
