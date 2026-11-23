package com.anpfuel.app.ui.premium

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.port.StoreBillingPort
import com.anpfuel.application.port.StoreLaunchResult
import com.anpfuel.application.port.StorePurchaseHost
import com.anpfuel.application.port.StorePurchaseUpdate
import com.anpfuel.application.port.StoreQueryResult
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.premium.GetPremiumStatusUseCase
import com.anpfuel.application.usecase.premium.HandleStorePurchaseUpdateUseCase
import com.anpfuel.application.usecase.premium.LoadStorePlansUseCase
import com.anpfuel.application.usecase.premium.RecoverStorePurchasesUseCase
import com.anpfuel.application.usecase.premium.RestoreStorePurchasesUseCase
import com.anpfuel.application.usecase.premium.StoreFlowResult
import com.anpfuel.application.usecase.premium.StoreRecoveryResult
import com.anpfuel.application.usecase.premium.StoreRestoreResult
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusException
import com.anpfuel.domain.premium.PremiumStatusSnapshot
import com.anpfuel.domain.premium.PurchaseEligibility
import com.anpfuel.domain.premium.StoreOffer
import com.anpfuel.domain.premium.StorePlan
import com.anpfuel.domain.premium.StorePurchaseOutcome
import com.anpfuel.domain.premium.StoreVerification
import com.anpfuel.domain.premium.purchaseEligibility
import dagger.hilt.android.lifecycle.HiltViewModel
import java.io.IOException
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Honest Premium entry and purchase states; no local entitlement is ever inferred. */
enum class PremiumNotice {
    OFFLINE,
    DENIED,
    ERROR,
    RESTORE_UNAVAILABLE,
    STORE_FAILED,
    PURCHASE_PENDING,
    PURCHASE_CANCELLED,
    PURCHASE_VERIFIED,
    PURCHASE_DENIED,
    PURCHASE_RETRY,
    PURCHASE_STORE_ERROR,
    PURCHASE_ALREADY_OWNED,
    PURCHASE_OWNER_CHANGED,
    RESTORE_DONE,
    RESTORE_NONE,
    RESTORE_OTHER_ACCOUNT,
}

/** A store or server step is running; purchase and restore actions are disabled meanwhile. */
enum class PremiumPurchasePhase { IDLE, LAUNCHING, VERIFYING, RESTORING }

data class PremiumUiState(
    val signedOut: Boolean = true,
    val loading: Boolean = false,
    val status: PremiumStatus? = null,
    val notice: PremiumNotice? = null,
    val plans: List<StorePlan> = emptyList(),
    val plansLoading: Boolean = false,
    val plansUnavailable: Boolean = false,
    val purchasePhase: PremiumPurchasePhase = PremiumPurchasePhase.IDLE,
    /** Google Play reported a pending payment in this session; it grants nothing. */
    val storePending: Boolean = false,
) {
    /** Server-derived; NO_OFFER keeps the page to status only (B-BR-STORE-10). */
    val eligibility: PurchaseEligibility
        get() = status?.purchaseEligibility() ?: PurchaseEligibility.NO_OFFER

    val purchaseBusy: Boolean
        get() = purchasePhase != PremiumPurchasePhase.IDLE

    val canRestore: Boolean
        get() = status?.playOffer != null

    /** The store's own management page, or the frozen Play page when no offer is listed. */
    val managementUrl: String
        get() = status?.playOffer?.managementUrl ?: PremiumStatus.MANAGEMENT_URL
}

private val purchaseNotices = setOf(
    PremiumNotice.PURCHASE_PENDING,
    PremiumNotice.PURCHASE_CANCELLED,
    PremiumNotice.PURCHASE_VERIFIED,
    PremiumNotice.PURCHASE_DENIED,
    PremiumNotice.PURCHASE_RETRY,
    PremiumNotice.PURCHASE_STORE_ERROR,
    PremiumNotice.PURCHASE_ALREADY_OWNED,
    PremiumNotice.PURCHASE_OWNER_CHANGED,
    PremiumNotice.RESTORE_DONE,
    PremiumNotice.RESTORE_NONE,
    PremiumNotice.RESTORE_OTHER_ACCOUNT,
)

/**
 * MEX-16 verified Premium status with Google Play purchases (BUC-STORE-01/02).
 *
 * The status always comes from the server. A purchase starts only when the
 * server lists a Play offer and the account has no paid access; the store
 * result is handed to the signed verify route and only the server's answer
 * changes the status. Account changes and late responses are fenced by
 * generation and owner comparison: a result for another owner is discarded.
 */
