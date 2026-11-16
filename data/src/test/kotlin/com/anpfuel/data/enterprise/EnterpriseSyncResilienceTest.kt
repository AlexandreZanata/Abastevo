package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseOperationReused
import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.queue.EnterpriseOperationTransport
import com.anpfuel.application.enterprise.queue.EnterpriseQueueDeliveryFailure
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.domain.enterprise.expense.ExpenseEnvelope
import com.anpfuel.domain.enterprise.expense.ExpenseOperations
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncResult
import java.io.File
import java.net.URLClassLoader
import java.nio.file.Path
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/**
 * EMP-OFFL-013 device half of the sync resilience suite (B-BR-EMP-540..543, BUC-EMP-278, EDR-351..353): the real
 * protected vault, queue, product producers, queue service and signed HTTP client, a real child JVM for process death,
 * and the [ResilienceLedger] at the network edge. The server half is `TestSyncResilience*` on real PostGIS.
 */
class EnterpriseSyncResilienceTest {
    @TempDir lateinit var root: Path
    private val a = partition()
    private val b = partition(org = B)

    private fun device(name: String = "device") = ResilienceDevice(root.resolve("$name-vault"), root.resolve("$name-keys"), ResilienceLedger(root.resolve("ledger")))

    private fun child(stage: String): Process {
        val classpath = linkedSetOf<String>()
        var loader: ClassLoader? = javaClass.classLoader
        while (loader != null) {
            if (loader is URLClassLoader) classpath += loader.urLs.map { Path.of(it.toURI()).toString() }
            loader = loader.parent
        }
        classpath += requireNotNull(System.getProperty("java.class.path")).split(File.pathSeparator)
        return ProcessBuilder(requireNotNull(System.getProperty("java.home")) + "/bin/java", "-cp", classpath.joinToString(File.pathSeparator),
            SyncResilienceProcessProbe::class.java.name, root.resolve("device-vault").toString(), root.resolve("device-keys").toString(),
            root.resolve("ledger").toString(), stage).redirectErrorStream(true).redirectOutput(root.resolve("$stage.log").toFile()).start()
    }

    private fun exit(process: Process): Int = try {
        assertTrue(process.waitFor(120, TimeUnit.SECONDS), "Synthetic device process timed out")
        process.exitValue()
    } finally { if (process.isAlive) process.destroyForcibly() }

    /** Every field-day operation has the ledger's exact stored bytes as its one local receipt, projected once. */
    private suspend fun assertConverged(device: ResilienceDevice, operations: List<EnterpriseOperation>) {
        val attempt = device.attempt()
        assertTrue(device.queue.pending(attempt).isEmpty(), "Work stayed pending after the drain")
        for (operation in operations) {
            val local = device.queue.entry(attempt, operation.operationId)!!.acknowledgement!!
            assertArrayEquals(device.ledger.stored(a.organizationId, operation.operationId), local.canonicalBytes)
            assertEquals(SyncResult.ACCEPTED, local.result, "${operation.type} did not converge on the in-order acceptance")
            assertEquals(1, device.projections[operation.operationId], "${operation.type} receipt was projected more than once")
        }
        assertEquals(operations.size, device.ledger.records(a.organizationId), "One server record per operation")
    }

    // Killed process: the saved field day survives the death of the process that saved it, and a second process that
    // dies after the server committed but before its receipt write is replayed at least once with one effect each.
    @Test fun `killed processes lose no saved operation and a resend after a committed delivery applies each effect once`(): Unit = runBlocking {
        // The departure run is answered before the process that completes and saves the field day starts.
        val run = device().run {
            f.confirm(a); FieldDay.prepareChecklist(this)
            FieldDay.checklist(this).also { close() }
        }
        assertEquals(SyncResilienceProcessProbe.SAVED, exit(child("save")))
        val reopened = device()
        val pending = reopened.queue.pending(reopened.attempt())
        val expected = listOf(run, FieldDay.start(), FieldDay.stop(), FieldDay.fueling(), FieldDay.expense(), FieldDay.finish(), FieldDay.submit())
        assertEquals(expected.map { it.operationId }, pending.map { it.operation.operationId }, "The queue lost or reordered a saved operation")
        expected.zip(pending).forEach { (built, kept) -> assertArrayEquals(built.canonicalBytes, kept.operation.canonicalBytes) }
        assertTrue(pending.all { it.retry.initial && it.acknowledgement == null })
        assertEquals(0, reopened.ledger.records(a.organizationId), "Nothing was sent before delivery")
        reopened.close()
        assertEquals(SyncResilienceProcessProbe.KILLED_BEFORE_ACKNOWLEDGEMENT, exit(child("deliver")))
        val committedBeforeDeath = ResilienceLedger(root.resolve("ledger")).records(a.organizationId)
        assertTrue(committedBeforeDeath > 0, "The killed delivery never reached the server")
        val restarted = device()
        assertEquals(expected.map { it.operationId }, restarted.queue.pending(restarted.attempt()).map { it.operation.operationId },
            "An acknowledgement the dead process never wrote was applied")
        restarted.drain()
        assertEquals(expected.size - committedBeforeDeath, restarted.ledger.commits.get(), "A resend after the committed delivery created a second effect")
        assertConverged(restarted, expected)
        restarted.close()
    }

