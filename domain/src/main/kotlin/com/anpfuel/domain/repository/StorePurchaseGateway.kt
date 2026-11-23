package com.anpfuel.domain.repository

import com.anpfuel.domain.premium.StoreProvider
import com.anpfuel.domain.premium.StorePurchaseKind
import com.anpfuel.domain.premium.StoreVerification

/**
 * Signed store purchase verification and restore (`POST /v1/premium/purchases/{verify,restore}`).
 *
 * The request carries the current session proof for [owner] and is signed by
 * the device key bound to that account. The client never acknowledges or
 * finishes the purchase itself (B-BR-STORE-08); the server verifies with the
 * store and returns the merged status. Implementations map every failure to a
 * [StoreVerification] and never log the purchase token.
 */
interface StorePurchaseGateway {
    suspend fun submit(
        kind: StorePurchaseKind,
        owner: String,
        operationId: String,
        purchaseToken: String,
        provider: StoreProvider,
    ): StoreVerification
}
