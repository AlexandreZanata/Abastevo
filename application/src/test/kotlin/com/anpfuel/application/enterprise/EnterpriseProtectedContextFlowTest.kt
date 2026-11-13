package com.anpfuel.application.enterprise

import com.anpfuel.domain.enterprise.EnterprisePartition
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseProtectedContextFlowTest {
    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val org = "7b0e2a54-0000-4000-8000-000000000002"
    private val member = "7b0e2a54-0000-4000-8000-000000000003"
    private val other = "7b0e2a54-0000-4000-8000-000000000004"
    private val origin = "https://api.example.invalid"
    private fun summary(id: String = org) = EnterpriseContextSummary(EnterprisePartition.of(account, origin, id), member, "Synthetic protected company")

    private inner class Fixture {
        val fence = EnterpriseSessionFence()
        val events = mutableListOf<String>()
        var identity = EnterpriseAccessBinding(account, origin, account, 0)
        fun current() = identity.copy(epoch = fence.epoch)
        val local = EnterpriseLocalContext({ identity.accountId }, { identity.origin }, UnavailableEnterprisePersistence, { fence.epoch })
        val gateway = object : EnterpriseContextGateway {
            override fun currentBinding() = current()
            override suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding> { events += "open"; return EnterpriseAccessResult.Ok(current()) }
            override suspend fun list(binding: EnterpriseAccessBinding, afterOrganizationId: String?) = EnterpriseAccessResult.Ok(EnterpriseContextPage(listOf(summary())))
            override suspend fun read(binding: EnterpriseAccessBinding, context: EnterpriseContextSummary): EnterpriseAccessResult<EnterpriseContextSummary> {
                events += "read"; return EnterpriseAccessResult.Ok(context)
            }
            override fun invalidate() = Unit
        }
        fun flow(confirm: suspend (EnterpriseDeviceTarget, EnterpriseAccessBinding, EnterpriseFenceOwner, Boolean) -> EnterpriseFenceHandoff): EnterpriseContextFlow =
            EnterpriseContextFlow(gateway, local, object : EnterpriseContextProtection {
                override suspend fun confirm(target: EnterpriseDeviceTarget, binding: EnterpriseAccessBinding, owner: EnterpriseFenceOwner,
                    acknowledgeErasedWork: Boolean): EnterpriseFenceHandoff {
                    events += "contact"
                    return confirm(target, binding, owner, acknowledgeErasedWork)
                }
            }, fence)
    }

    @Test fun `owned contact transition precedes new cookie and source metadata even while epoch observer runs`() = runTest {
        val f = Fixture()
        lateinit var flow: EnterpriseContextFlow
        flow = f.flow { target, binding, owner, _ ->
            assertEquals(EnterpriseDeviceTarget(summary().partition, member), target)
            assertEquals(0L, binding.epoch)
            f.fence.commitHandoff(owner, binding.epoch).also { flow.invalidateIfStale() }
        }
        flow.select(summary())
        assertEquals(listOf("contact", "open", "read"), f.events)
        assertEquals(1L, f.fence.epoch)
        assertTrue(flow.state.value.selectionConfirmed)
        assertEquals(summary(), flow.state.value.selected)
        assertTrue(f.local.select(summary().partition).isCurrent())
    }

    @Test fun `generic refusal unlock and lost work require fresh explicit feedback without metadata channel`() = runTest {
        for (failure in listOf(EnterpriseProtectionRefused(EnterpriseAccessError.ACCESS_DENIED), EnterpriseUnlockRequired(), EnterpriseVaultUnrecoverable())) {
            val f = Fixture()
            val flow = f.flow { _, _, _, _ -> throw failure }
            flow.select(summary())
            assertEquals(listOf("contact"), f.events)
            assertFalse(flow.state.value.selectionConfirmed)
            assertEquals(failure is EnterpriseUnlockRequired, flow.state.value.unlockRequired)
            assertEquals(failure is EnterpriseVaultUnrecoverable, flow.state.value.erasedWorkAcknowledgementRequired)
        }
        val f = Fixture()
        var acknowledged = false
        val flow = f.flow { _, binding, owner, ack -> acknowledged = ack; f.fence.commitHandoff(owner, binding.epoch) }
        flow.select(summary(), acknowledgeErasedWork = true)
        assertTrue(acknowledged)
        assertTrue(flow.state.value.selectionConfirmed)
    }

    @Test fun `late committed receipt cannot reopen or withdraw a newer owned selection`() = runTest {
        val f = Fixture()
        val entered = CompletableDeferred<Unit>(); val release = CompletableDeferred<Unit>()
        val old = f.flow { _, binding, owner, _ ->
            val receipt = f.fence.commitHandoff(owner, binding.epoch)
            entered.complete(Unit); release.await(); receipt
        }
        val pending = async { old.select(summary()) }
        entered.await()
        old.unconfirm()
        val newest = f.flow { _, binding, owner, _ -> f.fence.commitHandoff(owner, binding.epoch) }
        newest.select(summary(other))
        val handle = f.local.select(summary(other).partition)
        release.complete(Unit); pending.await()
        assertTrue(newest.state.value.selectionConfirmed)
        assertEquals(summary(other), newest.state.value.selected)
        assertTrue(handle.isCurrent())
        assertFalse(old.state.value.selectionConfirmed)
    }

    @Test fun `background or different family before contact commit never opens a new cookie channel`() = runTest {
        for (change in listOf<(Fixture) -> Unit>({ it.fence.markBackground() }, { it.identity = it.identity.copy(familyId = other) })) {
            val f = Fixture()
            val flow = f.flow { _, binding, owner, _ -> change(f); f.fence.commitHandoff(owner, binding.epoch) }
            flow.select(summary())
            assertEquals(listOf("contact"), f.events)
            assertFalse(flow.state.value.selectionConfirmed)
            assertFalse(flow.state.value.busy)
        }
    }

    @Test fun `already consumed receipt cannot be adopted by the flow`() = runTest {
        val f = Fixture()
        val flow = f.flow { _, binding, owner, _ ->
            f.fence.commitHandoff(owner, binding.epoch).also { f.local.consumeContact(owner, it, f.current()) }
        }
        flow.select(summary())
        assertEquals(listOf("contact"), f.events)
        assertFalse(flow.state.value.selectionConfirmed)
        assertFalse(flow.state.value.busy)
    }
}
