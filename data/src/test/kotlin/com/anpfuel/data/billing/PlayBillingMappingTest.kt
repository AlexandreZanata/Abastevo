package com.anpfuel.data.billing

import com.android.billingclient.api.BillingClient.BillingResponseCode
import com.android.billingclient.api.Purchase
import com.anpfuel.application.port.StoreLaunchResult
import com.anpfuel.application.port.StorePurchaseUpdate
import com.anpfuel.domain.premium.StorePurchase
import com.anpfuel.domain.premium.StorePurchaseState
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class PlayBillingMappingTest {

    @Test fun `purchase states keep pending distinct and never grant unknown states`() {
        assertEquals(StorePurchaseState.PURCHASED, playPurchaseState(Purchase.PurchaseState.PURCHASED))
        assertEquals(StorePurchaseState.PENDING, playPurchaseState(Purchase.PurchaseState.PENDING))
        assertEquals(StorePurchaseState.UNSPECIFIED, playPurchaseState(Purchase.PurchaseState.UNSPECIFIED_STATE))
        assertEquals(StorePurchaseState.UNSPECIFIED, playPurchaseState(99))
    }

    @Test fun `launch results separate cancel, ownership and unavailability`() {
        assertEquals(StoreLaunchResult.LAUNCHED, playLaunchResult(BillingResponseCode.OK))
        assertEquals(StoreLaunchResult.CANCELLED, playLaunchResult(BillingResponseCode.USER_CANCELED))
        assertEquals(StoreLaunchResult.ALREADY_OWNED, playLaunchResult(BillingResponseCode.ITEM_ALREADY_OWNED))
        listOf(
            BillingResponseCode.SERVICE_UNAVAILABLE,
            BillingResponseCode.BILLING_UNAVAILABLE,
            BillingResponseCode.SERVICE_DISCONNECTED,
            BillingResponseCode.NETWORK_ERROR,
            BillingResponseCode.ITEM_UNAVAILABLE,
            BillingResponseCode.FEATURE_NOT_SUPPORTED,
        ).forEach { assertEquals(StoreLaunchResult.UNAVAILABLE, playLaunchResult(it), it.toString()) }
        assertEquals(StoreLaunchResult.FAILED, playLaunchResult(BillingResponseCode.DEVELOPER_ERROR))
        assertEquals(StoreLaunchResult.FAILED, playLaunchResult(BillingResponseCode.ERROR))
    }

    @Test fun `purchase updates carry purchases only on success`() {
        val purchase = StorePurchase("t", listOf("abastevo_premium"), StorePurchaseState.PURCHASED, false, null)
        assertEquals(StorePurchaseUpdate.Purchases(listOf(purchase)), playPurchaseUpdate(BillingResponseCode.OK, listOf(purchase)))
        assertEquals(StorePurchaseUpdate.Purchases(emptyList()), playPurchaseUpdate(BillingResponseCode.OK, null))
        assertEquals(StorePurchaseUpdate.Cancelled, playPurchaseUpdate(BillingResponseCode.USER_CANCELED, null))
        assertEquals(StorePurchaseUpdate.AlreadyOwned, playPurchaseUpdate(BillingResponseCode.ITEM_ALREADY_OWNED, null))
        assertEquals(StorePurchaseUpdate.Failed, playPurchaseUpdate(BillingResponseCode.ERROR, listOf(purchase)))
    }
}
