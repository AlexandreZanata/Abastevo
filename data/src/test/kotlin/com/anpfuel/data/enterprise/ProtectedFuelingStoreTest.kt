package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.fueling.EnterpriseFuelingProducer
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.fueling.ProtectedFuelingStore
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fueling.FuelingEnvelope
import com.anpfuel.domain.enterprise.fueling.FuelingOperations
import com.anpfuel.domain.enterprise.fueling.FuelingPresentation
import com.anpfuel.domain.enterprise.fueling.FuelingReadingInput
import com.anpfuel.domain.enterprise.fueling.FuelingSubmitInput
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import java.nio.file.Files
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/** EMP-FUEL-011 (B-BR-EMP-456, EDR-295): fuelings are read back from the exact partition's sealed queue only. */
class ProtectedFuelingStoreTest {
    @TempDir lateinit var root: Path

    private fun fueling(p: EnterprisePartition = partition(), op: Int = 700, paid: String = "97000"): EnterpriseOperation = FuelingOperations.submit(
        FuelingEnvelope(SyncBinding.of(p.ownerAccount, p.origin, p.organizationId), B, C, id(op),
            TimeDeclaration(EnterpriseTime.parseInstant("2026-11-15T12:00:00Z"), "America/Cuiaba", -14_400)), id(op + 10),
        FuelingSubmitInput(id(720), id(730), "DIESEL_S10", "L", "150000", paid, null, "6490", null, "FULL", "COMPANY", "FUEL_CARD",
            FuelingReadingInput(id(740), id(741), id(op + 50), "ODOMETER", "184320000"), null, "NOT_FOUND", "Posto privado sintético"))

    @Test fun `fuelings are read back exactly from their own partition and nothing readable leaves encryption`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition()); f.confirm(partition(org = B))
        val queue = ProtectedEnterpriseQueue(f.store)
        val store = ProtectedFuelingStore(f.store)
        val producer = EnterpriseFuelingProducer()
        assertTrue(store.read(attempt(f)).isEmpty())
        val at = attempt(f)
        val first = fueling()
        assertTrue(queue.save(at, first) { tx -> producer.save(tx, at, first) })
        val second = fueling(op = 701, paid = "50000")
        assertTrue(queue.save(at, second) { tx -> producer.save(tx, at, second) })
        assertFalse(queue.save(at, second) { tx -> producer.save(tx, at, second) }, "an identical retry is idempotent")
        val entries = store.read(attempt(f))
        assertEquals(setOf(id(700), id(701)), entries.map { it.operation.operationId }.toSet())
        assertArrayEquals(first.canonicalBytes, entries.first { it.operation.operationId == id(700) }.operation.canonicalBytes)
        assertEquals(listOf(97000L, 50000L).sorted(), FuelingPresentation.views(entries).map { it.fact.amounts.paid }.sorted())
        assertTrue(store.read(attempt(f, partition(org = B))).isEmpty(), "another organization of the same account reads nothing")
        // No fueling fact, identifier or place name is readable outside encryption.
        val secrets = listOf(id(710), id(720), "Posto privado sintético", "DIESEL_S10", "fueling.submit", "97000")
        Files.walk(root).use { paths ->
            paths.filter { Files.isRegularFile(it) }.forEach { file ->
                val text = String(Files.readAllBytes(file), Charsets.ISO_8859_1)
                for (secret in secrets) assertFalse(text.contains(secret), "$secret appeared outside encryption")
            }
        }
    }
}
