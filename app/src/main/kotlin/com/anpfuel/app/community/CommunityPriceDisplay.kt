package com.anpfuel.app.community

import com.anpfuel.domain.model.BackendPriceGroup
import com.anpfuel.domain.model.BackendPriceGroups

/**
 * P10-T06 — Official/community price presentation (B-BR-001/002/008,
 * PRODUCT_CONTRACT, COMMUNITY_PRICING_SPEC).
 *
 * Sources, conditions, uncertainty and age stay separate: the existing ANP
 * panel is never blended with community numbers. Community is UNKNOWN
 * until P04 (domain refuses non-null community); STALE comes only from an
 * explicit stale cache; DISPUTED is a future availability the mapper
 * already labels honestly without inventing backend data. Confidence is
 * support, never a guarantee. Money stays exact integer thousandths of a
 * real; every value carries its unit. APP/LOYALTY require a qualifier and
 * never compete as unconditional STANDARD.
 */
enum class CommunityAvailability {
    AVAILABLE,
    DISPUTED,
    UNKNOWN,
}

enum class CommunityFreshness {
    FRESH,
    AGING,
    STALE,
    UNKNOWN,
}

data class CommunityPriceRowUiModel(
    val fuelProductWire: String,
    val unit: String,
    val conditionKind: String,
    val qualifierId: String?,
    val conditionLabel: String,
    val officialAmountMilli: Long?,
    val officialSource: String?,
    val officialCollectedOn: String?,
    val officialRevisionId: String?,
    val availability: CommunityAvailability,
    val freshness: CommunityFreshness,
    val staleCache: Boolean,
)

object CommunityPriceDisplay {

    /** Exact milli-BRL with 3 fraction digits, pt-BR style, no float. */
    fun formatMilliBrl(amountMilliBrl: Long): String {
        require(amountMilliBrl in 1..1_000_000) { "amount_milli_brl out of range" }
        val reais = amountMilliBrl / 1000L
        val frac = (amountMilliBrl % 1000L).toInt()
        return "R$ $reais,%03d".format(frac)
    }

    /**
     * Human condition label with explicit qualifier rules. STANDARD needs
     * no qualifier; APP/LOYALTY without one stay pending (never silent
     * STANDARD); OTHER is excluded from generic cheapest ranking.
     */
    fun formatCondition(conditionKind: String, qualifierId: String?): String {
        val kind = conditionKind.trim().uppercase()
        require(kind.isNotBlank()) { "condition kind is blank" }
        return when (kind) {
            "STANDARD" -> "STANDARD"
            "APP", "LOYALTY" -> if (qualifierId.isNullOrBlank()) {
                "$kind — qualifier required"
            } else {
                "$kind · $qualifierId"
            }
            "OTHER" -> "OTHER — excluded from ranking"
            else -> kind
        }
    }

    /** Confidence is support, never a guarantee (PRODUCT_CONTRACT). */
    fun confidenceNote(): String =
        "Community support, not a guarantee. ANP is an official survey, not a live pump price."

    fun availabilityLabel(availability: CommunityAvailability): String =
        when (availability) {
            CommunityAvailability.AVAILABLE -> "AVAILABLE"
            CommunityAvailability.DISPUTED -> "DISPUTED"
            CommunityAvailability.UNKNOWN -> "UNKNOWN"
        }

    fun freshnessLabel(freshness: CommunityFreshness): String =
        when (freshness) {
            CommunityFreshness.FRESH -> "FRESH"
            CommunityFreshness.AGING -> "AGING"
            CommunityFreshness.STALE -> "STALE"
            CommunityFreshness.UNKNOWN -> "UNKNOWN"
        }

    /**
     * Maps one backend response to distinct official/community rows.
     * Community stays UNKNOWN until P04; [staleCache] marks an explicit
     * stale fallback (never presented as fresh). [qualifierByFuel] carries
     * optional APP/LOYALTY qualifiers keyed by wire product.
     */
    fun fromGroups(
        groups: BackendPriceGroups,
        staleCache: Boolean = false,
        qualifierByFuel: Map<String, String?> = emptyMap(),
    ): List<CommunityPriceRowUiModel> =
        groups.groups.map { group -> fromGroup(group, staleCache, qualifierByFuel[group.fuelProductWire]) }

    internal fun fromGroup(
        group: BackendPriceGroup,
        staleCache: Boolean,
        qualifierId: String?,
    ): CommunityPriceRowUiModel {
        val official = group.official
        return CommunityPriceRowUiModel(
            fuelProductWire = group.fuelProductWire,
            unit = group.unit,
            conditionKind = group.conditionKind,
            qualifierId = qualifierId,
            conditionLabel = formatCondition(group.conditionKind, qualifierId),
            officialAmountMilli = official?.amountMilliBrl,
            officialSource = official?.source,
            officialCollectedOn = official?.collectedOn,
            officialRevisionId = official?.revisionId,
            availability = CommunityAvailability.UNKNOWN,
            freshness = if (staleCache) CommunityFreshness.STALE else CommunityFreshness.UNKNOWN,
            staleCache = staleCache,
        )
    }
}