    // Lost response and missing acknowledgement: neither applies anything; the resend replays the stored bytes.
    @Test fun `a lost answer and an answer missing an acknowledgement apply nothing and the resend replays the stored receipts`(): Unit = runBlocking {
        val device = device(); device.f.confirm(a)
        val operations = FieldDay.save(device)
        val service = device.service()
        device.ledger.fault = ResilienceLedger.Fault.LOSE_ANSWER
        val lost = service.dispatch(device.source.deliveryAccount(), a)
        val committed = device.ledger.records(a.organizationId)
        assertTrue(lost.attempted > 0 && committed == lost.attempted, "The lost answer's operations were not committed by the server")
        assertEquals(0, lost.completed)
        assertTrue(device.queue.pending(device.attempt()).size == operations.size, "A lost answer acknowledged something")
        assertTrue(device.projections.isEmpty())
        device.advance(lost.retryAfterMillis!!)
        device.ledger.fault = ResilienceLedger.Fault.DROP_LAST_ACKNOWLEDGEMENT
        val incomplete = service.dispatch(device.source.deliveryAccount(), a)
        assertEquals(lost.attempted, incomplete.attempted)
        assertEquals(0, incomplete.completed)
        assertEquals(operations.size, device.queue.pending(device.attempt()).size, "An incomplete answer acknowledged a prefix")
        assertEquals(committed, device.ledger.commits.get(), "A resend after a lost answer created a second effect")
        device.drain()
        assertEquals(operations.size, device.ledger.commits.get())
        assertConverged(device, operations)
        // Every operation of the lost batch was delivered at least three times and stored once.
        val sends = device.ledger.requests.flatMap { it.second }.groupingBy { it }.eachCount()
        assertTrue(operations.take(1).all { (sends[it.operationId] ?: 0) >= 3 })
        device.close()
    }

    // Wrong dependency order on the device (the defect, EDR-352): the departure run is in backoff when the start naming
    // it is saved; the start must not reach the server before the run's final receipt, or it is held ORPHAN_REFERENCE.
    @Test fun `the trip start naming its departure run waits for the run's final receipt even while the run is in backoff`(): Unit = runBlocking {
        val device = device(); device.f.confirm(a)
        val run = FieldDay.checklist(device)
        assertTrue(device.service().save(a, run))
        val offline = device.service(EnterpriseOperationTransport { _, _ -> throw EnterpriseQueueDeliveryFailure() })
        offline.dispatch(device.source.deliveryAccount(), a)
        assertTrue(device.queue.entry(device.attempt(), run.operationId)!!.retry.attempts > 0, "The run was not left in backoff")
        val rest = listOf(FieldDay.start(), FieldDay.stop(), FieldDay.fueling(), FieldDay.expense(), FieldDay.finish(), FieldDay.submit())
        rest.forEach { assertTrue(device.service().save(a, it)) }
        device.drain(rounds = 12, strict = false)
        val start = device.queue.entry(device.attempt(), id(FieldDay.START))!!.acknowledgement
        assertEquals(SyncResult.ACCEPTED, start?.result, "The start reached the server before its departure run and was held ${start?.code}")
        val order = device.ledger.requests.flatMap { it.second }
        assertTrue(order.indexOf(run.operationId) < order.indexOf(id(FieldDay.START)), "The start was sent before its departure run")
        assertConverged(device, listOf(run) + rest)
        device.close()
    }

