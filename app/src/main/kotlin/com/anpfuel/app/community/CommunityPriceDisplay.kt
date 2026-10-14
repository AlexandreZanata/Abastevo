package com.anpfuel.app.community

import com.anpfuel.domain.model.BackendPriceGroup
import com.anpfuel.domain.model.BackendPriceGroups

/**
 * P10-T06 — Official/community price presentation (B-BR-001/002/008,
 * PRODUCT_CONTRACT, COMMUNITY_PRICING_SPEC).
 *
 * Sources, conditions, uncertainty and age stay separate: the existing ANP
 * panel is never blended with community numbers. Missing/expired projections
 * are UNKNOWN; cached or expired values are labelled STALE. Confidence is
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
    val communityAmountMilli: Long? = null,
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
     * Community follows its typed current projection; [staleCache] marks an explicit
     * stale fallback (never presented as fresh). [qualifierByFuel] carries
     * optional APP/LOYALTY qualifiers keyed by wire product.
     */
    fun fromGroups(
        groups: BackendPriceGroups,
        staleCache: Boolean = false,
        qualifierByFuel: Map<String, String?> = emptyMap(),
        now: java.time.Instant = java.time.Instant.now(),
    ): List<CommunityPriceRowUiModel> =
        groups.groups.map { group -> fromGroup(group, staleCache, qualifierByFuel[group.fuelProductWire], now) }

    internal fun fromGroup(
        group: BackendPriceGroup,
        staleCache: Boolean,
        qualifierId: String?,
        now: java.time.Instant = java.time.Instant.now(),
    ): CommunityPriceRowUiModel {
        val official = group.official
        val community = group.community
        val expired = community?.expiresAt?.let { it<=now } == true
        val future = community?.lastReceivedAt?.let { it>now } == true
        val available = community?.availability=="AVAILABLE" && !expired && !future && !staleCache
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
            availability = when { available -> CommunityAvailability.AVAILABLE; community?.availability=="DISPUTED" -> CommunityAvailability.DISPUTED; else -> CommunityAvailability.UNKNOWN },
            freshness = when { staleCache || expired -> CommunityFreshness.STALE; future || community==null -> CommunityFreshness.UNKNOWN; else -> CommunityFreshness.valueOf(community.freshness) },
            communityAmountMilli = if(available) community?.amountMilliBrl else null,
            staleCache = staleCache,
        )
    }
}
