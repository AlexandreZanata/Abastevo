package com.anpfuel.application.usecase.charging

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.charging.ChargingCondition
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingObservationDraft
import com.anpfuel.domain.charging.ChargingObservationException
import com.anpfuel.domain.charging.ChargingObservationGateway
import com.anpfuel.domain.charging.ChargingPendingStore
import com.anpfuel.domain.charging.ChargingReportReason
import com.anpfuel.domain.charging.ChargingTally
import com.anpfuel.domain.charging.ChargingVote
import com.anpfuel.domain.charging.CommunityChargingObservation
import com.anpfuel.domain.charging.OwnedChargingObservation
import com.anpfuel.domain.charging.PendingChargingKind
import com.anpfuel.domain.charging.PendingChargingOperation
import com.anpfuel.domain.portable.PortableAuth
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ChargingCommunityUseCaseTest {
    private val place = "11111111-1111-4111-8111-111111111111"
    private val draft = ChargingObservationDraft(
        placeId = place, connector = ChargingConnector.TYPE2, priceMilliBrlPerKwh = 1800,
        condition = ChargingCondition.STANDARD, observedDay = "2026-10-22", note = "n",
    )
    private val session = PortableAuth.Session("fam", "acct", "token", "refresh", Long.MAX_VALUE, Long.MAX_VALUE)

    private class MemoryStore : ChargingPendingStore {
        val map = mutableMapOf<String, List<PendingChargingOperation>>()
        override fun load(accountId: String, origin: String) = map["$accountId|$origin"].orEmpty()
        override fun save(accountId: String, origin: String, operations: List<PendingChargingOperation>) {
            map["$accountId|$origin"] = operations
        }
        override fun clear(accountId: String, origin: String) { map.remove("$accountId|$origin") }
    }

    private class FakeGateway : ChargingObservationGateway {
        val submitted = mutableListOf<String>()
        var onSubmit: () -> Unit = {}
        var onHistory: () -> Unit = {}
        var failSubmitOnce = false
        var conflictCorrect = false
        override suspend fun history(placeId: String, limit: Int): List<CommunityChargingObservation> { onHistory(); return emptyList() }
        override suspend fun mine(familyId: String, accessToken: String, limit: Int) = emptyList<OwnedChargingObservation>()
        override suspend fun submit(familyId: String, accessToken: String, id: String, draft: ChargingObservationDraft): String {
            submitted += id
            onSubmit()
            if (failSubmitOnce) {
                failSubmitOnce = false
                throw ChargingObservationException.Unavailable("lost reply")
            }
            return id
        }
        override suspend fun correct(familyId: String, accessToken: String, id: String, expectedRevision: Long, draft: ChargingObservationDraft): Long =
            if (conflictCorrect) throw ChargingObservationException.Conflict("stale") else expectedRevision + 1
        override suspend fun withdraw(familyId: String, accessToken: String, id: String, expectedRevision: Long): Long = expectedRevision + 1
        override suspend fun vote(familyId: String, accessToken: String, observationId: String, choice: ChargingVote, submissionId: String) =
            ChargingTally(1, 0)
        override suspend fun report(familyId: String, accessToken: String, observationId: String, reason: ChargingReportReason, detail: String) = Unit
    }

    private fun useCase(gateway: FakeGateway, store: MemoryStore, sessions: AuthSessionStore) =
        ChargingCommunityUseCase(gateway, store, sessions, "https://api.example.test")

    private fun sessions(session: PortableAuth.Session?): AuthSessionStore = object : AuthSessionStore {
        var current = session
        override fun save(session: PortableAuth.Session) { current = session }
        override fun load() = current
        override fun clear() { current = null }
    }

    @Test fun `lost reply keeps the same operation id and retry converges`() = runTest {
        val gateway = FakeGateway().apply { failSubmitOnce = true }
        val store = MemoryStore()
        val uc = useCase(gateway, store, sessions(session))
        val first = uc.save(draft)
        assertTrue(first is ChargingSaveOutcome.Offline)
        val pending = store.load("acct", "https://api.example.test")
        assertEquals(1, pending.size)
        val id = pending.single().operationId
        val retried = uc.retryPending()
        assertTrue(retried is ChargingSaveOutcome.Saved)
        assertEquals(listOf(id, id), gateway.submitted)
        assertTrue(store.load("acct", "https://api.example.test").isEmpty())
    }

    @Test fun `reopened use case retries the persisted operation`() = runTest {
        val gateway = FakeGateway().apply { failSubmitOnce = true }
        val store = MemoryStore()
        useCase(gateway, store, sessions(session)).save(draft)
        val reopened = useCase(gateway, store, sessions(session))
        assertTrue(reopened.retryPending() is ChargingSaveOutcome.Saved)
        assertTrue(store.load("acct", "https://api.example.test").isEmpty())
    }

    @Test fun `account switch during pending refuses without misdelivering`() = runTest {
        val gateway = FakeGateway().apply { failSubmitOnce = true }
        val store = MemoryStore()
        val auth = sessions(session)
        val uc = useCase(gateway, store, auth)
        uc.save(draft)
        auth.clear()
        val outcome = uc.retryPending()
        assertTrue(outcome is ChargingSaveOutcome.SignedOut)
        assertEquals(1, store.load("acct", "https://api.example.test").size)
        assertEquals(1, gateway.submitted.size)
    }

    @Test fun `stale correction stays pending with explicit conflict`() = runTest {
        val gateway = FakeGateway().apply { conflictCorrect = true }
        val store = MemoryStore()
        val uc = useCase(gateway, store, sessions(session))
        val owned = OwnedChargingObservation(
            id = "22222222-2222-4222-8222-222222222222", placeId = place,
            connector = ChargingConnector.TYPE2, priceMilliBrlPerKwh = 1800,
            condition = ChargingCondition.STANDARD, observedDay = "2026-10-22",
            note = "n", revision = 2,
        )
        val outcome = uc.save(draft, owned)
        assertTrue(outcome is ChargingSaveOutcome.Conflict)
        val pending = store.load("acct", "https://api.example.test")
        assertEquals(PendingChargingKind.CORRECT, pending.single().kind)
        assertEquals("conflict", pending.single().lastError)
    }

    @Test fun `full outbox refuses new command without dropping oldest`() = runTest {
        val gateway = FakeGateway(); val store = MemoryStore()
        val queued = (1..50).map { PendingChargingOperation(java.util.UUID.randomUUID().toString(), PendingChargingKind.SUBMIT, null, null, draft, it.toLong()) }
        store.save("acct", "https://api.example.test", queued)
        val result = useCase(gateway, store, sessions(session)).save(draft)
        assertTrue(result is ChargingSaveOutcome.Refused)
        assertEquals(queued, store.load("acct", "https://api.example.test"))
        assertTrue(gateway.submitted.isEmpty())
    }

    @Test fun `FIFO parks after first outage and keeps remaining commands`() = runTest {
        val gateway = FakeGateway().apply { failSubmitOnce = true }; val store = MemoryStore()
        val queued = (1..3).map { PendingChargingOperation(java.util.UUID.randomUUID().toString(), PendingChargingKind.SUBMIT, null, null, draft, it.toLong()) }
        store.save("acct", "https://api.example.test", queued)
        val uc = useCase(gateway, store, sessions(session))
        assertTrue(uc.retryPending() is ChargingSaveOutcome.Offline)
        assertEquals(listOf(queued.first().operationId), gateway.submitted)
        assertEquals(queued.map { it.operationId }, store.load("acct", "https://api.example.test").map { it.operationId })
        assertTrue(uc.retryPending() is ChargingSaveOutcome.Saved)
        assertEquals(listOf(queued.first().operationId) + queued.map { it.operationId }, gateway.submitted)
    }

    @Test fun `worker for former owner never drains new owners queue`() = runTest {
        val gateway=FakeGateway();val store=MemoryStore()
        val uc=useCase(gateway, store, sessions(session.copy(accountId="new-account")))
        assertTrue(uc.retryPending("acct") is ChargingSaveOutcome.SignedOut)
        assertTrue(gateway.submitted.isEmpty())
    }

    @Test fun `storage failure sends nothing`() = runTest {
        val gateway=FakeGateway()
        val broken=object : ChargingPendingStore {
            override fun load(accountId: String, origin: String)=emptyList<PendingChargingOperation>()
            override fun save(accountId: String, origin: String, operations: List<PendingChargingOperation>) { throw java.io.IOException("disk full") }
            override fun clear(accountId: String, origin: String)=Unit
        }
        try { ChargingCommunityUseCase(gateway,broken,sessions(session),"https://api.example.test").save(draft); throw AssertionError("storage failure hidden") }
        catch (_:java.io.IOException) { }
        assertTrue(gateway.submitted.isEmpty())
    }

    @Test fun `startup repairs commit to scheduler crash window without sending inline`() = runTest {
        val gateway=FakeGateway();val store=MemoryStore();val auth=sessions(session)
        val operation=PendingChargingOperation(java.util.UUID.randomUUID().toString(),PendingChargingKind.SUBMIT,null,null,draft,1)
        store.save("acct","https://api.example.test",listOf(operation))
        val scheduled=mutableListOf<String>()
        ChargingCommunityUseCase(gateway,store,auth,"https://api.example.test",scheduled::add).recoverPendingSchedule()
        assertEquals(listOf("acct"),scheduled);assertTrue(gateway.submitted.isEmpty())
        auth.clear()
        ChargingCommunityUseCase(gateway,store,auth,"https://api.example.test",scheduled::add).recoverPendingSchedule()
        assertEquals(1,scheduled.size)
    }

    @Test fun `account change during response retains former owners command and hides receipts`() = runTest {
        val gateway=FakeGateway();val store=MemoryStore();val auth=sessions(session)
        gateway.onSubmit={auth.save(session.copy(accountId="new-owner"))}
        val uc=useCase(gateway,store,auth)
        assertTrue(uc.save(draft) is ChargingSaveOutcome.SignedOut)
        assertEquals(1,store.load("acct","https://api.example.test").size)
        assertTrue(uc.pendingForCurrentOwner().isEmpty())
        auth.save(session)
        gateway.onHistory={auth.save(session.copy(accountId="new-owner"))}
        val snapshot=uc.snapshot(place)
        assertTrue(snapshot.pending.isEmpty());assertTrue(snapshot.mine.isEmpty())
    }
}
