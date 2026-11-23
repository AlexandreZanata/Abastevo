package com.anpfuel.domain.premium

/**
 * Store billing model (B-BR-STORE-01..10).
 *
 * The server lists one offer per configured store with the catalog
 * products, the account binding and the store's management page. The
 * app sells only through its own store and never shows a web price or
 * checkout. A missing, malformed or ambiguous offer means no purchase
 * button (fail closed). Only the server grants access.
 */
enum class StoreProvider(val wire: String) {
    PLAY("play"),
    APP_STORE("app_store"),
    ;

    companion object {
        fun parse(raw: String): StoreProvider? = entries.firstOrNull { it.wire == raw }
    }
}

/** Channel that granted one active Premium tier (B-BR-STORE-02). */
enum class PremiumSource(val wire: String) {
    PLAY("play"),
    APP_STORE("app_store"),
    WEB("web"),
    ADMIN("admin"),
    UNKNOWN("unknown"),
    ;

    companion object {
        fun parse(raw: String): PremiumSource = entries.firstOrNull { it != UNKNOWN && it.wire == raw } ?: UNKNOWN
    }
}

/** One active entitlement source with its own paid-through time. */
data class PremiumTier(
    val tier: String,
    val source: PremiumSource,
    val validUntil: String?,
) {
    init {
        require(tier.isNotBlank() && tier.length <= MAX_TIER_CHARS)
    }

    companion object {
        const val MAX_TIER_CHARS: Int = 32
    }
}

data class StoreProduct(
    val productId: String,
    val planIds: List<String>,
)

data class StoreOffer(
    val provider: StoreProvider,
    val products: List<StoreProduct>,
    val accountBinding: String,
    val managementUrl: String,
) {
    init {
        require(products.size in 1..MAX_PRODUCTS)
        require(products.map { it.productId }.toSet().size == products.size)
        require(products.all { it.planIds.size <= MAX_PLANS && it.planIds.toSet().size == it.planIds.size })
        when (provider) {
            StoreProvider.PLAY -> {
                require(PLAY_BINDING.matches(accountBinding))
                require(PLAY_MANAGEMENT.matches(managementUrl))
                require(products.all { product ->
                    PLAY_PRODUCT.matches(product.productId) &&
                        product.planIds.isNotEmpty() &&
                        product.planIds.all { PLAY_PLAN.matches(it) }
                })
            }
            StoreProvider.APP_STORE -> {
                require(LOWER_UUID.matches(accountBinding))
                require(managementUrl == APP_STORE_MANAGEMENT)
                require(products.all { APP_STORE_PRODUCT.matches(it.productId) })
            }
        }
    }

    /** True when [purchase] is for a product of this server catalog. */
    fun sells(purchase: StorePurchase): Boolean {
        val catalog = products.map { it.productId }.toSet()
        return purchase.productIds.any { it in catalog }
    }

    companion object {
        const val MAX_PRODUCTS: Int = 16
        const val MAX_PLANS: Int = 16
        const val APP_STORE_MANAGEMENT: String = "https://apps.apple.com/account/subscriptions"
        private val PLAY_BINDING = Regex("[0-9a-f]{64}")
        private val LOWER_UUID = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
        private val PLAY_MANAGEMENT =
            Regex("https://play\\.google\\.com/store/account/subscriptions(\\?[A-Za-z0-9._~=&%-]{1,512})?")
        private val PLAY_PRODUCT = Regex("[a-z0-9][a-z0-9._]{0,139}")
        private val PLAY_PLAN = Regex("[a-z0-9][a-z0-9-]{0,62}")
        private val APP_STORE_PRODUCT = Regex("[A-Za-z0-9][A-Za-z0-9._-]{0,99}")

        /** Validated offer, or null so a malformed offer can only remove the purchase button. */
        fun parseOrNull(
            provider: StoreProvider,
            products: List<StoreProduct>,
            accountBinding: String,
            managementUrl: String,
        ): StoreOffer? = try {
            StoreOffer(provider, products, accountBinding, managementUrl)
        } catch (_: IllegalArgumentException) {
            null
        }
    }
}

