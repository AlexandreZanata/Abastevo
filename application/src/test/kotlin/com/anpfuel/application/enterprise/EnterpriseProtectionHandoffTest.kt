package com.anpfuel.application.enterprise

import com.anpfuel.domain.enterprise.EnterprisePartition
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseProtectionHandoffTest {
    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val other = "7b0e2a54-0000-4000-8000-000000000002"
    private val origin = "https://api.example.invalid"
    private class Fixture(val account: String, val origin: String) {
        val fence = EnterpriseSessionFence()
        var identity = EnterpriseAccessBinding(account, origin, account, 0)
        fun current() = identity.copy(epoch = fence.epoch)
        val local = EnterpriseLocalContext({ identity.accountId }, { identity.origin }, UnavailableEnterprisePersistence, { fence.epoch })
        val lease = local.acquire(EnterpriseLocalContext.Owner())
        val contact = local.reserveContact(lease, current(), fence, ::current)
    }

    @Test fun `only registered exact epoch transition can be consumed once and select context`() = runTest {
        val f = Fixture(account, origin)
        assertTrue(f.fence.ownerIsCurrent(f.contact, 0))
        val handoff = f.fence.commitHandoff(f.contact, 0)
        assertEquals(1L, f.fence.epoch)
        assertFalse(f.fence.ownerIsCurrent(f.contact, 0))
        assertTrue(f.fence.handoffIsCurrent(f.contact, handoff, f.current()))
        val successor = f.local.consumeContact(f.contact, handoff, f.current())
        assertThrows(EnterpriseLocalDenied::class.java) { f.local.consumeContact(f.contact, handoff, f.current()) }
        assertThrows(EnterpriseLocalDenied::class.java) { f.fence.commitHandoff(f.contact, 0) }
        val store = f.local.select(successor, EnterprisePartition.of(account, origin, other))
        assertTrue(store.isCurrent())
        assertFalse(handoff.toString().contains(account))
    }

    @Test fun `replacement local owner account family origin and background never acquire a handoff`() {
        val changes: List<(Fixture) -> Unit> = listOf(
            { it.local.acquire(EnterpriseLocalContext.Owner()) },
            { it.identity = it.identity.copy(accountId = other) },
            { it.identity = it.identity.copy(familyId = other) },
            { it.identity = it.identity.copy(origin = "https://other.example.invalid") },
            { it.fence.markBackground() },
            { it.fence.lock() },
        )
        for (change in changes) {
            val f = Fixture(account, origin)
            change(f)
            val before = f.fence.epoch
            assertFalse(f.fence.ownerIsCurrent(f.contact, 0))
            assertThrows(EnterpriseLocalDenied::class.java) { f.fence.commitHandoff(f.contact, 0) }
            assertEquals(before, f.fence.epoch)
        }
    }

    @Test fun `late receipt cannot adopt another epoch or withdraw a newer selection`() = runTest {
        for (change in listOf<(Fixture) -> Unit>({ it.fence.markBackground() }, { it.fence.lock() }, { it.local.acquire(EnterpriseLocalContext.Owner()) })) {
            val f = Fixture(account, origin)
            val handoff = f.fence.commitHandoff(f.contact, 0)
            change(f)
            val next = f.local.acquire(EnterpriseLocalContext.Owner())
            val store = f.local.select(next, EnterprisePartition.of(account, origin, other))
            assertFalse(f.fence.handoffIsCurrent(f.contact, handoff, f.current()))
            assertThrows(EnterpriseLocalDenied::class.java) { f.local.consumeContact(f.contact, handoff, f.current()) }
            assertTrue(store.isCurrent())
        }
    }
}
