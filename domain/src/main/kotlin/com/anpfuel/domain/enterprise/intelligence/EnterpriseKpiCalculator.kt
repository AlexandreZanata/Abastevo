package com.anpfuel.domain.enterprise.intelligence

import java.math.BigInteger

/**
 * EMP-KPI-001..006 & EMP-FIND-001..005: Exact ratio-of-sums consumption,
 * availability, non-judgmental price comparisons, FT-001..FT-006 detection,
 * and self-resolve prevention matching Go domain logic.
 */
object EnterpriseKpiCalculator {
    const val KPI_VERSION = "kpi-v1"
    const val PRICE_METHODOLOGY_VERSION = "price-comparison-v1"
    const val FINDINGS_METHOD_VERSION = "findings-v1"

    enum class InsufficiencyReason {
        NO_CLOSED_INTERVAL,
        NO_DISTANCE_BASIS,
        NO_HOUR_BASIS,
        METHOD_UNSUPPORTED,
        NO_AVAILABILITY_HISTORY,
        NO_REFERENCE,
        PLACE_UNKNOWN,
        REFERENCE_STALE,
        LOW_COVERAGE,
        CONDITION_UNKNOWN,
        CONDITION_MISMATCH,
        ZERO_DENOMINATOR,
        METHOD_MISMATCH,
        ZERO_BASELINE,
    }

    enum class FindingType(val code: String) {
        FT_001("FT-001"),
        FT_002("FT-002"),
        FT_003("FT-003"),
        FT_004("FT-004"),
        FT_005("FT-005"),
        FT_006("FT-006"),
    }

    enum class FindingSeverity { LOW, MEDIUM, HIGH }

    enum class FindingState { OPEN, IN_REVIEW, CONFIRMED, EXPLAINED, FALSE_POSITIVE, SUPERSEDED }

    enum class DecisionError { FINDING_CLOSED, SELF_APPROVAL, JUSTIFICATION_REQUIRED }

    data class RatioInterval(
        val product: String = "DIESEL_S10",
        val distanceM: Long = 0L,
        val hourSeconds: Long = 0L,
        val quantityMilli: Long = 0L,
        val costCentavos: Long = 0L,
    )

    data class ConsumptionSummary(
        val status: String,
        val reason: InsufficiencyReason? = null,
        val distanceM: Long = 0L,
        val hourSeconds: Long = 0L,
        val quantityMilli: Long = 0L,
        val costCentavos: Long = 0L,
        val kmPerUnitRat: String = "",
        val kmPerUnit2Dp: String = "",
        val litresPer100Km2Dp: String = "",
        val litresPerHourRat: String = "",
        val litresPerHour2Dp: String = "",
        val centsPerKmRounded: Long = 0L,
        val centsPerHourRounded: Long = 0L,
        val sampleCount: Int = 0,
        val smallSample: Boolean = false,
    )

    data class AvailabilitySummary(
        val status: String,
        val reason: InsufficiencyReason? = null,
        val activeSeconds: Long = 0L,
        val unavailableSeconds: Long = 0L,
        val availableSeconds: Long = 0L,
        val ratio: String = "",
        val percent1Dp: String = "",
    )

    data class PriceComparisonInput(
        val fuelingProduct: String,
        val fuelingUnit: String,
        val placeKnown: Boolean,
        val placeRule: String,
        val fuelingCondition: String,
        val quantityMilli: Long,
        val paidCents: Long,
        val hasReference: Boolean,
        val referenceSource: String,
        val referenceProduct: String,
        val referenceUnit: String,
        val referenceCondition: String,
        val referencePriceMilli: Long,
        val referenceAgeDays: Int,
        val maxAgeDays: Int,
        val coverageStations: Int,
        val minCoverageStations: Int,
    )

    data class PriceComparisonResult(
        val status: String,
        val reason: InsufficiencyReason? = null,
        val methodologyVersion: String = PRICE_METHODOLOGY_VERSION,
        val referenceSource: String,
        val placeRule: String,
        val effectiveUnitMilli: Long = 0L,
        val referenceUnitMilli: Long = 0L,
        val referenceCents: Long = 0L,
        val differenceCents: Long = 0L,
        val differencePerUnitMilli: Long = 0L,
    )

    data class FindingSignal(
        val raised: Boolean,
        val type: FindingType? = null,
        val severity: FindingSeverity? = null,
    )

    private val validConditions = setOf("STANDARD", "CASH", "DEBIT", "CREDIT", "APP", "LOYALTY", "OTHER")

