package com.anpfuel.data.remote.premium

import com.anpfuel.domain.premium.PremiumSource
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusException
import com.anpfuel.domain.premium.PremiumTier
import com.anpfuel.domain.premium.StoreOffer
import com.anpfuel.domain.premium.StoreProduct
import com.anpfuel.domain.premium.StoreProvider
import java.time.Instant
import org.json.JSONArray
import org.json.JSONObject

private const val MAX_CAPABILITIES = 32
private const val MAX_CAPABILITY_CHARS = 64
private const val MAX_SOURCE_CHARS = 32
private val allowedCapabilities = setOf(
    "cloud.sync",
    "cloud.backup",
    "cloud.restore",
    "alerts.advanced",
    "professional.write",
    "professional.report",
    "vehicles.multiple",
)

/**
 * Frozen Premium status shape shared by the status read and the purchase
 * receipts. Corrupt core fields fail closed as [PremiumStatusException.Invalid].
 * A malformed or unknown store offer is dropped instead, so it can only remove
 * the purchase button (B-BR-STORE-10) and never hides the verified status.
 */
internal object PremiumStatusJson {

    fun decode(root: JSONObject): PremiumStatus {
        try {
            if (root.optInt("policy_version", -1) != 1) throw invalid("policy")
            val state = PremiumState.parse(root.getString("state"))
            val reason = root.getString("reason")
            if (reason.length > 64) throw invalid("reason")
            val capabilitiesRaw = root.optJSONArray("capabilities") ?: JSONArray()
            if (capabilitiesRaw.length() > MAX_CAPABILITIES) throw invalid("capabilities")
            val capabilities = mutableSetOf<String>()
            for (index in 0 until capabilitiesRaw.length()) {
                val value = capabilitiesRaw.opt(index)
                if (value !is String || value.isBlank() || value.length > MAX_CAPABILITY_CHARS ||
                    value !in allowedCapabilities
                ) {
                    throw invalid("capability")
                }
                capabilities.add(value)
            }
            val validUntil = instantOrNull(root, "valid_until")
            if (root.opt("retained_data_access") != true) throw invalid("retained")
            if (root.opt("payments_configured") !is Boolean) throw invalid("payments")
            val management = if (root.has("management_url")) {
                if (root.getString("management_url") != PremiumStatus.MANAGEMENT_URL) throw invalid("management")
                PremiumStatus.MANAGEMENT_URL
            } else {
                PremiumStatus.MANAGEMENT_URL
            }
            return PremiumStatus(
                policyVersion = 1,
                state = state,
                reason = reason,
                capabilities = capabilities.toSet(),
                validUntil = validUntil,
                retainedDataAccess = true,
                paymentsConfigured = root.getBoolean("payments_configured"),
                managementUrl = management,
                tiers = tiers(root),
                storeOffers = storeOffers(root),
            )
        } catch (e: PremiumStatusException) {
            throw e
        } catch (e: Exception) {
            throw invalid("body", e)
        }
    }

    private fun instantOrNull(obj: JSONObject, name: String): String? {
        if (!obj.has(name) || obj.isNull(name)) return null
        val raw = obj.opt(name) as? String ?: throw invalid(name)
        Instant.parse(raw)
        return raw
    }

    private fun tiers(root: JSONObject): List<PremiumTier> {
        if (!root.has("tiers") || root.isNull("tiers")) return emptyList()
        val raw = root.opt("tiers") as? JSONArray ?: throw invalid("tiers")
        if (raw.length() > PremiumStatus.MAX_TIERS) throw invalid("tiers")
        return (0 until raw.length()).map { index ->
            val tier = raw.opt(index) as? JSONObject ?: throw invalid("tier")
            val name = tier.opt("tier") as? String ?: throw invalid("tier")
            val source = tier.opt("source") as? String ?: throw invalid("tier")
            if (name.isBlank() || name.length > PremiumTier.MAX_TIER_CHARS || source.length > MAX_SOURCE_CHARS) {
                throw invalid("tier")
            }
            PremiumTier(name, PremiumSource.parse(source), instantOrNull(tier, "valid_until"))
        }
    }

    private fun storeOffers(root: JSONObject): List<StoreOffer> {
        if (!root.has("store_offers") || root.isNull("store_offers")) return emptyList()
        val raw = root.opt("store_offers") as? JSONArray ?: return emptyList()
        if (raw.length() > PremiumStatus.MAX_STORE_OFFERS) return emptyList()
        return (0 until raw.length()).mapNotNull { index -> storeOffer(raw.opt(index)) }
    }

    private fun storeOffer(value: Any?): StoreOffer? {
        val offer = value as? JSONObject ?: return null
        val provider = (offer.opt("provider") as? String)?.let(StoreProvider::parse) ?: return null
        val productsRaw = offer.opt("products") as? JSONArray ?: return null
        if (productsRaw.length() > StoreOffer.MAX_PRODUCTS) return null
        val products = (0 until productsRaw.length()).map { index ->
            val product = productsRaw.opt(index) as? JSONObject ?: return null
            val productId = product.opt("product_id") as? String ?: return null
            val plansRaw = when {
                !product.has("plan_ids") || product.isNull("plan_ids") -> JSONArray()
                else -> product.opt("plan_ids") as? JSONArray ?: return null
            }
            if (plansRaw.length() > StoreOffer.MAX_PLANS) return null
            val plans = (0 until plansRaw.length()).map { planIndex -> plansRaw.opt(planIndex) as? String ?: return null }
            StoreProduct(productId, plans)
        }
        val binding = offer.opt("account_binding") as? String ?: return null
        val management = offer.opt("management_url") as? String ?: return null
        return StoreOffer.parseOrNull(provider, products, binding, management)
    }

    fun invalid(part: String, cause: Throwable? = null) =
        PremiumStatusException.Invalid("premium.invalid-$part", cause)
}
