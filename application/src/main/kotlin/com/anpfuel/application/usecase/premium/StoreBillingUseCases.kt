package com.anpfuel.application.usecase.premium

import com.anpfuel.application.port.StoreBillingPort
import com.anpfuel.application.port.StorePurchaseUpdate
import com.anpfuel.application.port.StoreQueryResult
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.StoreOffer
import com.anpfuel.domain.premium.StoreOperationId
import com.anpfuel.domain.premium.StorePlan
import com.anpfuel.domain.premium.StoreProvider
import com.anpfuel.domain.premium.StorePurchase
import com.anpfuel.domain.premium.StorePurchaseKind
import com.anpfuel.domain.premium.StorePurchaseOutcome
import com.anpfuel.domain.premium.StorePurchaseState
import com.anpfuel.domain.premium.StoreVerification
import com.anpfuel.domain.premium.selectStorePlans
import com.anpfuel.domain.premium.strongestStoreOutcome
import com.anpfuel.domain.repository.StorePurchaseGateway
import java.util.UUID
import kotlinx.coroutines.CancellationException

/**
 * In-process idempotency keys for store verification (B-BR-STORE-03).
 *
 * One fresh lowercase UUID per (owner, kind, purchase token) attempt. A
 * retryable answer keeps the ID so the retry of the same attempt reuses it;
 * a final answer drops it. A purchase already in flight is not sent twice.
 * Nothing is persisted or logged: after process death a new attempt starts.
 */
class StoreOperationIds(private val newId: () -> String = { UUID.randomUUID().toString() }) {
    class Key(val owner: String, val kind: StorePurchaseKind, val purchaseToken: String) {
        override fun equals(other: Any?): Boolean =
            other is Key && other.owner == owner && other.kind == kind && other.purchaseToken == purchaseToken

        override fun hashCode(): Int = (owner.hashCode() * 31 + kind.hashCode()) * 31 + purchaseToken.hashCode()

        override fun toString(): String = "StoreOperationIds.Key(kind=$kind)"
    }

    private val ids = HashMap<Key, String>()
    private val inFlight = HashSet<Key>()

    /** The operation ID for [key], or null when the same attempt is already in flight. */
    @Synchronized
    fun acquire(key: Key): String? {
        if (!inFlight.add(key)) return null
        return ids.getOrPut(key) {
            newId().also {
                if (!StoreOperationId.isValid(it)) {
                    inFlight.remove(key)
                    throw IllegalStateException("store.operation-id")
                }
            }
        }
    }

    @Synchronized
    fun release(key: Key, retain: Boolean) {
        inFlight.remove(key)
        if (!retain) ids.remove(key)
    }
}

/**
 * Hands one store purchase to the server for [owner] (BUC-STORE-01/02).
 *
 * Pending purchases grant nothing and are not sent (B-BR-STORE-06). The
 * signed-in account is checked before and after the request; a switch
 * discards the result. The client never acknowledges the purchase: an
 * unavailable server leaves it unacknowledged so it is sent again later.
 */
class SubmitStorePurchaseUseCase(
    private val gateway: StorePurchaseGateway,
    private val sessions: AuthSessionStore,
    private val ids: StoreOperationIds,
    private val provider: StoreProvider = StoreProvider.PLAY,
) {
    suspend operator fun invoke(owner: String, purchase: StorePurchase, kind: StorePurchaseKind): StorePurchaseOutcome {
        when (purchase.state) {
            StorePurchaseState.PENDING -> return StorePurchaseOutcome.Pending
            StorePurchaseState.UNSPECIFIED -> return StorePurchaseOutcome.NotPurchased
            StorePurchaseState.PURCHASED -> Unit
        }
        if (!StorePurchase.isValidToken(purchase.purchaseToken)) return StorePurchaseOutcome.NotPurchased
        if (owner.isBlank() || sessions.load()?.accountId != owner) return StoreVerification.OwnerChanged
        val key = StoreOperationIds.Key(owner, kind, purchase.purchaseToken)
        val operationId = ids.acquire(key) ?: return StorePurchaseOutcome.InProgress
        var retain = true
        try {
            val verdict = try {
                gateway.submit(kind, owner, operationId, purchase.purchaseToken, provider)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                StoreVerification.Unavailable
            }
            retain = verdict is StoreVerification.RetryLater ||
                verdict == StoreVerification.Unavailable ||
                verdict == StoreVerification.OwnerChanged
            if (sessions.load()?.accountId != owner) return StoreVerification.OwnerChanged
            return verdict
        } finally {
            ids.release(key, retain)
        }
    }
}

/** Store-formatted plans for the server-listed catalog; never a web price (B-BR-STORE-01). */
class LoadStorePlansUseCase(private val store: StoreBillingPort) {
    suspend operator fun invoke(offer: StoreOffer): StoreQueryResult<List<StorePlan>> {
        if (offer.provider != store.provider) return StoreQueryResult.Unavailable
        return when (val details = store.productDetails(offer.products.map { it.productId })) {
            is StoreQueryResult.Ok -> StoreQueryResult.Ok(selectStorePlans(offer, details.value))
            StoreQueryResult.Unavailable -> StoreQueryResult.Unavailable
        }
    }
}

sealed interface StoreFlowResult {
    data object Cancelled : StoreFlowResult

    data object AlreadyOwned : StoreFlowResult

    data object StoreFailed : StoreFlowResult

