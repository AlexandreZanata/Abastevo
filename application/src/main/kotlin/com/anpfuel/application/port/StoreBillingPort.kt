package com.anpfuel.application.port

import com.anpfuel.domain.premium.StorePlan
import com.anpfuel.domain.premium.StoreProductDetails
import com.anpfuel.domain.premium.StoreProvider
import com.anpfuel.domain.premium.StorePurchase
import kotlinx.coroutines.flow.Flow

/** Opaque presentation host for the store purchase sheet (an Android Activity for Google Play). */
class StorePurchaseHost(val handle: Any)

/** Result of a store query; [Unavailable] means the store is missing, disconnected or refused it. */
sealed interface StoreQueryResult<out T> {
    data class Ok<T>(val value: T) : StoreQueryResult<T>

    data object Unavailable : StoreQueryResult<Nothing>
}

enum class StoreLaunchResult {
    /** The store sheet is shown; the result arrives through [StoreBillingPort.purchaseUpdates]. */
    LAUNCHED,
    CANCELLED,
    ALREADY_OWNED,
    UNAVAILABLE,
    FAILED,
}

/** Store callback after a launched purchase sheet. */
sealed interface StorePurchaseUpdate {
    data class Purchases(val purchases: List<StorePurchase>) : StorePurchaseUpdate

    data object Cancelled : StorePurchaseUpdate

    data object AlreadyOwned : StorePurchaseUpdate

    data object Failed : StorePurchaseUpdate
}

/**
 * The app's own store (B-BR-STORE-01): Google Play Billing on Android.
 *
 * The port never acknowledges, consumes or finishes a purchase; only the
 * server does that after verification (B-BR-STORE-08).
 */
interface StoreBillingPort {
    val provider: StoreProvider

    val purchaseUpdates: Flow<StorePurchaseUpdate>

    suspend fun productDetails(productIds: List<String>): StoreQueryResult<List<StoreProductDetails>>

    /** Opens the store sheet for [plan] bound to the account with [accountBinding] (B-BR-STORE-04). */
    suspend fun launchPurchase(host: StorePurchaseHost, plan: StorePlan, accountBinding: String): StoreLaunchResult

    /** Current subscriptions owned by the store account on this device. */
    suspend fun ownedSubscriptions(): StoreQueryResult<List<StorePurchase>>
}
