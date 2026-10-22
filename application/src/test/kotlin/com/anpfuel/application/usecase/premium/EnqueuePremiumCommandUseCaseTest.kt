package com.anpfuel.application.usecase.premium

import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.repository.PremiumCommandStore
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.Test

/**
 * PRM09-T01 durable enqueue: stores before upload, duplicate saves converge
 * and divergent reuse refuses; process-death replay finds the same intent.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class EnqueuePremiumCommandUseCaseTest {

    private class FakeStore : PremiumCommandStore {
        val rows = mutableMapOf<String, PremiumCommand>()

        override suspend fun enqueue(command: PremiumCommand) {
            rows[command.scope.cursorKey(command.dataset) + "|" + command.operationId] = command
        }

        override suspend fun findPending(
            scope: PremiumOwnerScope,
            operationId: String,
        ): PremiumCommand? =
            rows.values.firstOrNull { it.scope == scope && it.operationId == operationId }

        override suspend fun listPending(scope: PremiumOwnerScope, limit: Int): List<PremiumCommand> =
            rows.values.filter { it.scope == scope }.take(limit)

        override suspend fun remove(scope: PremiumOwnerScope, operationId: String) {
            rows.entries.removeIf { it.value.scope == scope && it.value.operationId == operationId }
        }

        override suspend fun clearOwner(scope: PremiumOwnerScope) {
            rows.entries.removeIf { it.value.scope == scope }
        }

        override suspend fun releaseExpired(nowMillis: Long): Int = 0
    }

    @Test
    fun `duplicate save converges and divergent reuse refuses`() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        val store = FakeStore()
        val useCase = EnqueuePremiumCommandUseCase(store, dispatcher)
        val scope = PremiumOwnerScope("acc-1", "origin-1")
        val command = PremiumCommand(
            operationId = "op-1",
            scope = scope,
            dataset = "vehicles",
            recordId = "rec-1",
            expectedRevision = 0,
            payloadJson = "{}",
        )

        assertEquals(command, useCase.invoke(command))
        // Replay after process death: same id + payload converges.
        assertEquals(command, useCase.invoke(command))

        val divergent = command.copy(recordId = "rec-2")
        try {
            useCase.invoke(divergent)
            fail("divergent operation reuse must refuse")
        } catch (e: IllegalArgumentException) {
            assertTrue(e.message!!.contains("divergent"))
        }
    }

    @Test
    fun `owner switch cannot see another owners queue`() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        val store = FakeStore()
        val useCase = EnqueuePremiumCommandUseCase(store, dispatcher)
        val a = PremiumOwnerScope("acc-a", "origin-a")
        val b = PremiumOwnerScope("acc-a", "origin-b")
        useCase.invoke(
            PremiumCommand(
                operationId = "op-1",
                scope = a,
                dataset = "vehicles",
                recordId = "rec-1",
                expectedRevision = 0,
            ),
        )
        assertEquals(0, store.listPending(b, 10).size)
        assertEquals(1, store.listPending(a, 10).size)
    }
}
