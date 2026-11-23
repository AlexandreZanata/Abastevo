package com.anpfuel.domain.premium

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class StoreBillingTest {
    private val binding = "a".repeat(64)
    private val playUrl = "https://play.google.com/store/account/subscriptions?package=com.anpfuel.app"
    private val play = StoreOffer(
        provider = StoreProvider.PLAY,
        products = listOf(StoreProduct("abastevo_premium", listOf("monthly", "yearly"))),
        accountBinding = binding,
        managementUrl = playUrl,
    )
    private val appStore = StoreOffer(
        provider = StoreProvider.APP_STORE,
        products = listOf(StoreProduct("com.anpfuel.app.premium.monthly", emptyList())),
        accountBinding = "1a2b3c4d-1111-4111-8111-abcdefabcdef",
        managementUrl = "https://apps.apple.com/account/subscriptions",
    )
    private val none = PremiumStatus(
        policyVersion = 1, state = PremiumState.NONE, reason = "no-purchase", capabilities = emptySet(),
        validUntil = null, retainedDataAccess = true, paymentsConfigured = true,
        managementUrl = PremiumStatus.MANAGEMENT_URL, storeOffers = listOf(play),
    )
    private val bundle = setOf("cloud.sync", "vehicles.multiple")

    @Test
    fun `providers and sources parse only frozen wire names`() {
        assertEquals(StoreProvider.PLAY, StoreProvider.parse("play"))
        assertEquals(StoreProvider.APP_STORE, StoreProvider.parse("app_store"))
        assertNull(StoreProvider.parse("PLAY"))
        assertNull(StoreProvider.parse("stripe"))
        assertEquals(PremiumSource.PLAY, PremiumSource.parse("play"))
        assertEquals(PremiumSource.APP_STORE, PremiumSource.parse("app_store"))
        assertEquals(PremiumSource.WEB, PremiumSource.parse("web"))
        assertEquals(PremiumSource.ADMIN, PremiumSource.parse("admin"))
        assertEquals(PremiumSource.UNKNOWN, PremiumSource.parse("unknown"))
        assertEquals(PremiumSource.UNKNOWN, PremiumSource.parse("future"))
    }

    @Test
    fun `play offer requires a hex binding, a play management page and catalog ids`() {
        assertEquals(play, StoreOffer.parseOrNull(StoreProvider.PLAY, play.products, binding, playUrl))
        assertEquals(
            play.copy(managementUrl = "https://play.google.com/store/account/subscriptions"),
            StoreOffer.parseOrNull(StoreProvider.PLAY, play.products, binding, "https://play.google.com/store/account/subscriptions"),
        )
        listOf("A".repeat(64), "a".repeat(63), "a".repeat(65), "g".repeat(64), "").forEach {
            assertNull(StoreOffer.parseOrNull(StoreProvider.PLAY, play.products, it, playUrl), it)
        }
        listOf(
            "http://play.google.com/store/account/subscriptions",
            "https://play.google.com.evil.example/store/account/subscriptions",
            "https://evil.example/?u=https://play.google.com/store/account/subscriptions",
            "https://play.google.com/store/account/subscriptions#x",
            "https://play.google.com/store/account/subscriptions?package=a b",
            "https://example.com/checkout",
        ).forEach { assertNull(StoreOffer.parseOrNull(StoreProvider.PLAY, play.products, binding, it), it) }
        listOf(
            emptyList(),
            listOf(StoreProduct("Abastevo", listOf("monthly"))),
            listOf(StoreProduct("abastevo_premium", emptyList())),
            listOf(StoreProduct("abastevo_premium", listOf("Monthly"))),
            listOf(StoreProduct("abastevo_premium", listOf("monthly", "monthly"))),
            listOf(StoreProduct("a", listOf("m")), StoreProduct("a", listOf("y"))),
            (0..16).map { StoreProduct("p$it", listOf("m")) },
            listOf(StoreProduct("abastevo_premium", (0..16).map { "p$it" })),
        ).forEach { assertNull(StoreOffer.parseOrNull(StoreProvider.PLAY, it, binding, playUrl), it.toString()) }
    }

    @Test
    fun `app store offer keeps its own binding and management page`() {
        assertEquals(appStore, StoreOffer.parseOrNull(appStore.provider, appStore.products, appStore.accountBinding, appStore.managementUrl))
        assertNull(StoreOffer.parseOrNull(StoreProvider.APP_STORE, appStore.products, binding, appStore.managementUrl))
        assertNull(StoreOffer.parseOrNull(StoreProvider.APP_STORE, appStore.products, appStore.accountBinding.uppercase(), appStore.managementUrl))
        assertNull(StoreOffer.parseOrNull(StoreProvider.APP_STORE, appStore.products, appStore.accountBinding, playUrl))
    }

    @Test
    fun `status exposes exactly one play offer and fails closed when ambiguous`() {
        assertEquals(play, none.playOffer)
        assertEquals(play, none.copy(storeOffers = listOf(appStore, play)).playOffer)
        assertNull(none.copy(storeOffers = listOf(appStore)).playOffer)
        assertNull(none.copy(storeOffers = emptyList()).playOffer)
        assertNull(none.copy(storeOffers = listOf(play, play.copy(accountBinding = "b".repeat(64)))).playOffer)
        assertThrows<IllegalArgumentException> { none.copy(storeOffers = List(9) { appStore }) }
        assertThrows<IllegalArgumentException> { none.copy(tiers = List(17) { PremiumTier("PRO", PremiumSource.WEB, null) }) }
        assertThrows<IllegalArgumentException> { PremiumTier("", PremiumSource.WEB, null) }
        assertThrows<IllegalArgumentException> { PremiumTier("x".repeat(33), PremiumSource.WEB, null) }
    }

    @Test
    fun `purchase is offered only for a configured offer and a state without paid access`() {
        assertEquals(PurchaseEligibility.AVAILABLE, none.purchaseEligibility())
        assertEquals(PurchaseEligibility.AVAILABLE, none.copy(state = PremiumState.EXPIRED).purchaseEligibility())
        assertEquals(PurchaseEligibility.AVAILABLE, none.copy(state = PremiumState.REVOKED).purchaseEligibility())
        assertEquals(PurchaseEligibility.NO_OFFER, none.copy(storeOffers = emptyList()).purchaseEligibility())
        assertEquals(PurchaseEligibility.NO_OFFER, none.copy(storeOffers = listOf(appStore)).purchaseEligibility())
        assertEquals(PurchaseEligibility.PENDING, none.copy(state = PremiumState.PENDING).purchaseEligibility())
        assertEquals(PurchaseEligibility.UNKNOWN_STATE, none.copy(state = PremiumState.UNKNOWN).purchaseEligibility())
        listOf(PremiumState.ACTIVE, PremiumState.CANCELED, PremiumState.GRACE, PremiumState.HOLD, PremiumState.PAUSED).forEach {
            assertEquals(PurchaseEligibility.MANAGED_IN_STORE, none.copy(state = it).purchaseEligibility(), it.name)
        }
    }

    @Test
    fun `active access from another channel never offers a second purchase`() {
        val web = none.copy(
            state = PremiumState.ACTIVE, reason = "verified-web", capabilities = bundle,
            tiers = listOf(PremiumTier("PRO", PremiumSource.WEB, "2026-12-31T00:00:00Z")),
        )
        assertEquals(PurchaseEligibility.ACTIVE_FROM_OTHER_SOURCE, web.purchaseEligibility())
        assertEquals(PurchaseEligibility.ACTIVE_FROM_OTHER_SOURCE, web.copy(storeOffers = emptyList()).purchaseEligibility())
        assertEquals(
            PurchaseEligibility.ACTIVE_FROM_OTHER_SOURCE,
            web.copy(tiers = listOf(PremiumTier("PRO", PremiumSource.ADMIN, null))).purchaseEligibility(),
        )
        assertEquals(
            PurchaseEligibility.ACTIVE_FROM_OTHER_SOURCE,
            web.copy(tiers = listOf(PremiumTier("PRO", PremiumSource.APP_STORE, null))).purchaseEligibility(),
        )
        val playAndWeb = web.copy(tiers = web.tiers + PremiumTier("PRO", PremiumSource.PLAY, "2026-12-01T00:00:00Z"))
        assertEquals(PurchaseEligibility.MANAGED_IN_STORE, playAndWeb.purchaseEligibility())
        assertEquals(PurchaseEligibility.MANAGED_IN_STORE, web.copy(tiers = emptyList()).purchaseEligibility())
    }

    @Test
    fun `billing periods parse iso durations only`() {
        assertEquals(StoreBillingPeriod(StoreBillingPeriod.PeriodUnit.MONTH, 1), StoreBillingPeriod.parse("P1M"))
        assertEquals(StoreBillingPeriod(StoreBillingPeriod.PeriodUnit.YEAR, 1), StoreBillingPeriod.parse("P1Y"))
        assertEquals(StoreBillingPeriod(StoreBillingPeriod.PeriodUnit.WEEK, 1), StoreBillingPeriod.parse("P1W"))
        assertEquals(StoreBillingPeriod(StoreBillingPeriod.PeriodUnit.MONTH, 6), StoreBillingPeriod.parse("P6M"))
        assertEquals(StoreBillingPeriod(StoreBillingPeriod.PeriodUnit.DAY, 3), StoreBillingPeriod.parse("P3D"))
        listOf("", "P", "P0M", "1M", "P1H", "PT1H", "P1M1D", "P1000Y", "p1m").forEach { assertNull(StoreBillingPeriod.parse(it), it) }
    }

    @Test
    fun `plans keep server order and only base plans with a recurring price`() {
        val monthlyBase = StoreSubscriptionOffer(
            "monthly", null, "tok-m",
            listOf(StorePricePhase("R$ 9,90", "P1M", infiniteRecurring = true)),
        )
        val monthlyTrial = StoreSubscriptionOffer(
            "monthly", "trial", "tok-trial",
            listOf(
                StorePricePhase("Free", "P1W", infiniteRecurring = false),
                StorePricePhase("R$ 9,90", "P1M", infiniteRecurring = true),
            ),
        )
        val yearlyBase = StoreSubscriptionOffer(
            "yearly", null, "tok-y",
            listOf(StorePricePhase("R$ 99,00", "P1Y", infiniteRecurring = true)),
        )
        val unlisted = StoreSubscriptionOffer("weekly", null, "tok-w", listOf(StorePricePhase("R$ 3,00", "P1W", true)))
        val prepaid = StoreSubscriptionOffer("yearly", null, "tok-p", listOf(StorePricePhase("R$ 80,00", "P1Y", false)))
        val catalog = listOf(
            StoreProductDetails("other_product", listOf(monthlyBase)),
            StoreProductDetails("abastevo_premium", listOf(unlisted, yearlyBase, monthlyTrial, monthlyBase)),
        )

        val plans = selectStorePlans(play, catalog)

        assertEquals(listOf("monthly", "yearly"), plans.map { it.basePlanId })
        assertEquals(listOf("tok-m", "tok-y"), plans.map { it.offerToken })
        assertEquals("R$ 9,90", plans[0].formattedPrice)
        assertEquals(StoreBillingPeriod(StoreBillingPeriod.PeriodUnit.YEAR, 1), plans[1].billingPeriod)
        assertTrue(plans.all { it.productId == "abastevo_premium" })
        assertTrue(selectStorePlans(play, listOf(StoreProductDetails("abastevo_premium", listOf(prepaid)))).isEmpty())
        assertTrue(selectStorePlans(play, emptyList()).isEmpty())
        val blank = StoreSubscriptionOffer("monthly", null, "", listOf(StorePricePhase("R$ 9,90", "P1M", true)))
        assertTrue(selectStorePlans(play, listOf(StoreProductDetails("abastevo_premium", listOf(blank)))).isEmpty())
        val duplicated = listOf(StoreProductDetails("abastevo_premium", listOf(monthlyBase, monthlyBase.copy(offerToken = "tok-2"))))
        assertEquals(listOf("tok-m"), selectStorePlans(play, duplicated).map { it.offerToken })
    }

    @Test
    fun `purchase tokens are validated and never printed`() {
        val purchase = StorePurchase("secret-token", listOf("abastevo_premium"), StorePurchaseState.PURCHASED, false, binding)
        assertFalse(purchase.toString().contains("secret-token"))
        assertTrue(StorePurchase.isValidToken("secret-token"))
        assertFalse(StorePurchase.isValidToken(""))
        assertFalse(StorePurchase.isValidToken("has space"))
        assertFalse(StorePurchase.isValidToken("x".repeat(4097)))
        assertTrue(StorePurchase.isValidToken("x".repeat(4096)))
        assertTrue(purchase.boundTo(binding))
        assertTrue(purchase.copy(accountBinding = null).boundTo(binding))
        assertFalse(purchase.copy(accountBinding = "b".repeat(64)).boundTo(binding))
    }

    @Test
    fun `operation ids are fresh lowercase uuids`() {
        assertTrue(StoreOperationId.isValid("11111111-1111-4111-8111-111111111111"))
        assertFalse(StoreOperationId.isValid("11111111-1111-4111-8111-11111111111A"))
        assertFalse(StoreOperationId.isValid("not-a-uuid"))
        assertFalse(StoreOperationId.isValid("111111111111-4111-8111-1111111111111"))
    }

    @Test
    fun `the most relevant outcome wins when several purchases are handed over`() {
        val verified = StoreVerification.Verified(none.copy(state = PremiumState.ACTIVE), replayed = false)
        val later = StoreVerification.Verified(none.copy(state = PremiumState.CANCELED), replayed = true)
        assertEquals(StorePurchaseOutcome.NotPurchased, strongestStoreOutcome(emptyList()))
        assertEquals(later, strongestStoreOutcome(listOf(verified, StoreVerification.Denied, later)))
        assertEquals(
            StoreVerification.OwnerChanged,
            strongestStoreOutcome(listOf(StoreVerification.Unavailable, StoreVerification.OwnerChanged)),
        )
        assertEquals(
            StoreVerification.SignInRequired,
            strongestStoreOutcome(listOf(StoreVerification.Unavailable, StoreVerification.SignInRequired)),
        )
        assertEquals(
            StoreVerification.Unavailable,
            strongestStoreOutcome(listOf(StoreVerification.RetryLater(5), StoreVerification.Unavailable, StoreVerification.Denied)),
        )
        assertEquals(
            StoreVerification.RetryLater(9),
            strongestStoreOutcome(listOf(StoreVerification.RetryLater(5), StoreVerification.RetryLater(9), StoreVerification.Denied)),
        )
        assertEquals(StoreVerification.Denied, strongestStoreOutcome(listOf(StoreVerification.Invalid, StoreVerification.Denied)))
        assertEquals(StoreVerification.Invalid, strongestStoreOutcome(listOf(StorePurchaseOutcome.Pending, StoreVerification.Invalid)))
        assertEquals(
            StorePurchaseOutcome.InProgress,
            strongestStoreOutcome(listOf(StorePurchaseOutcome.Pending, StorePurchaseOutcome.InProgress)),
        )
        assertEquals(
            StorePurchaseOutcome.Pending,
            strongestStoreOutcome(listOf(StorePurchaseOutcome.NotPurchased, StorePurchaseOutcome.Pending)),
        )
        assertEquals(StorePurchaseOutcome.NotPurchased, strongestStoreOutcome(listOf(StorePurchaseOutcome.NotPurchased)))
    }

    @Test
    fun `store purchases outside the server catalog are not ours to send`() {
        val purchase = StorePurchase("t", listOf("abastevo_premium"), StorePurchaseState.PURCHASED, false, binding)
        assertTrue(play.sells(purchase))
        assertFalse(play.sells(purchase.copy(productIds = listOf("retired_product"))))
        assertFalse(play.sells(purchase.copy(productIds = emptyList())))
    }
}