/** What the Premium page may offer for the current verified status. */
enum class PurchaseEligibility {
    /** No configured offer for this store (B-BR-STORE-10): status only, no purchase button. */
    NO_OFFER,
    AVAILABLE,
    /** A store purchase waits for payment or verification and grants nothing yet. */
    PENDING,
    /** A Play subscription exists; changes, renewals and payment fixes happen in Play. */
    MANAGED_IN_STORE,
    /** Web, App Store or a granted tier already unlocks the account (BUC-STORE-04). */
    ACTIVE_FROM_OTHER_SOURCE,
    /** An unknown server state never opens a purchase. */
    UNKNOWN_STATE,
}

private val playSubscriptionStates = setOf(
    PremiumState.ACTIVE,
    PremiumState.CANCELED,
    PremiumState.GRACE,
    PremiumState.HOLD,
    PremiumState.PAUSED,
)

fun PremiumStatus.purchaseEligibility(): PurchaseEligibility {
    if (canUsePaidCapabilities) {
        val sources = tiers.map { it.source }.toSet()
        return if (sources.isEmpty() || PremiumSource.PLAY in sources) {
            PurchaseEligibility.MANAGED_IN_STORE
        } else {
            PurchaseEligibility.ACTIVE_FROM_OTHER_SOURCE
        }
    }
    if (playOffer == null) return PurchaseEligibility.NO_OFFER
    return when (state) {
        PremiumState.NONE, PremiumState.EXPIRED, PremiumState.REVOKED -> PurchaseEligibility.AVAILABLE
        PremiumState.PENDING -> PurchaseEligibility.PENDING
        PremiumState.UNKNOWN -> PurchaseEligibility.UNKNOWN_STATE
        in playSubscriptionStates -> PurchaseEligibility.MANAGED_IN_STORE
        else -> PurchaseEligibility.UNKNOWN_STATE
    }
}

/** ISO-8601 store billing period such as `P1M` or `P1Y`. */
data class StoreBillingPeriod(
    val unit: PeriodUnit,
    val count: Int,
) {
    enum class PeriodUnit { DAY, WEEK, MONTH, YEAR }

    companion object {
        private val PATTERN = Regex("P([1-9][0-9]{0,2})([DWMY])")

        fun parse(raw: String): StoreBillingPeriod? {
            val match = PATTERN.matchEntire(raw) ?: return null
            val unit = when (match.groupValues[2]) {
                "D" -> PeriodUnit.DAY
                "W" -> PeriodUnit.WEEK
                "M" -> PeriodUnit.MONTH
                else -> PeriodUnit.YEAR
            }
            return StoreBillingPeriod(unit, match.groupValues[1].toInt())
        }
    }
}

/** One pricing phase of a store offer, already formatted by the store. */
data class StorePricePhase(
    val formattedPrice: String,
    val billingPeriod: String,
    val infiniteRecurring: Boolean,
)

/** One subscription offer the store lists for this user; [offerId] is null for the base plan offer. */
data class StoreSubscriptionOffer(
    val basePlanId: String,
    val offerId: String?,
    val offerToken: String,
    val phases: List<StorePricePhase>,
)

data class StoreProductDetails(
    val productId: String,
    val offers: List<StoreSubscriptionOffer>,
)

/** A purchasable plan: a server-listed base plan with the store-formatted recurring price. */
data class StorePlan(
    val productId: String,
    val basePlanId: String,
    val offerToken: String,
    val formattedPrice: String,
    val billingPeriod: StoreBillingPeriod?,
)

/**
 * Plans the app may sell: the server catalog decides which base plans exist
 * and their order; the store decides the localized price. Developer offers
 * (trials, introductory prices) and prepaid plans are not presented.
 */
fun selectStorePlans(offer: StoreOffer, catalog: List<StoreProductDetails>): List<StorePlan> {
    val plans = mutableListOf<StorePlan>()
    for (product in offer.products) {
        val details = catalog.firstOrNull { it.productId == product.productId } ?: continue
        for (planId in product.planIds) {
            val base = details.offers.firstOrNull { it.basePlanId == planId && it.offerId == null } ?: continue
            val recurring = base.phases.lastOrNull { it.infiniteRecurring } ?: continue
            if (base.offerToken.isBlank() || recurring.formattedPrice.isBlank()) continue
            plans += StorePlan(
                productId = product.productId,
                basePlanId = planId,
                offerToken = base.offerToken,
                formattedPrice = recurring.formattedPrice,
                billingPeriod = StoreBillingPeriod.parse(recurring.billingPeriod),
            )
        }
    }
    return plans
}

