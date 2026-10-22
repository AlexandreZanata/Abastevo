package com.anpfuel.domain.charging

import java.math.BigInteger
import java.time.Instant

/**
 * EE-11 — Pure exact tariff scenario calculation.
 *
 * Exact tariff + declared scenario + known eligibility produces either a
 * KNOWN centavos total or an explicit UNKNOWN carrying the frozen reason.
 * There is no network, SQL, entitlement, provider call or floating point
 * here. Rates are exact rational centavos per base unit; quantities are
 * rounded up to their declared billing step only after the applicable grace,
 * and the single rounding is the tariff's declared half-up-centavo boundary.
 */
data class RationalValue(
    val numerator: String,
    val denominator: String,
)

data class TariffComponent(
    val dimension: String,
    val rateCentavos: RationalValue,
    val billingStep: Long,
    val graceSeconds: Long,
)

data class Tariff(
    val version: Int,
    val id: String,
    val revision: String,
    val targetKind: String,
    val targetId: String,
    val currency: String,
    val condition: String,
    val qualifierId: String?,
    val timezone: String,
    val validFrom: Instant,
    val validUntil: Instant?,
    val taxBasis: String,
    val mandatoryComponentsComplete: Boolean,
    val components: List<TariffComponent>,
    val minimumCentavos: Long?,
    val maximumCentavos: Long?,
    val billingRounding: String,
    val provenance: String,
)

data class TariffScenario(
    val condition: String,
    val qualifierId: String?,
    val windowStart: Instant,
    val windowEnd: Instant,
    val energyWh: Long?,
    val chargingSeconds: Long?,
    val idleSeconds: Long?,
    val sessionCount: Long?,
    val parkingSeconds: Long?,
)

data class TariffEstimate(
    val state: String,
    val method: String? = null,
    val unit: String? = null,
    val reason: String? = null,
    val value: RationalValue? = null,
)

class TariffValidationException(val code: String) : RuntimeException(code)

object TariffCalculator {

    const val MAX_ENERGY_WH: Long = 1_000_000_000L
    const val MAX_SECONDS: Long = 31_622_400L
    const val MAX_CENTAVOS: Long = 1_000_000_000_000L
    const val MAX_BILLING_STEP: Long = 1_000_000_000L
    const val MAX_COMPONENTS: Int = 32