@HiltViewModel
class PremiumViewModel @Inject constructor(
    private val status: GetPremiumStatusUseCase,
    private val sessions: AuthSessionStore,
    private val cache: PremiumStatusCache,
    private val store: StoreBillingPort,
    private val loadPlans: LoadStorePlansUseCase,
    private val handleUpdate: HandleStorePurchaseUpdateUseCase,
    private val restorePurchases: RestoreStorePurchasesUseCase,
    private val recoverPurchases: RecoverStorePurchasesUseCase,
) : ViewModel() {
    private val state = MutableStateFlow(PremiumUiState(signedOut = false, loading = true))
    val uiState = state.asStateFlow()
    private var generation = 0L
    private var statusOwner: String? = null
    private var plansGeneration = 0L
    private var plansOffer: StoreOffer? = null
    private var purchaseGeneration = 0L
    private var launch: LaunchContext? = null

    private class LaunchContext(val owner: String, val offer: StoreOffer, val attempt: Long)

    init {
        viewModelScope.launch {
            store.purchaseUpdates.collect { onStoreUpdate(it) }
        }
    }

    fun refresh(force: Boolean = false) {
        val request = ++generation
        val owner = sessions.load()?.accountId
        if (owner == null) {
            cache.clear()
            forgetOwner()
            state.value = PremiumUiState(signedOut = true)
            return
        }
        if (!force) {
            val cached = cache.get(owner, System.currentTimeMillis())
            if (cached != null) {
                showStatus(owner, cached)
                return
            }
        }
        val keepVisibleStatus = force && statusOwner == owner && state.value.status != null
        state.value = when {
            keepVisibleStatus -> state.value.copy(signedOut = false, loading = true, notice = keptNotice())
            // Same owner (e.g. back from the Play sheet): keep the purchase in progress.
            statusOwner == owner -> state.value.copy(signedOut = false, loading = true, status = null, notice = keptNotice())
            else -> {
                forgetOwner()
                statusOwner = owner
                PremiumUiState(signedOut = false, loading = true)
            }
        }
        viewModelScope.launch {
            try {
                val snapshot = status.invoke()
                if (request != generation) return@launch
                if (sessions.load()?.accountId != owner || snapshot.accountId != owner) {
                    cache.clear()
                    forgetOwner()
                    state.value = PremiumUiState(signedOut = true)
                    return@launch
                }
                cache.put(snapshot, System.currentTimeMillis())
                showStatus(owner, snapshot.status)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (error: Exception) {
                if (request != generation) return@launch
                cache.clear()
                if (sessions.load()?.accountId != owner) {
                    forgetOwner()
                    state.value = PremiumUiState(signedOut = true)
                    return@launch
                }
                state.value = state.value.copy(
                    loading = false,
                    status = null,
                    notice = noticeFor(error),
                    plans = emptyList(),
                    plansLoading = false,
                    plansUnavailable = false,
                )
                plansOffer = null
            }
        }
    }

    fun forceRefresh() {
        refresh(force = true)
    }

    fun invalidateCache() {
        cache.clear()
    }

    fun clearNotice() {
        state.value = state.value.copy(notice = null)
    }

    fun restoreUnavailable() {
        state.value = state.value.copy(notice = PremiumNotice.RESTORE_UNAVAILABLE)
    }

    fun storeOpenFailed() {
        state.value = state.value.copy(notice = PremiumNotice.STORE_FAILED)
    }

    fun retryPlans() {
        val owner = statusOwner ?: return
        val verified = state.value.status ?: return
        plansOffer = null
        loadPlansFor(owner, verified)
    }

    /** BUC-STORE-01: opens the Play sheet for [plan] bound to the current account. */
    fun subscribe(host: StorePurchaseHost, plan: StorePlan) {
        val current = state.value
        val verified = current.status ?: return
        val offer = verified.playOffer ?: return
        if (current.purchaseBusy || current.storePending || current.eligibility != PurchaseEligibility.AVAILABLE) return
        if (plan !in current.plans) return
        val owner = sessions.load()?.accountId ?: return
        if (owner != statusOwner) return
        val attempt = ++purchaseGeneration
        launch = LaunchContext(owner, offer, attempt)
        state.value = current.copy(purchasePhase = PremiumPurchasePhase.LAUNCHING, notice = null)
        viewModelScope.launch {
            val result = try {
                store.launchPurchase(host, plan, offer.accountBinding)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                StoreLaunchResult.FAILED
            }
            if (attempt != purchaseGeneration) return@launch
            when (result) {
                StoreLaunchResult.LAUNCHED -> Unit
                StoreLaunchResult.CANCELLED -> finishPurchase(PremiumNotice.PURCHASE_CANCELLED)
                StoreLaunchResult.ALREADY_OWNED -> finishPurchase(PremiumNotice.PURCHASE_ALREADY_OWNED)
                StoreLaunchResult.UNAVAILABLE,
                StoreLaunchResult.FAILED,
                -> finishPurchase(PremiumNotice.PURCHASE_STORE_ERROR)
            }
        }
    }

    /** BUC-STORE-02: sends this account's owned Play subscriptions to the signed restore route. */
    fun restore() {
        val current = state.value
        val offer = current.status?.playOffer
        if (offer == null) {
            restoreUnavailable()
            return
        }
        if (current.purchaseBusy) return
        val owner = sessions.load()?.accountId
        if (owner == null || owner != statusOwner) {
            refresh(force = true)
            return
        }
        val attempt = ++purchaseGeneration
        launch = null
        state.value = current.copy(purchasePhase = PremiumPurchasePhase.RESTORING, notice = null)
        viewModelScope.launch {
            val result = try {
                restorePurchases(owner, offer)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                StoreRestoreResult.NotRestored(StoreVerification.Unavailable)
            }
            if (attempt != purchaseGeneration) return@launch
            when (result) {
                is StoreRestoreResult.Restored -> applyOutcome(
                    owner,
                    StoreVerification.Verified(result.status, replayed = false),
                    PremiumNotice.RESTORE_DONE,
                )
                StoreRestoreResult.NothingToRestore -> finishPurchase(PremiumNotice.RESTORE_NONE)
                StoreRestoreResult.OtherAccount -> finishPurchase(PremiumNotice.RESTORE_OTHER_ACCOUNT)
                StoreRestoreResult.Pending -> applyOutcome(owner, StorePurchaseOutcome.Pending)
                StoreRestoreResult.InProgress -> applyOutcome(owner, StorePurchaseOutcome.InProgress)
                StoreRestoreResult.StoreUnavailable -> finishPurchase(PremiumNotice.PURCHASE_STORE_ERROR)
                is StoreRestoreResult.NotRestored -> applyOutcome(owner, result.verification)
            }
        }
    }

    /** Sends unacknowledged Play purchases again, reusing the operation ID of a retried attempt. */
    fun retryVerification() {
        val current = state.value
        if (current.purchaseBusy) return
        val owner = sessions.load()?.accountId
        if (owner == null || owner != statusOwner) {
            refresh(force = true)
            return
        }
        val attempt = ++purchaseGeneration
        state.value = current.copy(purchasePhase = PremiumPurchasePhase.VERIFYING, notice = null)
        viewModelScope.launch {
            val result = try {
                recoverPurchases()
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                StoreRecoveryResult.ServerUnavailable
            }
            if (attempt != purchaseGeneration) return@launch
            when (result) {
                StoreRecoveryResult.SignedOut,
                StoreRecoveryResult.OwnerChanged,
                -> ownerChanged()
                StoreRecoveryResult.StoreUnavailable -> finishPurchase(PremiumNotice.PURCHASE_STORE_ERROR)
                StoreRecoveryResult.ServerUnavailable -> finishPurchase(PremiumNotice.PURCHASE_RETRY)
                StoreRecoveryResult.NothingToVerify,
                StoreRecoveryResult.ChannelOff,
                -> {
                    finishPurchase(null)
                    refresh(force = true)
                }
                is StoreRecoveryResult.Submitted -> applyOutcome(owner, result.outcome)
            }
        }
    }

    private fun onStoreUpdate(update: StorePurchaseUpdate) {
        val context = launch ?: return
        if (context.attempt != purchaseGeneration) return
        if (update is StorePurchaseUpdate.Purchases) {
            state.value = state.value.copy(purchasePhase = PremiumPurchasePhase.VERIFYING)
        }
        viewModelScope.launch {
            val result = try {
                handleUpdate(context.owner, context.offer, update)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                StoreFlowResult.Handled(StoreVerification.Unavailable)
            }
            if (context.attempt != purchaseGeneration) return@launch
            when (result) {
                StoreFlowResult.Cancelled -> finishPurchase(PremiumNotice.PURCHASE_CANCELLED)
                StoreFlowResult.AlreadyOwned -> finishPurchase(PremiumNotice.PURCHASE_ALREADY_OWNED)
                StoreFlowResult.StoreFailed -> finishPurchase(PremiumNotice.PURCHASE_STORE_ERROR)
                is StoreFlowResult.Handled -> applyOutcome(context.owner, result.outcome)
            }
        }
    }

    private fun applyOutcome(
        owner: String,
        outcome: StorePurchaseOutcome,
        verifiedNotice: PremiumNotice = PremiumNotice.PURCHASE_VERIFIED,
    ) {
        if (sessions.load()?.accountId != owner || statusOwner != owner) {
            ownerChanged()
            return
        }
        when (outcome) {
            is StoreVerification.Verified -> {
                // The receipt is the newest server view: fence in-flight status reads.
                generation++
                launch = null
                cache.put(PremiumStatusSnapshot(owner, outcome.status), System.currentTimeMillis())
                state.value = state.value.copy(
                    signedOut = false,
                    loading = false,
                    status = outcome.status,
                    purchasePhase = PremiumPurchasePhase.IDLE,
                    storePending = false,
                    notice = verifiedNotice,
                )
                loadPlansFor(owner, outcome.status)
            }
            StorePurchaseOutcome.Pending -> {
                state.value = state.value.copy(
                    purchasePhase = PremiumPurchasePhase.IDLE,
                    storePending = true,
                    notice = PremiumNotice.PURCHASE_PENDING,
                )
            }
            StoreVerification.OwnerChanged -> ownerChanged()
            StoreVerification.SignInRequired -> finishPurchase(PremiumNotice.DENIED)
            StoreVerification.Denied -> finishPurchase(PremiumNotice.PURCHASE_DENIED)
            StoreVerification.Invalid -> finishPurchase(PremiumNotice.ERROR)
            StorePurchaseOutcome.NotPurchased -> finishPurchase(PremiumNotice.PURCHASE_STORE_ERROR)
            StorePurchaseOutcome.InProgress,
            StoreVerification.Unavailable,
            is StoreVerification.RetryLater,
            -> finishPurchase(PremiumNotice.PURCHASE_RETRY)
        }
    }

    private fun finishPurchase(notice: PremiumNotice?) {
        launch = null
        state.value = state.value.copy(purchasePhase = PremiumPurchasePhase.IDLE, notice = notice)
    }

    /** The signed-in account changed: drop the purchase result and read the new owner's status. */
    private fun ownerChanged() {
        cache.clear()
        forgetOwner()
        refresh(force = true)
        if (!state.value.signedOut) {
            state.value = state.value.copy(notice = PremiumNotice.PURCHASE_OWNER_CHANGED)
        }
    }

    private fun showStatus(owner: String, verified: PremiumStatus) {
        val base = if (statusOwner == owner) {
            state.value.copy(notice = keptNotice())
        } else {
            forgetOwner()
            PremiumUiState()
        }
        statusOwner = owner
        state.value = base.copy(signedOut = false, loading = false, status = verified)
        loadPlansFor(owner, verified)
    }

    private fun keptNotice(): PremiumNotice? = state.value.notice?.takeIf { it in purchaseNotices }

    private fun loadPlansFor(owner: String, verified: PremiumStatus) {
        val offer = verified.playOffer
        if (offer == null || verified.purchaseEligibility() != PurchaseEligibility.AVAILABLE) {
            plansGeneration++
            plansOffer = null
            state.value = state.value.copy(plans = emptyList(), plansLoading = false, plansUnavailable = false)
            return
        }
        if (offer == plansOffer) return
        plansOffer = offer
        val request = ++plansGeneration
        state.value = state.value.copy(plans = emptyList(), plansLoading = true, plansUnavailable = false)
        viewModelScope.launch {
            val result = try {
                loadPlans(offer)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                StoreQueryResult.Unavailable
            }
            if (request != plansGeneration || statusOwner != owner || sessions.load()?.accountId != owner) return@launch
            state.value = when (result) {
                is StoreQueryResult.Ok -> state.value.copy(
                    plans = result.value,
                    plansLoading = false,
                    plansUnavailable = result.value.isEmpty(),
                )
                StoreQueryResult.Unavailable -> {
                    plansOffer = null
                    state.value.copy(plans = emptyList(), plansLoading = false, plansUnavailable = true)
                }
            }
        }
    }

    /** Forget every owner-bound purchase and plan state; late results for it are discarded. */
    private fun forgetOwner() {
        statusOwner = null
        plansOffer = null
        plansGeneration++
        purchaseGeneration++
        launch = null
    }

    private fun noticeFor(error: Exception): PremiumNotice = when (error) {
        is PremiumStatusException.SignedOut -> PremiumNotice.DENIED
        is PremiumStatusException.Denied -> PremiumNotice.DENIED
        is PremiumStatusException.Unavailable -> PremiumNotice.OFFLINE
        is PremiumStatusException.Invalid -> PremiumNotice.ERROR
        is IOException -> PremiumNotice.OFFLINE
        else -> PremiumNotice.ERROR
    }
}