    fun formatRat(num: BigInteger, den: BigInteger): String {
        val g = num.gcd(den)
        val n = num.divide(g)
        val d = den.divide(g)
        return if (d == BigInteger.ONE) n.toString() else "$n/$d"
    }

    fun formatScaledDecimal(num: BigInteger, den: BigInteger, dp: Int): String {
        val scale = BigInteger.TEN.pow(dp)
        val scaled = num.multiply(scale)
        val neg = scaled.signum() < 0
        val absN = scaled.abs()
        val qr = absN.divideAndRemainder(den)
        var q = qr[0]
        if (qr[1].shiftLeft(1) >= den) {
            q = q.add(BigInteger.ONE)
        }
        val parts = q.divideAndRemainder(scale)
        val prefix = if (neg && (parts[0] != BigInteger.ZERO || parts[1] != BigInteger.ZERO)) "-" else ""
        if (dp == 0) return "$prefix${parts[0]}"
        val frac = parts[1].toString().padStart(dp, '0')
        return "$prefix${parts[0]}.$frac"
    }

    fun roundHalfUp(num: BigInteger, den: BigInteger): Long {
        val half = den.shiftRight(1)
        return num.add(half).divide(den).longValueExact()
    }

    fun computeDistanceConsumption(intervals: List<RatioInterval>): ConsumptionSummary {
        if (intervals.isEmpty()) {
            return ConsumptionSummary("INSUFFICIENT_DATA", InsufficiencyReason.NO_CLOSED_INTERVAL)
        }
        var sumDist = 0L
        var sumQty = 0L
        var sumCost = 0L
        val products = mutableSetOf<String>()
        for (iv in intervals) {
            if (iv.product == "CNG") {
                return ConsumptionSummary("INSUFFICIENT_DATA", InsufficiencyReason.METHOD_UNSUPPORTED)
            }
            if (iv.distanceM <= 0L || iv.quantityMilli <= 0L) continue
            products += iv.product
            sumDist += iv.distanceM
            sumQty += iv.quantityMilli
            sumCost += iv.costCentavos
        }
        if (products.size > 1) {
            return ConsumptionSummary("INSUFFICIENT_DATA", InsufficiencyReason.METHOD_UNSUPPORTED)
        }
        if (sumDist <= 0L || sumQty <= 0L) {
            return ConsumptionSummary("INSUFFICIENT_DATA", InsufficiencyReason.NO_CLOSED_INTERVAL)
        }
        val bDist = BigInteger.valueOf(sumDist)
        val bQty = BigInteger.valueOf(sumQty)
        val bCost = BigInteger.valueOf(sumCost)
        return ConsumptionSummary(
            status = "COMPUTED",
            distanceM = sumDist,
            quantityMilli = sumQty,
            costCentavos = sumCost,
            kmPerUnitRat = formatRat(bDist, bQty),
            kmPerUnit2Dp = formatScaledDecimal(bDist, bQty, 2),
            litresPer100Km2Dp = formatScaledDecimal(bQty.multiply(BigInteger.valueOf(100L)), bDist, 2),
            centsPerKmRounded = roundHalfUp(bCost.multiply(BigInteger.valueOf(1000L)), bDist),
            sampleCount = intervals.size,
            smallSample = intervals.size < 3,
        )
    }

    fun computeHourConsumption(intervals: List<RatioInterval>): ConsumptionSummary {
        if (intervals.isEmpty()) {
            return ConsumptionSummary("INSUFFICIENT_DATA", InsufficiencyReason.NO_HOUR_BASIS)
        }
        var sumSec = 0L
        var sumQty = 0L
        var sumCost = 0L
        for (iv in intervals) {
            if (iv.hourSeconds <= 0L || iv.quantityMilli <= 0L) continue
            sumSec += iv.hourSeconds
            sumQty += iv.quantityMilli
            sumCost += iv.costCentavos
        }
        if (sumSec <= 0L || sumQty <= 0L) {
            return ConsumptionSummary("INSUFFICIENT_DATA", InsufficiencyReason.NO_HOUR_BASIS)
        }
        val num = BigInteger.valueOf(sumQty).multiply(BigInteger.valueOf(3600L))
        val den = BigInteger.valueOf(sumSec).multiply(BigInteger.valueOf(1000L))
        val cphNum = BigInteger.valueOf(sumCost).multiply(BigInteger.valueOf(3600L))
        val cphDen = BigInteger.valueOf(sumSec)
        return ConsumptionSummary(
            status = "COMPUTED",
            hourSeconds = sumSec,
            quantityMilli = sumQty,
            costCentavos = sumCost,
            litresPerHourRat = formatRat(num, den),
            litresPerHour2Dp = formatScaledDecimal(num, den, 2),
            centsPerHourRounded = roundHalfUp(cphNum, cphDen),
            sampleCount = intervals.size,
            smallSample = intervals.size < 3,
        )
    }

