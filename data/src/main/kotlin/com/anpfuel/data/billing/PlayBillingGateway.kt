package com.anpfuel.data.billing

import android.app.Activity
import android.content.Context
import com.android.billingclient.api.BillingClient
import com.android.billingclient.api.BillingClientStateListener
import com.android.billingclient.api.BillingFlowParams
import com.android.billingclient.api.BillingResult
import com.android.billingclient.api.PendingPurchasesParams
import com.android.billingclient.api.ProductDetails
import com.android.billingclient.api.Purchase
import com.android.billingclient.api.PurchasesUpdatedListener
import com.android.billingclient.api.QueryProductDetailsParams
import com.android.billingclient.api.QueryPurchasesParams
import com.anpfuel.application.port.StoreBillingPort
import com.anpfuel.application.port.StoreLaunchResult
import com.anpfuel.application.port.StorePurchaseHost
import com.anpfuel.application.port.StorePurchaseUpdate
import com.anpfuel.application.port.StoreQueryResult
import com.anpfuel.domain.premium.StorePlan
import com.anpfuel.domain.premium.StorePricePhase
import com.anpfuel.domain.premium.StoreProductDetails
import com.anpfuel.domain.premium.StoreProvider
import com.anpfuel.domain.premium.StorePurchase
import com.anpfuel.domain.premium.StorePurchaseState
import com.anpfuel.domain.premium.StoreSubscriptionOffer
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.concurrent.ConcurrentHashMap
import javax.inject.Inject
import javax.inject.Singleton
import kotlin.coroutines.resume
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull

/**
 * Google Play Billing adapter (B-BR-STORE-01/04/06/08).
 *
 * One app-scoped [BillingClient] with automatic service reconnection. It
 * lists subscription product details, opens the purchase sheet with the
 * server's account binding as `obfuscatedAccountId`, reports purchase
 * updates and lists owned subscriptions. It never acknowledges or consumes
 * a purchase: the server acknowledges after verification. Pending purchases
 * are enabled and reported as pending; they grant nothing.
 */
@Singleton
class PlayBillingGateway internal constructor(
    private val clientFactory: (PurchasesUpdatedListener) -> BillingClient,
) : StoreBillingPort {

    @Inject constructor(@ApplicationContext context: Context) : this({ listener ->
        BillingClient.newBuilder(context)
            .setListener(listener)
            .enablePendingPurchases(PendingPurchasesParams.newBuilder().enableOneTimeProducts().build())
            .enableAutoServiceReconnection()
            .build()
    })

    override val provider: StoreProvider = StoreProvider.PLAY

    private val updates = MutableSharedFlow<StorePurchaseUpdate>(extraBufferCapacity = UPDATE_BUFFER)
    override val purchaseUpdates: Flow<StorePurchaseUpdate> = updates.asSharedFlow()

    private val listener = PurchasesUpdatedListener { result, purchases ->
        updates.tryEmit(playPurchaseUpdate(result.responseCode, purchases?.map(::toStorePurchase)))
    }
    private val client: BillingClient by lazy { clientFactory(listener) }
    private val connection = Mutex()
    private val productCache = ConcurrentHashMap<String, ProductDetails>()

    override suspend fun productDetails(productIds: List<String>): StoreQueryResult<List<StoreProductDetails>> {
        if (productIds.isEmpty()) return StoreQueryResult.Ok(emptyList())
        if (!connected()) return StoreQueryResult.Unavailable
        val params = QueryProductDetailsParams.newBuilder()
            .setProductList(
                productIds.map { id ->
                    QueryProductDetailsParams.Product.newBuilder()
                        .setProductId(id)
                        .setProductType(BillingClient.ProductType.SUBS)
                        .build()
                },
            )
            .build()
        val (code, details) = suspendCancellableCoroutine<Pair<Int, List<ProductDetails>>> { continuation ->
            client.queryProductDetailsAsync(params) { result, query ->
                if (continuation.isActive) continuation.resume(result.responseCode to query.productDetailsList.orEmpty())
            }
        }
        if (code != BillingClient.BillingResponseCode.OK) return StoreQueryResult.Unavailable
        details.forEach { productCache[it.productId] = it }
        return StoreQueryResult.Ok(details.map(::toStoreProduct))
    }

    override suspend fun launchPurchase(
        host: StorePurchaseHost,
        plan: StorePlan,
        accountBinding: String,
    ): StoreLaunchResult {
        val activity = host.handle as? Activity ?: return StoreLaunchResult.FAILED
        if (!connected()) return StoreLaunchResult.UNAVAILABLE
        if (productCache[plan.productId] == null) productDetails(listOf(plan.productId))
        val details = productCache[plan.productId] ?: return StoreLaunchResult.UNAVAILABLE
        if (details.subscriptionOfferDetails.orEmpty().none { it.offerToken == plan.offerToken }) {
            return StoreLaunchResult.UNAVAILABLE
        }
        val params = BillingFlowParams.newBuilder()
            .setProductDetailsParamsList(
                listOf(
                    BillingFlowParams.ProductDetailsParams.newBuilder()
                        .setProductDetails(details)
                        .setOfferToken(plan.offerToken)
                        .build(),
                ),
            )
            .setObfuscatedAccountId(accountBinding)
            .build()
        val result = withContext(Dispatchers.Main) { client.launchBillingFlow(activity, params) }
        return playLaunchResult(result.responseCode)
    }

    override suspend fun ownedSubscriptions(): StoreQueryResult<List<StorePurchase>> {
        if (!connected()) return StoreQueryResult.Unavailable
        val params = QueryPurchasesParams.newBuilder().setProductType(BillingClient.ProductType.SUBS).build()
        val (code, purchases) = suspendCancellableCoroutine<Pair<Int, List<Purchase>>> { continuation ->
            client.queryPurchasesAsync(params) { result, list ->
                if (continuation.isActive) continuation.resume(result.responseCode to list.orEmpty())
            }
        }
        if (code != BillingClient.BillingResponseCode.OK) return StoreQueryResult.Unavailable
        return StoreQueryResult.Ok(purchases.map(::toStorePurchase))
    }

    private suspend fun connected(): Boolean = connection.withLock {
        if (client.isReady) return@withLock true
        withTimeoutOrNull(CONNECT_TIMEOUT_MILLIS) {
            withContext(Dispatchers.Main) {
                suspendCancellableCoroutine<Boolean> { continuation ->
                    client.startConnection(object : BillingClientStateListener {
                        override fun onBillingSetupFinished(result: BillingResult) {
                            if (continuation.isActive) {
                                continuation.resume(result.responseCode == BillingClient.BillingResponseCode.OK)
                            }
                        }

                        override fun onBillingServiceDisconnected() {
                            if (continuation.isActive) continuation.resume(false)
                        }
                    })
                }
            }
        } ?: false
    }

    private companion object {
        const val UPDATE_BUFFER = 8
        const val CONNECT_TIMEOUT_MILLIS = 10_000L
    }
}

