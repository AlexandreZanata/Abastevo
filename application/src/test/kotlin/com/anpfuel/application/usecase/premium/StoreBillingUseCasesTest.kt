package com.anpfuel.application.usecase.premium

import com.anpfuel.application.port.StoreBillingPort
import com.anpfuel.application.port.StoreLaunchResult
import com.anpfuel.application.port.StorePurchaseHost
import com.anpfuel.application.port.StorePurchaseUpdate
import com.anpfuel.application.port.StoreQueryResult
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusException
import com.anpfuel.domain.premium.StoreOffer
import com.anpfuel.domain.premium.StorePlan
import com.anpfuel.domain.premium.StorePricePhase
import com.anpfuel.domain.premium.StoreProduct
import com.anpfuel.domain.premium.StoreProductDetails
import com.anpfuel.domain.premium.StoreProvider
import com.anpfuel.domain.premium.StorePurchase
import com.anpfuel.domain.premium.StorePurchaseKind
import com.anpfuel.domain.premium.StorePurchaseOutcome
import com.anpfuel.domain.premium.StorePurchaseState
import com.anpfuel.domain.premium.StoreSubscriptionOffer
import com.anpfuel.domain.premium.StoreVerification
import com.anpfuel.domain.repository.PremiumStatusGateway
import com.anpfuel.domain.repository.StorePurchaseGateway
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class StoreBillingUseCasesTest {
    private val binding = "a".repeat(64)
    private val offer = StoreOffer(
        StoreProvider.PLAY,
        listOf(StoreProduct("abastevo_premium", listOf("monthly", "yearly"))),
        binding,
        "https://play.google.com/store/account/subscriptions?package=com.anpfuel.app",
    )
    private val status = PremiumStatus(
        policyVersion = 1, state = PremiumState.NONE, reason = "no-purchase", capabilities = emptySet(),
        validUntil = null, retainedDataAccess = true, paymentsConfigured = true,
        managementUrl = PremiumStatus.MANAGEMENT_URL, storeOffers = listOf(offer),
    )
    private val active = status.copy(state = PremiumState.ACTIVE, reason = "verified", capabilities = setOf("cloud.sync"))
    private val session = PortableAuth.Session("fam", "acct", "token", "refresh", Long.MAX_VALUE, Long.MAX_VALUE)
    private val sessions = FakeSessions(session)
    private val gateway = FakeGateway()
    private val store = FakeStore()
    private var nextId = 0
    private val ids = StoreOperationIds { "00000000-0000-4000-8000-%012d".format(++nextId) }
    private val submit = SubmitStorePurchaseUseCase(gateway, sessions, ids)

    private fun purchase(
        token: String = "tok-1",
        state: StorePurchaseState = StorePurchaseState.PURCHASED,
        acknowledged: Boolean = false,
        boundTo: String? = binding,
        product: String = "abastevo_premium",
    ) = StorePurchase(token, listOf(product), state, acknowledged, boundTo)

    @Test fun `pending and incomplete purchases grant nothing and send nothing`() = runTest {
        assertEquals(StorePurchaseOutcome.Pending, submit("acct", purchase(state = StorePurchaseState.PENDING), StorePurchaseKind.VERIFY))
        assertEquals(
            StorePurchaseOutcome.NotPurchased,
            submit("acct", purchase(state = StorePurchaseState.UNSPECIFIED), StorePurchaseKind.VERIFY),
        )
        assertEquals(StorePurchaseOutcome.NotPurchased, submit("acct", purchase(token = "bad token"), StorePurchaseKind.VERIFY))
        assertTrue(gateway.calls.isEmpty())
    }

    @Test fun `verified purchase returns the server status and ends the attempt`() = runTest {
        gateway.answer = { StoreVerification.Verified(active, replayed = false) }
        val first = submit("acct", purchase(), StorePurchaseKind.VERIFY)
        assertEquals(StoreVerification.Verified(active, false), first)
        val call = gateway.calls.single()
        assertEquals(StorePurchaseKind.VERIFY, call.kind)
        assertEquals("acct", call.owner)
        assertEquals("tok-1", call.token)
        assertEquals(StoreProvider.PLAY, call.provider)
        submit("acct", purchase(), StorePurchaseKind.VERIFY)
        assertNotEquals(gateway.calls[0].operationId, gateway.calls[1].operationId)
    }

    @Test fun `server outage keeps the purchase unverified and retries with the same operation id`() = runTest {
        gateway.answer = { StoreVerification.Unavailable }
        assertEquals(StoreVerification.Unavailable, submit("acct", purchase(), StorePurchaseKind.VERIFY))
        gateway.answer = { StoreVerification.RetryLater(30) }
        assertEquals(StoreVerification.RetryLater(30), submit("acct", purchase(), StorePurchaseKind.VERIFY))
        gateway.answer = { StoreVerification.Verified(active, replayed = true) }
        assertEquals(StoreVerification.Verified(active, true), submit("acct", purchase(), StorePurchaseKind.VERIFY))
        assertEquals(1, gateway.calls.map { it.operationId }.toSet().size)
        assertEquals(3, gateway.calls.size)
    }

    @Test fun `verify and restore of the same token are separate attempts`() = runTest {
        gateway.answer = { StoreVerification.Unavailable }
        submit("acct", purchase(), StorePurchaseKind.VERIFY)
        submit("acct", purchase(), StorePurchaseKind.RESTORE)
        assertEquals(2, gateway.calls.map { it.operationId }.toSet().size)
    }

    @Test fun `denied purchase is final and a later attempt gets a fresh operation id`() = runTest {
        gateway.answer = { StoreVerification.Denied }
        assertEquals(StoreVerification.Denied, submit("acct", purchase(), StorePurchaseKind.VERIFY))
        submit("acct", purchase(), StorePurchaseKind.VERIFY)
        assertEquals(2, gateway.calls.map { it.operationId }.toSet().size)
    }

    @Test fun `account switch before or during the request discards the result`() = runTest {
        sessions.session = session.copy(accountId = "other")
        assertEquals(StoreVerification.OwnerChanged, submit("acct", purchase(), StorePurchaseKind.VERIFY))
        assertTrue(gateway.calls.isEmpty())

        sessions.session = session
        gateway.answer = {
            sessions.session = session.copy(accountId = "other")
            StoreVerification.Verified(active, replayed = false)
        }
        assertEquals(StoreVerification.OwnerChanged, submit("acct", purchase(), StorePurchaseKind.VERIFY))

        sessions.session = null
        assertEquals(StoreVerification.OwnerChanged, submit("acct", purchase(), StorePurchaseKind.VERIFY))
    }

    @Test fun `the same purchase is never verified twice at once`() = runTest {
        val gate = CompletableDeferred<Unit>()
        gateway.gate = gate
        gateway.answer = { StoreVerification.Verified(active, replayed = false) }
        val first = async { submit("acct", purchase(), StorePurchaseKind.VERIFY) }
        runCurrent()
        assertEquals(StorePurchaseOutcome.InProgress, submit("acct", purchase(), StorePurchaseKind.VERIFY))
        gate.complete(Unit)
        assertEquals(StoreVerification.Verified(active, false), first.await())
        assertEquals(1, gateway.calls.size)
    }

    @Test fun `unexpected gateway failure counts as unavailable and keeps the attempt`() = runTest {
        gateway.answer = { throw IllegalStateException("boom") }
        assertEquals(StoreVerification.Unavailable, submit("acct", purchase(), StorePurchaseKind.VERIFY))
        gateway.answer = { StoreVerification.Unavailable }
        submit("acct", purchase(), StorePurchaseKind.VERIFY)
        assertEquals(1, gateway.calls.map { it.operationId }.toSet().size)
    }

    @Test fun `plans come from the store but only for server listed base plans`() = runTest {
        store.details = StoreQueryResult.Ok(
            listOf(
                StoreProductDetails(
                    "abastevo_premium",
                    listOf(
                        StoreSubscriptionOffer("yearly", null, "tok-y", listOf(StorePricePhase("R$ 99,00", "P1Y", true))),
                        StoreSubscriptionOffer("weekly", null, "tok-w", listOf(StorePricePhase("R$ 3,00", "P1W", true))),
                    ),
                ),
            ),
        )
        val plans = LoadStorePlansUseCase(store).invoke(offer) as StoreQueryResult.Ok
        assertEquals(listOf("yearly"), plans.value.map { it.basePlanId })
        assertEquals(listOf(listOf("abastevo_premium")), store.queried)

        store.details = StoreQueryResult.Unavailable
        assertEquals(StoreQueryResult.Unavailable, LoadStorePlansUseCase(store).invoke(offer))
        val appStore = StoreOffer(
            StoreProvider.APP_STORE, listOf(StoreProduct("com.anpfuel.app.premium", emptyList())),
            "1a2b3c4d-1111-4111-8111-abcdefabcdef", StoreOffer.APP_STORE_MANAGEMENT,
        )
        assertEquals(StoreQueryResult.Unavailable, LoadStorePlansUseCase(store).invoke(appStore))
        assertEquals(2, store.queried.size)
    }

    @Test fun `restore sends each owned subscription of this account and reports the verified status`() = runTest {
        store.owned = StoreQueryResult.Ok(
            listOf(
                purchase(token = "mine", acknowledged = true),
                purchase(token = "foreign", boundTo = "b".repeat(64)),
                purchase(token = "retired", product = "retired_product"),
            ),
        )
        gateway.answer = { StoreVerification.Verified(active, replayed = false) }
        val result = RestoreStorePurchasesUseCase(store, submit).invoke("acct", offer)
        assertEquals(StoreRestoreResult.Restored(active), result)
        assertEquals(listOf("mine"), gateway.calls.map { it.token })
        assertEquals(StorePurchaseKind.RESTORE, gateway.calls.single().kind)
    }

    @Test fun `restore never moves another account's purchase and reports why nothing changed`() = runTest {
        val restore = RestoreStorePurchasesUseCase(store, submit)
        store.owned = StoreQueryResult.Unavailable
        assertEquals(StoreRestoreResult.StoreUnavailable, restore("acct", offer))
        store.owned = StoreQueryResult.Ok(emptyList())
        assertEquals(StoreRestoreResult.NothingToRestore, restore("acct", offer))
        store.owned = StoreQueryResult.Ok(listOf(purchase(product = "retired_product")))
        assertEquals(StoreRestoreResult.NothingToRestore, restore("acct", offer))
        store.owned = StoreQueryResult.Ok(listOf(purchase(boundTo = "b".repeat(64))))
        assertEquals(StoreRestoreResult.OtherAccount, restore("acct", offer))
        store.owned = StoreQueryResult.Ok(listOf(purchase(state = StorePurchaseState.PENDING)))
        assertEquals(StoreRestoreResult.Pending, restore("acct", offer))
        assertTrue(gateway.calls.isEmpty())

        store.owned = StoreQueryResult.Ok(listOf(purchase()))
        gateway.answer = { StoreVerification.Denied }
        assertEquals(StoreRestoreResult.NotRestored(StoreVerification.Denied), restore("acct", offer))
        gateway.answer = { StoreVerification.Unavailable }
        assertEquals(StoreRestoreResult.NotRestored(StoreVerification.Unavailable), restore("acct", offer))
    }

    @Test fun `purchase update verifies only this account's catalog purchases`() = runTest {
        val handle = HandleStorePurchaseUpdateUseCase(submit)
        assertEquals(StoreFlowResult.Cancelled, handle("acct", offer, StorePurchaseUpdate.Cancelled))
        assertEquals(StoreFlowResult.AlreadyOwned, handle("acct", offer, StorePurchaseUpdate.AlreadyOwned))
        assertEquals(StoreFlowResult.StoreFailed, handle("acct", offer, StorePurchaseUpdate.Failed))
        assertEquals(
            StoreFlowResult.Handled(StorePurchaseOutcome.Pending),
            handle("acct", offer, StorePurchaseUpdate.Purchases(listOf(purchase(state = StorePurchaseState.PENDING)))),
        )
        assertEquals(
            StoreFlowResult.Handled(StorePurchaseOutcome.NotPurchased),
            handle("acct", offer, StorePurchaseUpdate.Purchases(listOf(purchase(boundTo = "b".repeat(64))))),
        )
        assertTrue(gateway.calls.isEmpty())

        gateway.answer = { StoreVerification.Verified(active, replayed = false) }
        assertEquals(
            StoreFlowResult.Handled(StoreVerification.Verified(active, false)),
            handle("acct", offer, StorePurchaseUpdate.Purchases(listOf(purchase(), purchase(token = "x", product = "other")))),
        )
        assertEquals(listOf("tok-1"), gateway.calls.map { it.token })
        assertEquals(StorePurchaseKind.VERIFY, gateway.calls.single().kind)
    }

    @Test fun `recovery resends unacknowledged purchases after an outage`() = runTest {
        val statusGateway = FakeStatusGateway(status)
        val recover = RecoverStorePurchasesUseCase(store, GetPremiumStatusUseCase(statusGateway, sessions), submit, sessions)
        store.owned = StoreQueryResult.Ok(
            listOf(
                purchase(token = "done", acknowledged = true),
                purchase(token = "waiting", state = StorePurchaseState.PENDING),
                purchase(token = "foreign", boundTo = "b".repeat(64)),
                purchase(token = "lost"),
            ),
        )
        gateway.answer = { StoreVerification.Unavailable }
        val first = recover()
        assertEquals(StoreRecoveryResult.Submitted(StoreVerification.Unavailable), first)
        gateway.answer = { StoreVerification.Verified(active, replayed = false) }
        assertEquals(StoreRecoveryResult.Submitted(StoreVerification.Verified(active, false)), recover())
        assertEquals(listOf("lost", "lost"), gateway.calls.map { it.token })
        assertEquals(1, gateway.calls.map { it.operationId }.toSet().size)
        assertEquals(StorePurchaseKind.VERIFY, gateway.calls.first().kind)
    }

    @Test fun `recovery stays local when nothing waits and fails closed without a play offer`() = runTest {
        val statusGateway = FakeStatusGateway(status)
        val recover = RecoverStorePurchasesUseCase(store, GetPremiumStatusUseCase(statusGateway, sessions), submit, sessions)

        sessions.session = null
        assertEquals(StoreRecoveryResult.SignedOut, recover())
        assertEquals(0, store.ownedQueries)

        sessions.session = session
        store.owned = StoreQueryResult.Unavailable
        assertEquals(StoreRecoveryResult.StoreUnavailable, recover())
        store.owned = StoreQueryResult.Ok(listOf(purchase(acknowledged = true)))
        assertEquals(StoreRecoveryResult.NothingToVerify, recover())
        assertEquals(0, statusGateway.calls)

        store.owned = StoreQueryResult.Ok(listOf(purchase()))
        statusGateway.status = status.copy(storeOffers = emptyList())
        assertEquals(StoreRecoveryResult.ChannelOff, recover())
        statusGateway.failure = PremiumStatusException.Unavailable("down")
        assertEquals(StoreRecoveryResult.ServerUnavailable, recover())
        statusGateway.failure = null
        statusGateway.status = status
        store.owned = StoreQueryResult.Ok(listOf(purchase(boundTo = "b".repeat(64))))
        assertEquals(StoreRecoveryResult.NothingToVerify, recover())
        assertTrue(gateway.calls.isEmpty())
    }

    @Test fun `operation ids must be lowercase uuids`() {
        val bad = StoreOperationIds { "NOT-A-UUID" }
        val thrown = runCatching { bad.acquire(StoreOperationIds.Key("acct", StorePurchaseKind.VERIFY, "t")) }
        assertTrue(thrown.isFailure)
        val real = StoreOperationIds()
        val key = StoreOperationIds.Key("acct", StorePurchaseKind.VERIFY, "secret")
        val id = real.acquire(key)!!
        assertTrue(com.anpfuel.domain.premium.StoreOperationId.isValid(id))
        assertTrue(!key.toString().contains("secret"))
    }

    private class FakeSessions(var session: PortableAuth.Session?) : AuthSessionStore {
        override fun save(session: PortableAuth.Session) { this.session = session }
        override fun load(): PortableAuth.Session? = session
        override fun clear() { session = null }
    }

    private data class Call(
        val kind: StorePurchaseKind,
        val owner: String,
        val operationId: String,
        val token: String,
        val provider: StoreProvider,
    )

    private class FakeGateway : StorePurchaseGateway {
        val calls = mutableListOf<Call>()
        var answer: () -> StoreVerification = { StoreVerification.Unavailable }
        var gate: CompletableDeferred<Unit>? = null

        override suspend fun submit(
            kind: StorePurchaseKind,
            owner: String,
            operationId: String,
            purchaseToken: String,
            provider: StoreProvider,
        ): StoreVerification {
            calls += Call(kind, owner, operationId, purchaseToken, provider)
            gate?.await()
            return answer()
        }
    }

    private class FakeStore : StoreBillingPort {
        var details: StoreQueryResult<List<StoreProductDetails>> = StoreQueryResult.Unavailable
        var owned: StoreQueryResult<List<StorePurchase>> = StoreQueryResult.Ok(emptyList())
        val queried = mutableListOf<List<String>>()
        var ownedQueries = 0
        override val provider: StoreProvider = StoreProvider.PLAY
        override val purchaseUpdates: Flow<StorePurchaseUpdate> = emptyFlow()

        override suspend fun productDetails(productIds: List<String>): StoreQueryResult<List<StoreProductDetails>> {
            queried += productIds
            return details
        }

        override suspend fun launchPurchase(host: StorePurchaseHost, plan: StorePlan, accountBinding: String) =
            StoreLaunchResult.LAUNCHED

        override suspend fun ownedSubscriptions(): StoreQueryResult<List<StorePurchase>> {
            ownedQueries++
            return owned
        }
    }

    private class FakeStatusGateway(var status: PremiumStatus) : PremiumStatusGateway {
        var calls = 0
        var failure: Exception? = null
        override suspend fun status(familyId: String, accessToken: String): PremiumStatus {
            calls++
            failure?.let { throw it }
            return status
        }
    }
}