    fun computeAvailability(activeSeconds: Long, unavailableSeconds: Long): AvailabilitySummary {
        if (activeSeconds <= 0L) {
            return AvailabilitySummary("INSUFFICIENT_DATA", InsufficiencyReason.NO_AVAILABILITY_HISTORY)
        }
        val clampedUnavail = unavailableSeconds.coerceIn(0L, activeSeconds)
        val avail = activeSeconds - clampedUnavail
        val bAvail = BigInteger.valueOf(avail)
        val bActive = BigInteger.valueOf(activeSeconds)
        return AvailabilitySummary(
            status = "COMPUTED",
            activeSeconds = activeSeconds,
            unavailableSeconds = clampedUnavail,
            availableSeconds = avail,
            ratio = formatRat(bAvail, bActive),
            percent1Dp = formatScaledDecimal(bAvail.multiply(BigInteger.valueOf(100L)), bActive, 1),
        )
    }

    fun evaluatePriceComparison(input: PriceComparisonInput): PriceComparisonResult {
        val effUnit = if (input.quantityMilli > 0L && input.paidCents >= 0L) {
            roundHalfUp(
                BigInteger.valueOf(input.paidCents).multiply(BigInteger.valueOf(10_000L)),
                BigInteger.valueOf(input.quantityMilli),
            )
        } else 0L
        fun fail(reason: InsufficiencyReason) = PriceComparisonResult(
            status = "INSUFFICIENT_DATA",
            reason = reason,
            referenceSource = input.referenceSource,
            placeRule = input.placeRule,
            effectiveUnitMilli = effUnit,
        )
        if (!input.hasReference || input.referencePriceMilli <= 0L ||
            input.referenceProduct != input.fuelingProduct || input.referenceUnit != input.fuelingUnit
        ) return fail(InsufficiencyReason.NO_REFERENCE)
        if (!input.placeKnown) return fail(InsufficiencyReason.PLACE_UNKNOWN)
        if (input.referenceAgeDays > input.maxAgeDays) return fail(InsufficiencyReason.REFERENCE_STALE)
        if (input.placeRule == "SAME_MUNICIPALITY" && input.coverageStations < input.minCoverageStations) {
            return fail(InsufficiencyReason.LOW_COVERAGE)
        }
        if (input.fuelingCondition !in validConditions || input.referenceCondition !in validConditions) {
            return fail(InsufficiencyReason.CONDITION_UNKNOWN)
        }
        if (input.fuelingCondition.trim() != input.referenceCondition.trim()) {
            return fail(InsufficiencyReason.CONDITION_MISMATCH)
        }
        val refCents = roundHalfUp(
            BigInteger.valueOf(input.quantityMilli).multiply(BigInteger.valueOf(input.referencePriceMilli)),
            BigInteger.valueOf(10_000L),
        )
        return PriceComparisonResult(
            status = "COMPUTED",
            referenceSource = input.referenceSource,
            placeRule = input.placeRule,
            effectiveUnitMilli = effUnit,
            referenceUnitMilli = input.referencePriceMilli,
            referenceCents = refCents,
            differenceCents = input.paidCents - refCents,
            differencePerUnitMilli = effUnit - input.referencePriceMilli,
        )
    }

    fun evaluateFT001Duplicate(deltaMinutes: Int, windowMinutes: Int, sameKeyOrHash: Boolean): FindingSignal =
        if (!sameKeyOrHash && deltaMinutes > windowMinutes) FindingSignal(false)
        else FindingSignal(true, FindingType.FT_001, if (sameKeyOrHash) FindingSeverity.HIGH else FindingSeverity.MEDIUM)

