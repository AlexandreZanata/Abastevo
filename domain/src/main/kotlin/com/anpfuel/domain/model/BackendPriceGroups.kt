package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException

/**
 * P10-T02 backend price-group read model.
 *
 * Mirrors `GET /v1/stations/{id}/prices` (backend `PriceGroup`):
 * one product/unit/condition with its official section. Community stays
 * null until P04 (UNKNOWN, never an ANP substitution). Money is exact
 * integer thousandths of a real; no float money.
 */
class BackendOfficialSection private constructor(
    val source: String,
    val amountMilliBrl: Long,
    val currency: String,
    val collectedOn: String,
    val surveyWeekStart: String,
    val surveyWeekEnd: String,
    val revisionId: String,
) {
    companion object {
        fun create(
            source: String,
            amountMilliBrl: Long,
            currency: String,
            collectedOn: String,
            surveyWeekStart: String,
            surveyWeekEnd: String,
            revisionId: String,
        ): BackendOfficialSection {
            require(source.isNotBlank()) { "official source is blank" }
            require(amountMilliBrl in 1..1_000_000) { "amount_milli_brl out of range" }
            require(currency == "BRL") { "currency must be BRL" }
            require(collectedOn.isNotBlank()) { "collected_on is blank" }
            require(surveyWeekStart.isNotBlank()) { "survey_week_start is blank" }
            require(surveyWeekEnd.isNotBlank()) { "survey_week_end is blank" }
            require(revisionId.isNotBlank()) { "revision_id is blank" }
            return BackendOfficialSection(
                source = source,
                amountMilliBrl = amountMilliBrl,
                currency = currency,
                collectedOn = collectedOn,
                surveyWeekStart = surveyWeekStart,
                surveyWeekEnd = surveyWeekEnd,
                revisionId = revisionId,
            )
        }
    }
}

class BackendPriceGroup private constructor(
    val stationId: String,
    val fuelProductWire: String,
    val unit: String,
    val conditionKind: String,
    val official: BackendOfficialSection?,
    val community: String?,
) {
    companion object {
        val WIRE_PRODUCTS: Set<String> = setOf(
            "ETHANOL",
            "GASOLINE_REGULAR",
            "GASOLINE_ADDITIVED",
            "DIESEL_S500",
            "DIESEL_S10",
            "CNG",
            "LPG_P13",
        )

        fun create(
            stationId: String,
            fuelProductWire: String,
            unit: String,
            conditionKind: String,
            official: BackendOfficialSection?,
            community: String?,
        ): BackendPriceGroup {
            if (!isUuid(stationId)) {
                throw DomainException("malformed station_id")
            }
            if (fuelProductWire !in WIRE_PRODUCTS) {
                throw DomainException("unknown wire fuel product: $fuelProductWire")
            }
            require(unit.isNotBlank()) { "unit is blank" }
            require(conditionKind.isNotBlank()) { "condition kind is blank" }
            if (community != null) {
                throw DomainException("community must stay null until P04")
            }
            return BackendPriceGroup(
                stationId = stationId.lowercase(),
                fuelProductWire = fuelProductWire,
                unit = unit,
                conditionKind = conditionKind,
                official = official,
                community = null,
            )
        }

        internal fun isUuid(value: String): Boolean {
            if (value.length != 36) return false
            for ((index, char) in value.withIndex()) {
                when (index) {
                    8, 13, 18, 23 -> if (char != '-') return false
                    else -> if (!char.isDigit() && char.lowercaseChar() !in 'a'..'f') return false
                }
            }
            return true
        }
    }
}

/**
 * One cached backend response with explicit source/version/expiry.
 * `source` is always "backend", `version` the contract version ("v1").
 */
class BackendPriceGroups private constructor(
    val stationId: String,
    val fuelFilterWire: String?,
    val groups: List<BackendPriceGroup>,
    val source: String,
    val version: String,
    val fetchedAtMillis: Long,
    val expiresAtMillis: Long,
) {
    companion object {
        const val SOURCE = "backend"
        const val VERSION = "v1"

        fun create(
            stationId: String,
            fuelFilterWire: String?,
            groups: List<BackendPriceGroup>,
            fetchedAtMillis: Long,
            expiresAtMillis: Long,
        ): BackendPriceGroups {
            if (!BackendPriceGroup.isUuid(stationId)) {
                throw DomainException("malformed station_id")
            }
            if (fuelFilterWire != null && fuelFilterWire !in BackendPriceGroup.WIRE_PRODUCTS) {
                throw DomainException("unknown wire fuel product: $fuelFilterWire")
            }
            require(expiresAtMillis > fetchedAtMillis) { "expiry must be after fetch" }
            return BackendPriceGroups(
                stationId = stationId.lowercase(),
                fuelFilterWire = fuelFilterWire,
                groups = groups.toList(),
                source = SOURCE,
                version = VERSION,
                fetchedAtMillis = fetchedAtMillis,
                expiresAtMillis = expiresAtMillis,
            )
        }
    }
}
