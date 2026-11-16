package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.expense.EnterpriseExpenseProducer
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.expense.ProtectedExpenseStore
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.expense.ExpenseEnvelope
import com.anpfuel.domain.enterprise.expense.ExpenseOperations
import com.anpfuel.domain.enterprise.expense.ExpensePresentation
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import java.nio.file.Files
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/** EMP-EXP-004 (B-BR-EMP-518, EDR-336): expenses are read back from the exact partition's sealed queue only. */
class ProtectedExpenseStoreTest {
    @TempDir lateinit var root: Path

    private fun expense(p: EnterprisePartition = partition(), op: Int = 800, paid: String = "1250", company: String = "1250"): EnterpriseOperation =
        ExpenseOperations.submit(ExpenseEnvelope(SyncBinding.of(p.ownerAccount, p.origin, p.organizationId), B, C, id(op),
            TimeDeclaration(EnterpriseTime.parseInstant("2026-11-15T12:00:00Z"), "America/Cuiaba", -14_400)), id(op + 10), id(820),
            "TOLL", paid, company, "EMPLOYEE", null)

    @Test fun `expenses are read back exactly from their own partition and nothing readable leaves encryption`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition()); f.confirm(partition(org = B))
        val queue = ProtectedEnterpriseQueue(f.store)
        val store = ProtectedExpenseStore(f.store)
        val producer = EnterpriseExpenseProducer()
        assertTrue(store.read(attempt(f)).isEmpty())
        val at = attempt(f)
        val first = expense()
        assertTrue(queue.save(at, first) { tx -> producer.save(tx, at, first) })
        val second = expense(op = 801, paid = "98765", company = "4321")
        assertTrue(queue.save(at, second) { tx -> producer.save(tx, at, second) })
        assertFalse(queue.save(at, second) { tx -> producer.save(tx, at, second) }, "an identical retry is idempotent")
        val entries = store.read(attempt(f))
        assertEquals(setOf(id(800), id(801)), entries.map { it.operation.operationId }.toSet())
        assertArrayEquals(first.canonicalBytes, entries.first { it.operation.operationId == id(800) }.operation.canonicalBytes)
        assertEquals(listOf(1250L to 1250L, 98765L to 4321L).sortedBy { it.first },
            ExpensePresentation.views(entries).map { it.fact.paidCentavos to it.fact.companyCentavos }.sortedBy { it.first })
        assertTrue(store.read(attempt(f, partition(org = B))).isEmpty(), "another organization of the same account reads nothing")
        // No expense fact, identifier or amount is readable outside encryption.
        val secrets = listOf(id(810), id(820), "TOLL", "expense.submit", "98765", "EMPLOYEE")
        Files.walk(root).use { paths ->
            paths.filter { Files.isRegularFile(it) }.forEach { file ->
                val text = String(Files.readAllBytes(file), Charsets.ISO_8859_1)
                for (secret in secrets) assertFalse(text.contains(secret), "$secret appeared outside encryption")
            }
        }
    }
}
