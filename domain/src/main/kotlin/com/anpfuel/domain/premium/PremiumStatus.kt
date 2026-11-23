package com.anpfuel.domain.premium

/**
 * MEX-16 server-verified Premium status.
 *
 * The server derives the state from the purchase snapshot and current
 * proof freshness; the client never mints entitlements locally. Unknown
 * wire states keep an explicit [PremiumState.UNKNOWN] instead of being
 * interpreted as active or silently dropped.
 */
enum class PremiumState {
    NONE,
    PENDING,
    ACTIVE,
    CANCELED,
    GRACE,
    HOLD,
    PAUSED,
    EXPIRED,
    REVOKED,
    UNKNOWN,
    ;

    companion object {
        fun parse(raw: String): PremiumState = entries.firstOrNull { it.name == raw } ?: UNKNOWN
    }
}

data class PremiumStatus(
    val policyVersion: Int,
    val state: PremiumState,
    val reason: String,
    val capabilities: Set<String>,
    val validUntil: String?,
    val retainedDataAccess: Boolean,
    val paymentsConfigured: Boolean,
    val managementUrl: String,
    /** Active entitlement sources, each with its own paid-through time (B-BR-STORE-02). */
    val tiers: List<PremiumTier> = emptyList(),
    /** One validated offer per configured store; empty means no in-app purchase. */
    val storeOffers: List<StoreOffer> = emptyList(),
) {
    init {
        require(policyVersion == 1)
        require(reason.length <= 64)
        require(capabilities.size <= 32)
        require(capabilities.all { it.isNotBlank() && it.length <= 64 })
        require(retainedDataAccess)
        require(tiers.size <= MAX_TIERS)
        require(storeOffers.size <= MAX_STORE_OFFERS)
    }

    /** The single Google Play offer, or null when absent or ambiguous (fail closed). */
    val playOffer: StoreOffer?
        get() = storeOffers.filter { it.provider == StoreProvider.PLAY }.singleOrNull()

    /** Paid writes require a verified eligible state; free exits stay open. */
    val canUsePaidCapabilities: Boolean
        get() = state in setOf(PremiumState.ACTIVE, PremiumState.CANCELED, PremiumState.GRACE) &&
            capabilities.isNotEmpty()

    fun hasCapability(name: String): Boolean = name in capabilities

    companion object {
        const val MANAGEMENT_URL: String = "https://play.google.com/store/account/subscriptions"
        const val MAX_TIERS: Int = 16
        const val MAX_STORE_OFFERS: Int = 8
    }
}

sealed class PremiumStatusException(message: String, cause: Throwable? = null) : Exception(message, cause) {
    class SignedOut : PremiumStatusException("premium.signed-out")
    class Denied(message: String) : PremiumStatusException(message)
    class Unavailable(message: String, cause: Throwable? = null) : PremiumStatusException(message, cause)
    class Invalid(message: String, cause: Throwable? = null) : PremiumStatusException(message, cause)
}

data class PremiumStatusSnapshot(
    val accountId: String,
    val status: PremiumStatus,
)