    // Wrong dependency order on the server: a logbook stop and the finish delivered before their start are answered
    // RETRY DEPENDENCY_PENDING with nothing stored and converge after the start.
    @Test fun `dependent operations sent before their start are retried with nothing stored and then converge`(): Unit = runBlocking {
        val device = device(); device.f.confirm(a)
        val run = FieldDay.checklist(device)
        assertTrue(device.service().save(a, run))
        device.drain()
        val start = FieldDay.start()
        assertTrue(device.service().save(a, start))
        device.service(EnterpriseOperationTransport { _, _ -> throw EnterpriseQueueDeliveryFailure() }).dispatch(device.source.deliveryAccount(), a)
        val children = listOf(FieldDay.stop(), FieldDay.finish())
        children.forEach { assertTrue(device.service().save(a, it)) }
        val early = device.service().dispatch(device.source.deliveryAccount(), a)
        assertEquals(children.map { it.operationId }, device.ledger.requests.last().second, "The due children were not sent ahead of the start in backoff")
        assertEquals(0, early.completed)
        children.forEach {
            assertNull(device.ledger.stored(a.organizationId, it.operationId), "A RETRY answer stored a record")
            assertNull(device.queue.entry(device.attempt(), it.operationId)!!.acknowledgement, "A RETRY answer was applied as final")
        }
        device.drain()
        assertConverged(device, listOf(run, start) + children)
        device.close()
    }

    // Duplicate effects on the device: an identical save is idempotent, another intent under the same identifier is
    // refused, and an applied receipt is never projected again nor replaced by other bytes.
    @Test fun `duplicate saves and receipts keep one operation one receipt and one projection`(): Unit = runBlocking {
        val device = device(); device.f.confirm(a)
        val operations = FieldDay.save(device)
        val service = device.service()
        for (operation in operations) assertFalse(service.save(a, operation), "An identical resave was not idempotent")
        val changed = ExpenseOperations.submit(ExpenseEnvelope(SyncBinding.of(a.ownerAccount, a.origin, a.organizationId), B, VaultFixture.C,
            id(FieldDay.EXPENSE), TimeDeclaration(EnterpriseTime.parseInstant("2026-11-15T14:00:00Z"), "America/Cuiaba", -14_400)), id(1880),
            FieldDay.asset, "TOLL", "1300", "1300", "EMPLOYEE", null)
        assertThrows(EnterpriseOperationReused::class.java) { runBlocking { service.save(a, changed) } }
        assertEquals(operations.size, device.queue.pending(device.attempt()).size)
        device.drain()
        assertConverged(device, operations)
        val attempt = device.attempt()
        val expense = operations.single { it.operationId == id(FieldDay.EXPENSE) }
        val receipt = device.queue.entry(attempt, expense.operationId)!!.acknowledgement!!
        assertFalse(device.queue.acknowledge(attempt, expense, receipt) { error("A replayed receipt was projected again") })
        val other = OperationAcknowledgement.parse(String(receipt.canonicalBytes).replace("2026-11-16T12:00:0", "2026-11-16T12:09:0").toByteArray(), expense)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { device.queue.acknowledge(attempt, expense, other) { } } }
        assertArrayEquals(receipt.canonicalBytes, device.queue.entry(attempt, expense.operationId)!!.acknowledgement!!.canonicalBytes)
        assertEquals(1, device.projections[expense.operationId])
        device.close()
    }

    // Stale cross-scope state: organization B of the same account never sends, receives or applies A's state.
    @Test fun `another organization's partition never sends receives or applies this organization's operations`(): Unit = runBlocking {
        val device = device(); device.f.confirm(a); device.f.confirm(b)
        val operations = FieldDay.save(device)
        // B's own toll deliberately reuses A's operation and entity identifiers.
        val foreign = FieldDay.expense(b)
        assertTrue(device.service().save(b, foreign))
        device.drain(a)
        assertTrue(device.ledger.requests.all { it.first == a.organizationId }, "B's partition was sent during A's delivery")
        assertEquals(listOf(foreign.operationId), device.queue.pending(device.attempt(b)).map { it.operation.operationId })
        val receiptA = device.queue.entry(device.attempt(), id(FieldDay.EXPENSE))!!.acknowledgement!!
        val expenseA = operations.single { it.operationId == id(FieldDay.EXPENSE) }
        val attemptB = device.attempt(b)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { device.queue.acknowledge(attemptB, expenseA, receiptA) { } } }
        assertNull(device.queue.entry(attemptB, foreign.operationId)!!.acknowledgement, "A's receipt was applied in B")
        device.drain(b)
        val receiptB = device.queue.entry(device.attempt(b), foreign.operationId)!!.acknowledgement!!
        assertFalse(receiptB.canonicalBytes.contentEquals(receiptA.canonicalBytes))
        assertEquals(listOf(b.organizationId), device.ledger.requests.map { it.first }.distinct().filter { it != a.organizationId })
        assertArrayEquals(receiptA.canonicalBytes, device.ledger.stored(a.organizationId, expenseA.operationId))
        assertArrayEquals(receiptA.canonicalBytes, device.queue.entry(device.attempt(), expenseA.operationId)!!.acknowledgement!!.canonicalBytes)
        device.close()
    }
}