    data class Handled(val outcome: StorePurchaseOutcome) : StoreFlowResult
}

/** Verifies the purchases a launched store sheet reported for this account and catalog. */
class HandleStorePurchaseUpdateUseCase(private val submit: SubmitStorePurchaseUseCase) {
    suspend operator fun invoke(owner: String, offer: StoreOffer, update: StorePurchaseUpdate): StoreFlowResult =
        when (update) {
            StorePurchaseUpdate.Cancelled -> StoreFlowResult.Cancelled
            StorePurchaseUpdate.AlreadyOwned -> StoreFlowResult.AlreadyOwned
            StorePurchaseUpdate.Failed -> StoreFlowResult.StoreFailed
            is StorePurchaseUpdate.Purchases -> {
                val ours = update.purchases.filter { offer.sells(it) && it.boundTo(offer.accountBinding) }
                val outcomes = ours.map { submit(owner, it, StorePurchaseKind.VERIFY) }
                StoreFlowResult.Handled(strongestStoreOutcome(outcomes))
            }
        }
}

sealed interface StoreRestoreResult {
    data class Restored(val status: PremiumStatus) : StoreRestoreResult

    data object NothingToRestore : StoreRestoreResult

    /** The store account holds this catalog only for another app account; nothing was moved. */
    data object OtherAccount : StoreRestoreResult

    data object Pending : StoreRestoreResult

    data object InProgress : StoreRestoreResult

    data object StoreUnavailable : StoreRestoreResult

    data class NotRestored(val verification: StoreVerification) : StoreRestoreResult
}

/**
 * BUC-STORE-02: sends each owned subscription of this catalog to the signed
 * restore route. A purchase bound to another account is never sent, so a
 * restore cannot move it (B-BR-STORE-04); the server checks the binding too.
 */
class RestoreStorePurchasesUseCase(
    private val store: StoreBillingPort,
    private val submit: SubmitStorePurchaseUseCase,
) {
    suspend operator fun invoke(owner: String, offer: StoreOffer): StoreRestoreResult {
        val owned = when (val result = store.ownedSubscriptions()) {
            is StoreQueryResult.Ok -> result.value
            StoreQueryResult.Unavailable -> return StoreRestoreResult.StoreUnavailable
        }
        val catalog = owned.filter { offer.sells(it) }
        if (catalog.isEmpty()) return StoreRestoreResult.NothingToRestore
        val mine = catalog.filter { it.boundTo(offer.accountBinding) }
        if (mine.isEmpty()) return StoreRestoreResult.OtherAccount
        return when (val outcome = strongestStoreOutcome(mine.map { submit(owner, it, StorePurchaseKind.RESTORE) })) {
            is StoreVerification.Verified -> StoreRestoreResult.Restored(outcome.status)
            is StoreVerification -> StoreRestoreResult.NotRestored(outcome)
            StorePurchaseOutcome.Pending -> StoreRestoreResult.Pending
            StorePurchaseOutcome.InProgress -> StoreRestoreResult.InProgress
            StorePurchaseOutcome.NotPurchased -> StoreRestoreResult.NothingToRestore
        }
    }
}

sealed interface StoreRecoveryResult {
    data object SignedOut : StoreRecoveryResult

    data object StoreUnavailable : StoreRecoveryResult

    data object NothingToVerify : StoreRecoveryResult

    data object ServerUnavailable : StoreRecoveryResult

    /** No configured Play offer: the channel is off and nothing is sent (B-BR-STORE-10). */
    data object ChannelOff : StoreRecoveryResult

    data object OwnerChanged : StoreRecoveryResult

    data class Submitted(val outcome: StorePurchaseOutcome) : StoreRecoveryResult
}

/**
 * App start/resume recovery (B-BR-STORE-08): a completed store purchase that
 * the server has not acknowledged yet is sent to verify again, so a server
 * outage never loses it. Nothing leaves the device when no such purchase
 * exists; the status is read only to get the offer and its account binding.
 */
class RecoverStorePurchasesUseCase(
    private val store: StoreBillingPort,
    private val status: GetPremiumStatusUseCase,
    private val submit: SubmitStorePurchaseUseCase,
    private val sessions: AuthSessionStore,
) {
    suspend operator fun invoke(): StoreRecoveryResult {
        val owner = sessions.load()?.accountId?.takeIf { it.isNotBlank() } ?: return StoreRecoveryResult.SignedOut
        val owned = when (val result = store.ownedSubscriptions()) {
            is StoreQueryResult.Ok -> result.value
            StoreQueryResult.Unavailable -> return StoreRecoveryResult.StoreUnavailable
        }
        val unverified = owned.filter { it.state == StorePurchaseState.PURCHASED && !it.acknowledged }
        if (unverified.isEmpty()) return StoreRecoveryResult.NothingToVerify
        val snapshot = try {
            status()
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            return StoreRecoveryResult.ServerUnavailable
        }
        if (snapshot.accountId != owner) return StoreRecoveryResult.OwnerChanged
        val offer = snapshot.status.playOffer ?: return StoreRecoveryResult.ChannelOff
        val mine = unverified.filter { offer.sells(it) && it.boundTo(offer.accountBinding) }
        if (mine.isEmpty()) return StoreRecoveryResult.NothingToVerify
        val outcomes = mine.map { submit(owner, it, StorePurchaseKind.VERIFY) }
        return StoreRecoveryResult.Submitted(strongestStoreOutcome(outcomes))
    }
}
