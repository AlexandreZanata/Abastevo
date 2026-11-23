package com.anpfuel.app.ui.premium

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
import com.anpfuel.application.usecase.premium.StoreOperationIds
import com.anpfuel.application.usecase.premium.SubmitStorePurchaseUseCase
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.PremiumSource
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusException
import com.anpfuel.domain.premium.PremiumStatusSnapshot
import com.anpfuel.domain.premium.PremiumTier
import com.anpfuel.domain.premium.PurchaseEligibility
import com.anpfuel.domain.premium.StoreOffer
import com.anpfuel.domain.premium.StorePlan
import com.anpfuel.domain.premium.StorePricePhase
import com.anpfuel.domain.premium.StoreProduct
import com.anpfuel.domain.premium.StoreProductDetails
import com.anpfuel.domain.premium.StoreProvider
import com.anpfuel.domain.premium.StorePurchase
import com.anpfuel.domain.premium.StorePurchaseKind
import com.anpfuel.domain.premium.StorePurchaseState
import com.anpfuel.domain.premium.StoreSubscriptionOffer
import com.anpfuel.domain.premium.StoreVerification
import com.anpfuel.domain.repository.StorePurchaseGateway
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class PremiumViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val useCase = mockk<GetPremiumStatusUseCase>()
    private val sessions = mockk<AuthSessionStore>()
    private val cache = PremiumStatusCache()
    private val session = PortableAuth.Session("fam", "acct", "token", "refresh", Long.MAX_VALUE, Long.MAX_VALUE)
    private val active = PremiumStatus(
        policyVersion = 1, state = PremiumState.ACTIVE, reason = "verified",
        capabilities = setOf("cloud.sync"), validUntil = "2026-11-30T23:59:59Z",
        retainedDataAccess = true, paymentsConfigured = false,
        managementUrl = PremiumStatus.MANAGEMENT_URL,
    )

    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun teardown() { Dispatchers.resetMain() }

    private val binding = "a".repeat(64)
    private val offer = StoreOffer(
        StoreProvider.PLAY,
        listOf(StoreProduct("abastevo_premium", listOf("monthly", "yearly"))),
        binding,
        "https://play.google.com/store/account/subscriptions?package=com.anpfuel.app",
    )
    private val buyable = active.copy(
        state = PremiumState.NONE, reason = "no-purchase", capabilities = emptySet(), validUntil = null,
        storeOffers = listOf(offer),
    )
    private val purchased = active.copy(
        storeOffers = listOf(offer),
        tiers = listOf(PremiumTier("PRO", PremiumSource.PLAY, "2026-11-30T23:59:59Z")),
    )
    private val store = FakeStore()
    private val gateway = FakeGateway()
    private val submit = SubmitStorePurchaseUseCase(gateway, sessions, StoreOperationIds())
    private val host = StorePurchaseHost(Any())

    private fun model() = PremiumViewModel(
        useCase, sessions, cache, store,
        LoadStorePlansUseCase(store),
        HandleStorePurchaseUpdateUseCase(submit),
        RestoreStorePurchasesUseCase(store, submit),
        RecoverStorePurchasesUseCase(store, useCase, submit, sessions),
    )

    private fun purchase(state: StorePurchaseState = StorePurchaseState.PURCHASED, acknowledged: Boolean = false) =
        StorePurchase("tok-1", listOf("abastevo_premium"), state, acknowledged, binding)

    private suspend fun kotlinx.coroutines.test.TestScope.buyableModel(): PremiumViewModel {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", buyable)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        return vm
    }

    @Test fun `signed out state makes no private call`() = runTest(dispatcher) {
        every { sessions.load() } returns null
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.signedOut)
        coVerify(exactly = 0) { useCase.invoke() }
    }

    @Test fun `verified active status is rendered`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)
        assertFalse(vm.uiState.value.loading)
    }

    @Test fun `offline and denied status never retain a stale grant`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } throws PremiumStatusException.Unavailable("down")
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertNull(vm.uiState.value.status)
        assertEquals(PremiumNotice.OFFLINE, vm.uiState.value.notice)
        coEvery { useCase.invoke() } throws PremiumStatusException.Denied("no")
        vm.refresh()
        advanceUntilIdle()
        assertEquals(PremiumNotice.DENIED, vm.uiState.value.notice)
    }

    @Test fun `account switch during request discards the old status`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        every { sessions.load() } returns session.copy(accountId = "other")
        advanceUntilIdle()
        assertTrue(vm.uiState.value.signedOut)
        assertNull(vm.uiState.value.status)
    }

    @Test fun `refresh for a new owner clears the previous status before the response`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)

        every { sessions.load() } returns session.copy(accountId = "other")
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot(
            "other",
            active.copy(state = PremiumState.PAUSED),
        )
        vm.refresh()
        assertTrue(vm.uiState.value.loading)
        assertNull(vm.uiState.value.status)

        advanceUntilIdle()
        assertFalse(vm.uiState.value.signedOut)
        assertEquals(PremiumState.PAUSED, vm.uiState.value.status!!.state)
    }

    @Test fun `logout clears the previous verified status on the next refresh`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)

        every { sessions.load() } returns null
        vm.refresh()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.signedOut)
        assertNull(vm.uiState.value.status)
        coVerify(exactly = 1) { useCase.invoke() }
    }

    @Test fun `re-entering within the cache ttl does not call the private endpoint again`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        model().refresh()
        advanceUntilIdle()

        val second = model()
        second.refresh()
        advanceUntilIdle()

        assertEquals(PremiumState.ACTIVE, second.uiState.value.status!!.state)
        assertFalse(second.uiState.value.loading)
        coVerify(exactly = 1) { useCase.invoke() }
    }

    @Test fun `explicit refresh bypasses the cache`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()

        vm.forceRefresh()
        advanceUntilIdle()

        coVerify(exactly = 2) { useCase.invoke() }
    }

    @Test fun `logout clears the cache before another account signs in`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        model().refresh()
        advanceUntilIdle()

        every { sessions.load() } returns null
        model().refresh()
        advanceUntilIdle()

        every { sessions.load() } returns session
        val afterLogout = model()
        afterLogout.refresh()
        advanceUntilIdle()

        coVerify(exactly = 2) { useCase.invoke() }
    }

    @Test fun `restore stays explicitly unavailable without a provider token`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        vm.restoreUnavailable()
        assertEquals(PremiumNotice.RESTORE_UNAVAILABLE, vm.uiState.value.notice)
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)
        vm.clearNotice()
        assertNull(vm.uiState.value.notice)
    }

    @Test fun `store handoff failure is recoverable and keeps the verified status`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", active)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        vm.storeOpenFailed()
        assertEquals(PremiumNotice.STORE_FAILED, vm.uiState.value.notice)
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)
        vm.clearNotice()
        assertNull(vm.uiState.value.notice)
    }

    @Test fun `no play offer shows the status only and never asks the store`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", buyable.copy(storeOffers = emptyList()))
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(PurchaseEligibility.NO_OFFER, vm.uiState.value.eligibility)
        assertTrue(vm.uiState.value.plans.isEmpty())
        assertFalse(vm.uiState.value.canRestore)
        assertEquals(PremiumStatus.MANAGEMENT_URL, vm.uiState.value.managementUrl)
        vm.restore()
        assertEquals(PremiumNotice.RESTORE_UNAVAILABLE, vm.uiState.value.notice)
        assertEquals(0, store.detailQueries)
        assertEquals(0, store.ownedQueries)
    }

    @Test fun `configured offer lists store priced plans in server order`() = runTest(dispatcher) {
        val vm = buyableModel()
        val state = vm.uiState.value
        assertEquals(PurchaseEligibility.AVAILABLE, state.eligibility)
        assertEquals(listOf("monthly", "yearly"), state.plans.map { it.basePlanId })
        assertEquals(listOf("R$ 9,90", "R$ 99,00"), state.plans.map { it.formattedPrice })
        assertFalse(state.plansLoading)
        assertEquals(offer.managementUrl, state.managementUrl)
    }

    @Test fun `store plans unavailable is explicit and retryable`() = runTest(dispatcher) {
        store.details = StoreQueryResult.Unavailable
        val vm = buyableModel()
        assertTrue(vm.uiState.value.plansUnavailable)
        assertTrue(vm.uiState.value.plans.isEmpty())
        store.details = FakeStore.CATALOG
        vm.retryPlans()
        advanceUntilIdle()
        assertEquals(2, vm.uiState.value.plans.size)
    }

    @Test fun `subscribe binds the account and only the server verdict grants premium`() = runTest(dispatcher) {
        val vm = buyableModel()
        vm.subscribe(host, vm.uiState.value.plans.first())
        assertEquals(PremiumPurchasePhase.LAUNCHING, vm.uiState.value.purchasePhase)
        advanceUntilIdle()
        assertEquals(binding, store.launches.single().second)
        assertEquals("tok-m", store.launches.single().first.offerToken)
        assertEquals(PremiumState.NONE, vm.uiState.value.status!!.state)

        gateway.answer = { StoreVerification.Verified(purchased, replayed = false) }
        store.updates.emit(StorePurchaseUpdate.Purchases(listOf(purchase())))
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(PremiumState.ACTIVE, state.status!!.state)
        assertEquals(PremiumNotice.PURCHASE_VERIFIED, state.notice)
        assertEquals(PremiumPurchasePhase.IDLE, state.purchasePhase)
        assertEquals(PurchaseEligibility.MANAGED_IN_STORE, state.eligibility)
        assertTrue(state.plans.isEmpty())
        assertEquals(StorePurchaseKind.VERIFY, gateway.calls.single().kind)
        assertEquals(PremiumState.ACTIVE, cache.get("acct", System.currentTimeMillis())!!.state)
    }

    @Test fun `pending payment grants nothing and blocks a second purchase`() = runTest(dispatcher) {
        val vm = buyableModel()
        vm.subscribe(host, vm.uiState.value.plans.first())
        advanceUntilIdle()
        store.updates.emit(StorePurchaseUpdate.Purchases(listOf(purchase(StorePurchaseState.PENDING))))
        advanceUntilIdle()
        val state = vm.uiState.value
        assertEquals(PremiumNotice.PURCHASE_PENDING, state.notice)
        assertTrue(state.storePending)
        assertEquals(PremiumState.NONE, state.status!!.state)
        assertTrue(gateway.calls.isEmpty())
        vm.subscribe(host, state.plans.first())
        advanceUntilIdle()
        assertEquals(1, store.launches.size)

        gateway.answer = { StoreVerification.Verified(purchased, replayed = false) }
        store.updates.emit(StorePurchaseUpdate.Purchases(listOf(purchase())))
        advanceUntilIdle()
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)
        assertFalse(vm.uiState.value.storePending)
    }

    @Test fun `user cancel and store errors are recoverable without a grant`() = runTest(dispatcher) {
        val vm = buyableModel()
        vm.subscribe(host, vm.uiState.value.plans.first())
        advanceUntilIdle()
        store.updates.emit(StorePurchaseUpdate.Cancelled)
        advanceUntilIdle()
        assertEquals(PremiumNotice.PURCHASE_CANCELLED, vm.uiState.value.notice)
        assertEquals(PremiumPurchasePhase.IDLE, vm.uiState.value.purchasePhase)

        store.launchResult = StoreLaunchResult.UNAVAILABLE
        vm.subscribe(host, vm.uiState.value.plans.first())
        advanceUntilIdle()
        assertEquals(PremiumNotice.PURCHASE_STORE_ERROR, vm.uiState.value.notice)

        store.launchResult = StoreLaunchResult.ALREADY_OWNED
        vm.subscribe(host, vm.uiState.value.plans.first())
        advanceUntilIdle()
        assertEquals(PremiumNotice.PURCHASE_ALREADY_OWNED, vm.uiState.value.notice)
        assertEquals(PremiumState.NONE, vm.uiState.value.status!!.state)
        assertTrue(gateway.calls.isEmpty())
    }

    @Test fun `denied purchase keeps the free status`() = runTest(dispatcher) {
        val vm = buyableModel()
        vm.subscribe(host, vm.uiState.value.plans.first())
        advanceUntilIdle()
        gateway.answer = { StoreVerification.Denied }
        store.updates.emit(StorePurchaseUpdate.Purchases(listOf(purchase())))
        advanceUntilIdle()
        assertEquals(PremiumNotice.PURCHASE_DENIED, vm.uiState.value.notice)
        assertEquals(PremiumState.NONE, vm.uiState.value.status!!.state)
        assertFalse(vm.uiState.value.status!!.canUsePaidCapabilities)
    }

    @Test fun `server outage keeps the purchase unverified and the retry reuses its operation id`() = runTest(dispatcher) {
        val vm = buyableModel()
        vm.subscribe(host, vm.uiState.value.plans.first())
        advanceUntilIdle()
        gateway.answer = { StoreVerification.Unavailable }
        store.updates.emit(StorePurchaseUpdate.Purchases(listOf(purchase())))
        advanceUntilIdle()
        assertEquals(PremiumNotice.PURCHASE_RETRY, vm.uiState.value.notice)
        assertEquals(PremiumState.NONE, vm.uiState.value.status!!.state)

        store.owned = StoreQueryResult.Ok(listOf(purchase()))
        gateway.answer = { StoreVerification.Verified(purchased, replayed = true) }
        vm.retryVerification()
        advanceUntilIdle()
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)
        assertEquals(PremiumNotice.PURCHASE_VERIFIED, vm.uiState.value.notice)
        assertEquals(2, gateway.calls.size)
        assertEquals(gateway.calls[0].operationId, gateway.calls[1].operationId)
    }

    @Test fun `account switch mid purchase discards the result`() = runTest(dispatcher) {
        val vm = buyableModel()
        vm.subscribe(host, vm.uiState.value.plans.first())
        advanceUntilIdle()
        every { sessions.load() } returns session.copy(accountId = "other")
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("other", buyable.copy(storeOffers = emptyList()))
        gateway.answer = { StoreVerification.Verified(purchased, replayed = false) }
        store.updates.emit(StorePurchaseUpdate.Purchases(listOf(purchase())))
        advanceUntilIdle()
        assertTrue(gateway.calls.isEmpty())
        assertEquals(PremiumState.NONE, vm.uiState.value.status!!.state)
        assertEquals(PremiumNotice.PURCHASE_OWNER_CHANGED, vm.uiState.value.notice)
        assertEquals(PremiumState.NONE, cache.get("other", System.currentTimeMillis())!!.state)
        assertNull(cache.get("acct", System.currentTimeMillis()))
    }

    @Test fun `account switch during verification never shows the old owner's grant`() = runTest(dispatcher) {
        val vm = buyableModel()
        vm.subscribe(host, vm.uiState.value.plans.first())
        advanceUntilIdle()
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("other", buyable.copy(storeOffers = emptyList()))
        gateway.answer = {
            every { sessions.load() } returns session.copy(accountId = "other")
            StoreVerification.Verified(purchased, replayed = false)
        }
        store.updates.emit(StorePurchaseUpdate.Purchases(listOf(purchase())))
        advanceUntilIdle()
        assertEquals(1, gateway.calls.size)
        assertEquals(PremiumState.NONE, vm.uiState.value.status!!.state)
        assertFalse(vm.uiState.value.status!!.canUsePaidCapabilities)
    }

    @Test fun `a late status read cannot overwrite a verified receipt`() = runTest(dispatcher) {
        val vm = buyableModel()
        vm.subscribe(host, vm.uiState.value.plans.first())
        advanceUntilIdle()
        val gate = CompletableDeferred<Unit>()
        coEvery { useCase.invoke() } coAnswers {
            gate.await()
            PremiumStatusSnapshot("acct", buyable)
        }
        vm.forceRefresh()
        advanceUntilIdle()
        gateway.answer = { StoreVerification.Verified(purchased, replayed = false) }
        store.updates.emit(StorePurchaseUpdate.Purchases(listOf(purchase())))
        advanceUntilIdle()
        gate.complete(Unit)
        advanceUntilIdle()
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)
    }

    @Test fun `updates not started by this screen are left to start-up recovery`() = runTest(dispatcher) {
        val vm = buyableModel()
        store.updates.emit(StorePurchaseUpdate.Purchases(listOf(purchase())))
        advanceUntilIdle()
        assertTrue(gateway.calls.isEmpty())
        assertEquals(PremiumState.NONE, vm.uiState.value.status!!.state)
    }

    @Test fun `active access from another channel offers no purchase`() = runTest(dispatcher) {
        every { sessions.load() } returns session
        val web = active.copy(
            reason = "verified-web", storeOffers = listOf(offer),
            tiers = listOf(PremiumTier("PRO", PremiumSource.WEB, "2026-12-31T00:00:00Z")),
        )
        coEvery { useCase.invoke() } returns PremiumStatusSnapshot("acct", web)
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(PurchaseEligibility.ACTIVE_FROM_OTHER_SOURCE, vm.uiState.value.eligibility)
        assertTrue(vm.uiState.value.plans.isEmpty())
        vm.subscribe(host, StorePlan("abastevo_premium", "monthly", "tok-m", "R$ 9,90", null))
        advanceUntilIdle()
        assertTrue(store.launches.isEmpty())
        assertEquals(0, store.detailQueries)
    }

    @Test fun `restore verifies this account's subscription through the restore route`() = runTest(dispatcher) {
        val vm = buyableModel()
        store.owned = StoreQueryResult.Ok(listOf(purchase(acknowledged = true)))
        gateway.answer = { StoreVerification.Verified(purchased, replayed = false) }
        vm.restore()
        assertEquals(PremiumPurchasePhase.RESTORING, vm.uiState.value.purchasePhase)
        advanceUntilIdle()
        assertEquals(PremiumNotice.RESTORE_DONE, vm.uiState.value.notice)
        assertEquals(PremiumState.ACTIVE, vm.uiState.value.status!!.state)
        assertEquals(StorePurchaseKind.RESTORE, gateway.calls.single().kind)
    }

    @Test fun `restore reports nothing found and foreign purchases without moving them`() = runTest(dispatcher) {
        val vm = buyableModel()
        vm.restore()
        advanceUntilIdle()
        assertEquals(PremiumNotice.RESTORE_NONE, vm.uiState.value.notice)
        store.owned = StoreQueryResult.Ok(listOf(purchase().copy(accountBinding = "b".repeat(64))))
        vm.restore()
        advanceUntilIdle()
        assertEquals(PremiumNotice.RESTORE_OTHER_ACCOUNT, vm.uiState.value.notice)
        assertTrue(gateway.calls.isEmpty())
        assertEquals(PremiumState.NONE, vm.uiState.value.status!!.state)
    }

    private data class Call(val kind: StorePurchaseKind, val operationId: String)

    private class FakeGateway : StorePurchaseGateway {
        val calls = mutableListOf<Call>()
        var answer: () -> StoreVerification = { StoreVerification.Unavailable }

        override suspend fun submit(
            kind: StorePurchaseKind,
            owner: String,
            operationId: String,
            purchaseToken: String,
            provider: StoreProvider,
        ): StoreVerification {
            calls += Call(kind, operationId)
            return answer()
        }
    }

    private class FakeStore : StoreBillingPort {
        val updates = MutableSharedFlow<StorePurchaseUpdate>(extraBufferCapacity = 8)
        var details: StoreQueryResult<List<StoreProductDetails>> = CATALOG
        var owned: StoreQueryResult<List<StorePurchase>> = StoreQueryResult.Ok(emptyList())
        var launchResult = StoreLaunchResult.LAUNCHED
        val launches = mutableListOf<Pair<StorePlan, String>>()
        var detailQueries = 0
        var ownedQueries = 0
        override val provider: StoreProvider = StoreProvider.PLAY
        override val purchaseUpdates: Flow<StorePurchaseUpdate> = updates

        override suspend fun productDetails(productIds: List<String>): StoreQueryResult<List<StoreProductDetails>> {
            detailQueries++
            return details
        }

        override suspend fun launchPurchase(host: StorePurchaseHost, plan: StorePlan, accountBinding: String): StoreLaunchResult {
            launches += plan to accountBinding
            return launchResult
        }

        override suspend fun ownedSubscriptions(): StoreQueryResult<List<StorePurchase>> {
            ownedQueries++
            return owned
        }

        companion object {
            val CATALOG: StoreQueryResult<List<StoreProductDetails>> = StoreQueryResult.Ok(
                listOf(
                    StoreProductDetails(
                        "abastevo_premium",
                        listOf(
                            StoreSubscriptionOffer("yearly", null, "tok-y", listOf(StorePricePhase("R$ 99,00", "P1Y", true))),
                            StoreSubscriptionOffer("monthly", null, "tok-m", listOf(StorePricePhase("R$ 9,90", "P1M", true))),
                        ),
                    ),
                ),
            )
        }
    }
}