internal fun toStoreProduct(details: ProductDetails): StoreProductDetails = StoreProductDetails(
    productId = details.productId,
    offers = details.subscriptionOfferDetails.orEmpty().map { offer ->
        StoreSubscriptionOffer(
            basePlanId = offer.basePlanId,
            offerId = offer.offerId,
            offerToken = offer.offerToken,
            phases = offer.pricingPhases.pricingPhaseList.map { phase ->
                StorePricePhase(
                    formattedPrice = phase.formattedPrice,
                    billingPeriod = phase.billingPeriod,
                    infiniteRecurring = phase.recurrenceMode == ProductDetails.RecurrenceMode.INFINITE_RECURRING,
                )
            },
        )
    },
)

internal fun toStorePurchase(purchase: Purchase): StorePurchase = StorePurchase(
    purchaseToken = purchase.purchaseToken,
    productIds = purchase.products.toList(),
    state = playPurchaseState(purchase.purchaseState),
    acknowledged = purchase.isAcknowledged,
    accountBinding = purchase.accountIdentifiers?.obfuscatedAccountId,
)

internal fun playPurchaseState(state: Int): StorePurchaseState = when (state) {
    Purchase.PurchaseState.PURCHASED -> StorePurchaseState.PURCHASED
    Purchase.PurchaseState.PENDING -> StorePurchaseState.PENDING
    else -> StorePurchaseState.UNSPECIFIED
}

private val unavailableCodes = setOf(
    BillingClient.BillingResponseCode.SERVICE_UNAVAILABLE,
    BillingClient.BillingResponseCode.BILLING_UNAVAILABLE,
    BillingClient.BillingResponseCode.SERVICE_DISCONNECTED,
    BillingClient.BillingResponseCode.NETWORK_ERROR,
    BillingClient.BillingResponseCode.ITEM_UNAVAILABLE,
    BillingClient.BillingResponseCode.FEATURE_NOT_SUPPORTED,
)

internal fun playLaunchResult(code: Int): StoreLaunchResult = when (code) {
    BillingClient.BillingResponseCode.OK -> StoreLaunchResult.LAUNCHED
    BillingClient.BillingResponseCode.USER_CANCELED -> StoreLaunchResult.CANCELLED
    BillingClient.BillingResponseCode.ITEM_ALREADY_OWNED -> StoreLaunchResult.ALREADY_OWNED
    in unavailableCodes -> StoreLaunchResult.UNAVAILABLE
    else -> StoreLaunchResult.FAILED
}

internal fun playPurchaseUpdate(code: Int, purchases: List<StorePurchase>?): StorePurchaseUpdate = when (code) {
    BillingClient.BillingResponseCode.OK -> StorePurchaseUpdate.Purchases(purchases.orEmpty())
    BillingClient.BillingResponseCode.USER_CANCELED -> StorePurchaseUpdate.Cancelled
    BillingClient.BillingResponseCode.ITEM_ALREADY_OWNED -> StorePurchaseUpdate.AlreadyOwned
    else -> StorePurchaseUpdate.Failed
}