enum class StorePurchaseState { PURCHASED, PENDING, UNSPECIFIED }

/**
 * One store purchase as reported on this device. The purchase token is the
 * stable key the server verifies (B-BR-STORE-05); it is never printed.
 */
data class StorePurchase(
    val purchaseToken: String,
    val productIds: List<String>,
    val state: StorePurchaseState,
    val acknowledged: Boolean,
    val accountBinding: String?,
) {
    /** A purchase with another account's binding is never sent for this account. */
    fun boundTo(binding: String): Boolean = accountBinding == null || accountBinding == binding

    override fun toString(): String =
        "StorePurchase(products=$productIds, state=$state, acknowledged=$acknowledged)"

    companion object {
        const val MAX_TOKEN_CHARS: Int = 4096

        fun isValidToken(token: String): Boolean =
            token.length in 1..MAX_TOKEN_CHARS && token.all { it.code in 33..126 }
    }
}

/** Idempotency key of one verify/restore attempt: a fresh lowercase UUID reused only on retry. */
object StoreOperationId {
    private val PATTERN = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")

    fun isValid(id: String): Boolean = PATTERN.matches(id)
}

enum class StorePurchaseKind(val route: String) {
    VERIFY("verify"),
    RESTORE("restore"),
}

/** Local or server result of handing one store purchase to the server. */
sealed interface StorePurchaseOutcome {
    /** The store still waits for payment; nothing is sent and nothing is granted. */
    data object Pending : StorePurchaseOutcome

    /** Not a completed purchase, or an unusable token; nothing is sent. */
    data object NotPurchased : StorePurchaseOutcome

    /** The same purchase is already being verified in this process. */
    data object InProgress : StorePurchaseOutcome
}

/** Server verdict for one signed verify/restore request (B-BR-STORE-03). */
sealed interface StoreVerification : StorePurchaseOutcome {
    data class Verified(val status: PremiumStatus, val replayed: Boolean) : StoreVerification

    /** 403: binding, product, environment or app mismatch. Nothing was granted. */
    data object Denied : StoreVerification

    /** 401: the session or the device key bound to the account is no longer accepted. */
    data object SignInRequired : StoreVerification

    /** 409/429: retry the same attempt later with the same operation ID. */
    data class RetryLater(val retryAfterSeconds: Long?) : StoreVerification

    /** 503 or network failure: the purchase stays unverified and is sent again later. */
    data object Unavailable : StoreVerification

    /** 400 or a malformed answer. */
    data object Invalid : StoreVerification

    /** The signed-in account changed; the result is discarded. */
    data object OwnerChanged : StoreVerification
}

/**
 * The outcome that matters most to the user when several purchases were
 * handed over at once: any verified status wins (the last one is the most
 * recent server view); then the states that need the user's attention.
 */
fun strongestStoreOutcome(outcomes: List<StorePurchaseOutcome>): StorePurchaseOutcome {
    outcomes.lastOrNull { it is StoreVerification.Verified }?.let { return it }
    if (StoreVerification.OwnerChanged in outcomes) return StoreVerification.OwnerChanged
    if (StoreVerification.SignInRequired in outcomes) return StoreVerification.SignInRequired
    if (StoreVerification.Unavailable in outcomes) return StoreVerification.Unavailable
    outcomes.filterIsInstance<StoreVerification.RetryLater>()
        .maxByOrNull { it.retryAfterSeconds ?: 0L }
        ?.let { return it }
    if (StoreVerification.Denied in outcomes) return StoreVerification.Denied
    if (StoreVerification.Invalid in outcomes) return StoreVerification.Invalid
    if (StorePurchaseOutcome.InProgress in outcomes) return StorePurchaseOutcome.InProgress
    if (StorePurchaseOutcome.Pending in outcomes) return StorePurchaseOutcome.Pending
    return StorePurchaseOutcome.NotPurchased
}