    private val DIMENSIONS = setOf(
        "ENERGY_WH",
        "CHARGING_SECOND",
        "IDLE_SECOND",
        "SESSION",
        "PARKING_SECOND",
    )
    private val TIME_DIMENSIONS = setOf("CHARGING_SECOND", "IDLE_SECOND", "PARKING_SECOND")
    private val NUMERATOR = Regex("^(0|[1-9][0-9]{0,11}|1000000000000)$")
    private val DENOMINATOR = Regex("^([1-9][0-9]{0,8}|1000000000)$")
    private val CURRENCY = Regex("^[A-Z]{3}$")
    private val UUID = Regex("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
    private val ONE = BigInteger.ONE
    private val MAX_CENTAVOS_BIG = BigInteger.valueOf(MAX_CENTAVOS)

    /**
     * Precedence is fixed so shared vectors are deterministic:
     * invalid-tariff, invalid-scenario, ineligible-condition,
     * UNSUPPORTED_CURRENCY, UNKNOWN_FEES, MISMATCHED_WINDOW,
     * missing-scenario-quantity, exact arithmetic, OVERFLOW.
     */
    fun calculate(tariff: Tariff, scenario: TariffScenario): TariffEstimate {
        validateTariff(tariff)
        validateScenario(scenario)
        if (tariff.condition == "UNKNOWN" || scenario.condition == "UNKNOWN" ||
            tariff.condition != scenario.condition ||
            tariff.qualifierId != scenario.qualifierId
        ) {
            throw TariffValidationException("ineligible-condition")
        }
        if (tariff.currency != "BRL") {
            return unknown("UNSUPPORTED_CURRENCY")
        }
        if (tariff.taxBasis != "INCLUDED" || !tariff.mandatoryComponentsComplete ||
            tariff.minimumCentavos != null || tariff.maximumCentavos != null ||
            tariff.billingRounding != "FINAL_HALF_UP_CENTAVO"
        ) {
            return unknown("UNKNOWN_FEES")
        }
        if (scenario.windowStart.isBefore(tariff.validFrom) ||
            (tariff.validUntil != null && scenario.windowEnd.isAfter(tariff.validUntil))
        ) {
            return unknown("MISMATCHED_WINDOW")
        }

        // Validate every declared quantity before deciding completeness so a
        // malformed scenario is refused rather than reported as unknown fees.
        val declared = mapOf(
            "ENERGY_WH" to scenario.energyWh,
            "CHARGING_SECOND" to scenario.chargingSeconds,
            "IDLE_SECOND" to scenario.idleSeconds,
            "SESSION" to scenario.sessionCount,
            "PARKING_SECOND" to scenario.parkingSeconds,
        )
        val quantities = LongArray(tariff.components.size)
        var hasEnergy = false
        tariff.components.forEachIndexed { index, component ->
            quantities[index] = chargedQuantity(component, declared)
            if (component.dimension == "ENERGY_WH") {
                hasEnergy = true
            }
        }
        // An absent energy price is not a zero price.
        if (!hasEnergy) {
            return unknown("UNKNOWN_FEES")
        }

        var sumNumerator = BigInteger.ZERO
        var sumDenominator = BigInteger.ONE
        tariff.components.forEachIndexed { index, component ->
            val rate = parseRate(component.rateCentavos)
            val costNumerator = rate.first.multiply(BigInteger.valueOf(quantities[index]))
            val costDenominator = rate.second
            sumNumerator = sumNumerator.multiply(costDenominator)
                .add(costNumerator.multiply(sumDenominator))
            sumDenominator = sumDenominator.multiply(costDenominator)
        }

        val rounded = halfUpCentavos(sumNumerator, sumDenominator)
        if (rounded > MAX_CENTAVOS_BIG) {
            return unknown("OVERFLOW")
        }
        return TariffEstimate(
            state = "KNOWN",
            method = "TARIFF_SCENARIO",
            unit = "CENTAVOS",
            value = RationalValue(rounded.toString(), "1"),
        )
    }

    private fun unknown(reason: String): TariffEstimate =
        TariffEstimate(state = "UNKNOWN", reason = reason)

    private fun validateTariff(tariff: Tariff) {
        if (tariff.version != 1 || !UUID.matches(tariff.id) || !UUID.matches(tariff.targetId) ||
            !boundedText(tariff.revision) || !boundedText(tariff.timezone)
        ) {
            throw TariffValidationException("invalid-tariff")
        }
        if (tariff.targetKind !in setOf("LOCATION", "EVSE", "CONNECTOR")) {
            throw TariffValidationException("invalid-tariff")
        }
        if (!CURRENCY.matches(tariff.currency)) {
            throw TariffValidationException("invalid-tariff")
        }
        validateCondition(tariff.condition, tariff.qualifierId, "invalid-tariff")
        if (tariff.validUntil != null && tariff.validUntil.isBefore(tariff.validFrom)) {
            throw TariffValidationException("invalid-tariff")
        }
        if (tariff.taxBasis !in setOf("INCLUDED", "EXCLUDED", "UNKNOWN")) {
            throw TariffValidationException("invalid-tariff")
        }
        if (tariff.provenance !in setOf(
                "LICENSED_OPERATOR",
                "DATED_COMMUNITY_REPORT",
                "USER_ESTIMATE",
            )
        ) {
            throw TariffValidationException("invalid-tariff")
        }
        if (tariff.billingRounding !in setOf(
                "FINAL_HALF_UP_CENTAVO",
                "SOURCE_SPECIFIC_UNSUPPORTED",
            )
        ) {
            throw TariffValidationException("invalid-tariff")
        }
        if (tariff.components.isEmpty() || tariff.components.size > MAX_COMPONENTS) {
            throw TariffValidationException("invalid-tariff")
        }
        val seen = mutableSetOf<String>()
        for (component in tariff.components) {
            if (component.dimension !in DIMENSIONS || !seen.add(component.dimension)) {
                throw TariffValidationException("invalid-tariff")
            }
            parseRate(component.rateCentavos)
            if (component.billingStep < 1 || component.billingStep > MAX_BILLING_STEP ||
                component.graceSeconds < 0 || component.graceSeconds > MAX_SECONDS
            ) {
                throw TariffValidationException("invalid-tariff")
            }
            if (component.graceSeconds > 0 && component.dimension !in TIME_DIMENSIONS) {
                throw TariffValidationException("invalid-tariff")
            }
            if (component.dimension == "SESSION" && component.billingStep != 1L) {
                throw TariffValidationException("invalid-tariff")
            }
        }
        tariff.minimumCentavos?.let { if (it < 0 || it > MAX_CENTAVOS) throw TariffValidationException("invalid-tariff") }
        tariff.maximumCentavos?.let { if (it < 0 || it > MAX_CENTAVOS) throw TariffValidationException("invalid-tariff") }
        if (tariff.minimumCentavos != null && tariff.maximumCentavos != null &&
            tariff.minimumCentavos > tariff.maximumCentavos
        ) {
            throw TariffValidationException("invalid-tariff")
        }
    }

    private fun validateScenario(scenario: TariffScenario) {
        if (!scenario.windowEnd.isAfter(scenario.windowStart)) {
            throw TariffValidationException("invalid-scenario")
        }
        listOf(
            "ENERGY_WH" to scenario.energyWh, "CHARGING_SECOND" to scenario.chargingSeconds,
            "IDLE_SECOND" to scenario.idleSeconds, "SESSION" to scenario.sessionCount,
            "PARKING_SECOND" to scenario.parkingSeconds,
        ).forEach { (dimension, quantity) -> if (quantity != null) validateQuantity(dimension, quantity) }
        validateCondition(scenario.condition, scenario.qualifierId, "invalid-scenario")
    }

    private fun boundedText(value: String): Boolean =
        value.isNotEmpty() && value.codePointCount(0, value.length) <= 128

    private fun validateCondition(condition: String, qualifierId: String?, code: String) {
        when (condition) {
            "STANDARD" -> if (qualifierId != null) throw TariffValidationException(code)
            "APP", "MEMBER", "EMPLOYER" ->
                if (qualifierId == null || !boundedText(qualifierId)) {
                    throw TariffValidationException(code)
                }
            "UNKNOWN" -> if (qualifierId != null) throw TariffValidationException(code)
            else -> throw TariffValidationException(code)
        }
    }

    /** Returns the canonical reduced rational (numerator, denominator). */
    private fun parseRate(value: RationalValue): Pair<BigInteger, BigInteger> {
        if (!NUMERATOR.matches(value.numerator) || !DENOMINATOR.matches(value.denominator)) {
            throw TariffValidationException("invalid-tariff")
        }
        val numerator = BigInteger(value.numerator)
        val denominator = BigInteger(value.denominator)
        if (denominator.signum() <= 0 || numerator.gcd(denominator) != ONE) {
            throw TariffValidationException("invalid-tariff")
        }
        return numerator to denominator
    }

    private fun chargedQuantity(
        component: TariffComponent,
        declared: Map<String, Long?>,
    ): Long {
        val quantity = declared[component.dimension]
            ?: throw TariffValidationException("missing-scenario-quantity")
        validateQuantity(component.dimension, quantity)
        if (component.dimension == "SESSION") {
            return 1L
        }
        var value = quantity
        if (component.dimension in TIME_DIMENSIONS && component.graceSeconds > 0) {
            if (value <= component.graceSeconds) {
                return 0L
            }
            value -= component.graceSeconds
        }
        if (value == 0L) {
            return 0L
        }
        if (value % component.billingStep != 0L) {
            value = (value / component.billingStep + 1) * component.billingStep
        }
        return value
    }

    private fun validateQuantity(dimension: String, value: Long) {
        when (dimension) {
            "ENERGY_WH" -> if (value < 0 || value > MAX_ENERGY_WH) {
                throw TariffValidationException("invalid-scenario")
            }
            "SESSION" -> if (value != 1L) throw TariffValidationException("invalid-scenario")
            else -> if (value < 0 || value > MAX_SECONDS) {
                throw TariffValidationException("invalid-scenario")
            }
        }
    }

    /** floor(sum + 1/2) for a nonnegative exact sum. */
    private fun halfUpCentavos(numerator: BigInteger, denominator: BigInteger): BigInteger =
        numerator.multiply(BigInteger.TWO)
            .add(denominator)
            .divide(denominator.multiply(BigInteger.TWO))
}