    fun evaluateFT002IncoherentMeter(
        decreased: Boolean,
        deltaM: Long,
        deltaS: Long,
        maxSpeedKmh: Int,
        hourMeterExceeded: Boolean,
    ): FindingSignal {
        val speedExceeded = if (deltaS > 0L && deltaM > 0L) {
            BigInteger.valueOf(deltaM).multiply(BigInteger.valueOf(3600L)) >
                BigInteger.valueOf(maxSpeedKmh.toLong() * 1000L).multiply(BigInteger.valueOf(deltaS))
        } else false
        return if (!decreased && !speedExceeded && !hourMeterExceeded) FindingSignal(false)
        else FindingSignal(true, FindingType.FT_002, FindingSeverity.MEDIUM)
    }

    fun evaluateFT003TankCapacity(
        qtyMilli: Long,
        capacityMilli: Long,
        tolerancePct: Int,
        incompatibleProduct: Boolean,
    ): FindingSignal {
        if (incompatibleProduct) return FindingSignal(true, FindingType.FT_003, FindingSeverity.HIGH)
        if (capacityMilli <= 0L) return FindingSignal(false)
        val bQty = BigInteger.valueOf(qtyMilli)
        val bCap = BigInteger.valueOf(capacityMilli)
        val limit = bCap.multiply(BigInteger.valueOf((100 + tolerancePct).toLong())).divide(BigInteger.valueOf(100L))
        if (bQty <= limit) return FindingSignal(false)
        val high = bQty.multiply(BigInteger.valueOf(100L)) > bCap.multiply(BigInteger.valueOf(120L))
        return FindingSignal(true, FindingType.FT_003, if (high) FindingSeverity.HIGH else FindingSeverity.MEDIUM)
    }

    fun evaluateFT004Outlier(baseM: Long, baseMl: Long, testM: Long, testMl: Long, thresholdPct: Int): FindingSignal {
        if (baseM <= 0L || baseMl <= 0L || testM <= 0L || testMl <= 0L) return FindingSignal(false)
        val num = BigInteger.valueOf(testM).multiply(BigInteger.valueOf(baseMl))
        val den = BigInteger.valueOf(testMl).multiply(BigInteger.valueOf(baseM))
        val diff = num.subtract(den).abs()
        val hundredDiff = diff.multiply(BigInteger.valueOf(100L))
        if (hundredDiff <= den.multiply(BigInteger.valueOf(thresholdPct.toLong()))) return FindingSignal(false)
        val high = hundredDiff > den.multiply(BigInteger.valueOf((thresholdPct * 2).toLong()))
        return FindingSignal(true, FindingType.FT_004, if (high) FindingSeverity.HIGH else FindingSeverity.MEDIUM)
    }

    fun evaluateFT005DocDivergence(recCents: Long, docCents: Long, issuerMismatch: Boolean): FindingSignal {
        val diff = kotlin.math.abs(recCents - docCents)
        if (!issuerMismatch && diff <= 1L) return FindingSignal(false)
        val high = issuerMismatch || (recCents > 0L && diff * 100L > recCents * 5L)
        return FindingSignal(true, FindingType.FT_005, if (high) FindingSeverity.HIGH else FindingSeverity.MEDIUM)
    }

    fun evaluateFT006PriceAboveRef(
        unitMilli: Long,
        refMilli: Long,
        diffCents: Long,
        thresholdPct: Int,
        minDiffCents: Long,
    ): FindingSignal {
        if (unitMilli <= 0L || refMilli <= 0L || diffCents < minDiffCents) return FindingSignal(false)
        val lhs = BigInteger.valueOf(unitMilli).multiply(BigInteger.valueOf(100L))
        val bRef = BigInteger.valueOf(refMilli)
        if (lhs <= bRef.multiply(BigInteger.valueOf((100 + thresholdPct).toLong()))) return FindingSignal(false)
        val sev = when {
            lhs >= bRef.multiply(BigInteger.valueOf(120L)) -> FindingSeverity.HIGH
            lhs >= bRef.multiply(BigInteger.valueOf(110L)) -> FindingSeverity.MEDIUM
            else -> FindingSeverity.LOW
        }
        return FindingSignal(true, FindingType.FT_006, sev)
    }

    fun validateFindingDecision(
        currentState: FindingState,
        authorId: String?,
        actorId: String,
        reason: String,
    ): DecisionError? = when {
        currentState == FindingState.CONFIRMED || currentState == FindingState.EXPLAINED ||
            currentState == FindingState.FALSE_POSITIVE || currentState == FindingState.SUPERSEDED ->
            DecisionError.FINDING_CLOSED
        !authorId.isNullOrBlank() && authorId == actorId -> DecisionError.SELF_APPROVAL
        reason.isBlank() -> DecisionError.JUSTIFICATION_REQUIRED
        else -> null
    }
}
