package com.anpfuel.application.usecase.privacy

import com.anpfuel.application.port.ContributionScopeProvider
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.application.port.TrainingConsentLocalStore
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.repository.*
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class TrainingConsentUseCaseTest {
    private val owner = ContributionScope("account:account-1:key-a", "https://test.invalid")
    private class Journal : TrainingConsentLocalStore {
        val values = mutableMapOf<ContributionScope, TrainingConsentLocalState>()
        var fail = false
        override fun load(scope: ContributionScope) = values[scope] ?: TrainingConsentLocalState()
        override fun update(scope: ContributionScope, change: (TrainingConsentLocalState) -> TrainingConsentLocalState): TrainingConsentLocalState {
            if (fail) throw TrainingConsentException(TrainingConsentRejectKind.TRANSPORT, "storage unavailable")
            return change(load(scope)).also { values[scope] = it }
        }
    }
    private class Gateway : TrainingConsentGateway {
        data class Sent(val scope: ContributionScope, val revision: Long, val id: String)
        val grants = mutableListOf<Sent>()
        val revokes = mutableListOf<Sent>()
        var reads = 0
        var revision = 0L
        var failure: TrainingConsentException? = null
        var grantPause: CompletableDeferred<Unit>? = null
        var conflicts = 0
        override suspend fun consentStatus(scope: ContributionScope): TrainingConsentStatus {
            reads++
            return TrainingConsentStatus(false, null, null, null, revision, false)
        }
        override suspend fun grantConsent(scope: ContributionScope, termsVersion: String, termsDigest: String, revision: Long, commandId: String): TrainingConsentReceipt {
            grants += Sent(scope, revision, commandId)
            grantPause?.await()
            failure?.let { throw it }
            this.revision = revision
            return TrainingConsentReceipt("GRANTED", revision, termsVersion, false)
        }
        override suspend fun revokeConsent(scope: ContributionScope, revision: Long, commandId: String): TrainingConsentReceipt {
            revokes += Sent(scope, revision, commandId)
            if (conflicts-- > 0) { this.revision = 100; throw TrainingConsentException(TrainingConsentRejectKind.STALE_REVISION, "stale") }
            failure?.let { throw it }
            this.revision = revision
            return TrainingConsentReceipt("REVOKED", revision, "v1", false, true)
        }
    }
    private inner class Fixture {
        val local = Journal()
        val gateway = Gateway()
        val connected = MutableStateFlow(true)
        var scope = owner
        var now = 1000L
        var ids = 0
        fun useCase() = TrainingConsentUseCase(local, gateway, object : NetworkConnectivityGateway {
            override fun observeIsConnected() = connected
        }, ContributionScopeProvider { scope }, { now }, { "command-${++ids}" })
        val caller = TrainingConsentCaller("account-1")
    }

    @Test fun `guest performs no io`() = runTest {
        val f = Fixture(); val use = f.useCase()
        assertEquals(TrainingConsentOutcome.Guest, use.status(null))
        assertEquals(TrainingConsentOutcome.Guest, use.grant(null, "v1", "hash"))
        assertEquals(TrainingConsentOutcome.Guest, use.revoke(null))
        assertEquals(0, f.gateway.reads); assertTrue(f.local.values.isEmpty())
    }
    @Test fun `grant records server seeded revision and enables toggle`() = runTest {
        val f=Fixture();f.gateway.revision=100
        assertTrue(f.useCase().grant(f.caller,"v1","hash") is TrainingConsentOutcome.Granted)
        assertEquals(101,f.gateway.grants.single().revision)
        assertTrue(f.local.load(owner).granted)
    }
    @Test fun `revisions increase per decision`() = runTest {
        val f=Fixture();val use=f.useCase()
        use.grant(f.caller,"v1","hash");use.revoke(f.caller)
        assertEquals(listOf(1L),f.gateway.grants.map{it.revision})
        assertEquals(listOf(2L),f.gateway.revokes.map{it.revision})
    }
    @Test fun `grant refuses offline without queueing`() = runTest {
        val f=Fixture();f.connected.value=false
        assertEquals(TrainingConsentOutcome.Offline,f.useCase().grant(f.caller,"v1","hash"))
        assertTrue(f.local.values.isEmpty());assertTrue(f.gateway.grants.isEmpty())
    }
    @Test fun `revoke queues offline as pending`() = runTest {
        val f=Fixture();f.connected.value=false
        assertEquals(TrainingConsentOutcome.RevocationPending,f.useCase().revoke(f.caller))
        assertFalse(f.local.load(owner).granted);assertEquals(false,f.local.load(owner).pending?.grant)
        assertTrue(f.gateway.revokes.isEmpty())
    }
    @Test fun `sync uploads pending and clears`() = runTest {
        val f=Fixture();f.connected.value=false;f.useCase().revoke(f.caller)
        val id=f.local.load(owner).pending!!.id
        f.connected.value=true
        assertTrue(f.useCase().syncPending(f.caller) is TrainingConsentOutcome.Revoked)
        assertEquals(id,f.gateway.revokes.single().id);assertNull(f.local.load(owner).pending)
    }
    @Test fun `rejections carry fixed kinds`() = runTest {
        val f=Fixture();f.gateway.failure=TrainingConsentException(TrainingConsentRejectKind.STALE_REVISION,"stale")
        val result=f.useCase().grant(f.caller,"v1","hash") as TrainingConsentOutcome.Rejected
        assertEquals(TrainingConsentRejectKind.STALE_REVISION,result.kind)
        assertFalse(f.local.load(owner).granted)
    }
    @Test fun `blank terms refuse before io`() = runTest {
        val f=Fixture()
        assertTrue(f.useCase().grant(f.caller,"","hash") is TrainingConsentOutcome.Rejected)
        assertEquals(0,f.gateway.reads);assertTrue(f.gateway.grants.isEmpty())
    }
    @Test fun `online revoke failure keeps durable local off and pending intent`() = runTest {
        val f=Fixture();f.local.values[owner]=TrainingConsentLocalState(1,true)
        f.gateway.failure=TrainingConsentException(TrainingConsentRejectKind.TRANSPORT,"transport")
        assertEquals(TrainingConsentOutcome.RevocationPending,f.useCase().revoke(f.caller))
        assertFalse(f.local.load(owner).granted);assertNotNull(f.local.load(owner).pending)
        val sent=f.gateway.revokes.single()
        f.gateway.failure=null;f.now+=60000
        f.useCase().syncPending(f.caller)
        assertEquals(sent,f.gateway.revokes.last());assertNull(f.local.load(owner).pending)
    }
    @Test fun `another account cannot consume pending revocation`() = runTest {
        val f=Fixture();f.connected.value=false;f.useCase().revoke(f.caller)
        f.connected.value=true;f.scope=ContributionScope("account:account-2:key-b",owner.origin)
        f.useCase().syncPending(TrainingConsentCaller("account-2"))
        assertTrue(f.gateway.revokes.isEmpty());assertNotNull(f.local.load(owner).pending)
        assertTrue(f.useCase().syncPending(f.caller) is TrainingConsentOutcome.Rejected)
    }
    @Test fun `key or origin switch quarantines old pending decision`() = runTest {
        for (other in listOf(ContributionScope("account:account-1:key-b",owner.origin),ContributionScope(owner.ownerScope,"https://other.invalid"))) {
            val f=Fixture();f.connected.value=false;f.useCase().revoke(f.caller)
            f.scope=other;f.connected.value=true;f.useCase().syncPending(f.caller)
            assertTrue(f.gateway.revokes.isEmpty());assertNotNull(f.local.load(owner).pending)
        }
    }
    @Test fun `off dominates grant already in flight`() = runTest {
        val f=Fixture();val use=f.useCase();val pause=CompletableDeferred<Unit>();f.gateway.grantPause=pause
        val on=launch{use.grant(f.caller,"v1","hash")}
        testScheduler.runCurrent()
        assertEquals(1,f.gateway.grants.size)
        val off=launch{use.revoke(f.caller)}
        testScheduler.runCurrent()
        assertEquals(false,f.local.load(owner).pending?.grant)
        pause.complete(Unit);on.join();off.join()
        assertFalse(f.local.load(owner).granted);assertNull(f.local.load(owner).pending)
        assertEquals(2,f.gateway.revokes.single().revision)
    }
    @Test fun `confirmed stale off rebinds once but grant never does`() = runTest {
        val f=Fixture();f.gateway.conflicts=1
        assertTrue(f.useCase().revoke(f.caller) is TrainingConsentOutcome.Revoked)
        assertEquals(listOf(1L,101L),f.gateway.revokes.map{it.revision})
        assertEquals(1,f.gateway.revokes.map{it.id}.distinct().size)
    }
    @Test fun `retry after survives use case restart and prevents early io`() = runTest {
        val f=Fixture();f.gateway.failure=TrainingConsentException(TrainingConsentRejectKind.TRANSPORT,"limited",3600)
        f.useCase().revoke(f.caller);val count=f.gateway.revokes.size
        f.now+=60000;f.useCase().syncPending(f.caller)
        assertEquals(count,f.gateway.revokes.size)
        f.now+=3600000;f.gateway.failure=null;f.useCase().syncPending(f.caller)
        assertEquals(count+1,f.gateway.revokes.size)
    }
    @Test fun `storage failure prevents network mutation`() = runTest {
        val f=Fixture();f.local.fail=true
        assertTrue(f.useCase().revoke(f.caller) is TrainingConsentOutcome.Rejected)
        assertTrue(f.gateway.revokes.isEmpty());assertEquals(0,f.gateway.reads)
    }
    @Test fun `pending grant never resumes automatically`() = runTest {
        val f=Fixture();f.gateway.failure=TrainingConsentException(TrainingConsentRejectKind.TRANSPORT,"lost reply")
        f.useCase().grant(f.caller,"v1","hash")
        f.now+=60000;f.gateway.failure=null;f.useCase().syncPending(f.caller)
        assertEquals(1,f.gateway.grants.size);assertFalse(f.local.load(owner).granted)
    }
}
