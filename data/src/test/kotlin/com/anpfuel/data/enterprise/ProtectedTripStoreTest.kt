package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.trip.EnterpriseTripProducers
import com.anpfuel.application.enterprise.trip.EnterpriseTripStartHints
import com.anpfuel.data.enterprise.ProtectedQueueFixture.ack
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.ProtectedQueueFixture.operation
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.trip.ProtectedTripStore
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import com.anpfuel.domain.enterprise.trip.TripBook
import com.anpfuel.domain.enterprise.trip.TripClockReading
import com.anpfuel.domain.enterprise.trip.TripEnvelope
import com.anpfuel.domain.enterprise.trip.TripLocalState
import com.anpfuel.domain.enterprise.trip.TripOperations
import com.anpfuel.domain.enterprise.trip.TripReadingInput
import java.nio.file.Files
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/** Synthetic trip operations bound like the vault fixture's grant: membership B, enrollment C. */
internal object TripStoreFixture {
    val hints = EnterpriseTripStartHints()
    private val producers = EnterpriseTripProducers.all(hints).associateBy { it.descriptor().type }
    private fun envelope(p: EnterprisePartition, op: Int, at: String) = TripEnvelope(SyncBinding.of(p.ownerAccount, p.origin, p.organizationId), B, C,
        id(op), TimeDeclaration(EnterpriseTime.parseInstant(at), "America/Cuiaba", -14_400))
    fun start(p: EnterprisePartition = partition(), op: Int = 600, trip: Int = 610, asset: Int = 620): EnterpriseOperation =
        TripOperations.start(envelope(p, op, "2026-11-15T12:00:00Z"), id(trip), id(asset), id(630),
            TripReadingInput(id(640), id(641), id(op + 50), "ODOMETER", "184320000"), null, TripClockReading("boot-2", 1_000))
    fun stop(p: EnterprisePartition = partition(), op: Int = 601, trip: Int = 610, start: Int = 600): EnterpriseOperation =
        TripOperations.event(envelope(p, op, "2026-11-15T13:00:00Z"), id(op + 100), id(trip), "STOP", "Pátio privado", id(start))
    /** The queue save the service performs: the operation and its producer's projection in one protected write. */
    suspend fun save(queue: ProtectedEnterpriseQueue, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation): Boolean {
        val producer = producers.getValue(operation.type)
        return queue.save(attempt, operation) { tx -> producer.save(tx, attempt, operation) }
    }
}

class ProtectedTripStoreTest {
    @TempDir lateinit var root: Path

    @Test fun `the trip book and its operations commit together in the exact partition and nothing else reads them`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition()); f.confirm(partition(org = B))
        val queue = ProtectedEnterpriseQueue(f.store); val store = ProtectedTripStore(f.store)
        assertNull(store.read(attempt(f)).book)
        TripStoreFixture.hints.put(id(600), TripAssetLabel("Caminhão privado", "F-9", "PRV1A23"), true)
        assertTrue(TripStoreFixture.save(queue, attempt(f), TripStoreFixture.start()))
        TripStoreFixture.hints.remove(id(600))
        assertTrue(TripStoreFixture.save(queue, attempt(f), TripStoreFixture.stop()))
        assertFalse(TripStoreFixture.save(queue, attempt(f), TripStoreFixture.stop()), "an identical retry is idempotent")
        val snapshot = store.read(attempt(f))
        val trip = snapshot.book!!.trips.single()
        assertEquals(listOf(id(600), id(601)), trip.operationIds)
        assertEquals(TripAssetLabel("Caminhão privado", "F-9", "PRV1A23"), trip.label); assertTrue(trip.reviewWarningAccepted)
        assertEquals(setOf(id(600), id(601)), snapshot.entries.keys)
        assertArrayEquals(TripStoreFixture.stop().canonicalBytes, snapshot.entries.getValue(id(601)).operation.canonicalBytes)
        assertNull(store.read(attempt(f, partition(org = B))).book, "another organization of the same account reads nothing")
        // A final REJECTED start receipt is projected into the book in the receipt's own write.
        val producer = EnterpriseTripProducers.all(TripStoreFixture.hints).first { it.descriptor().type == TripOperations.START }
        val refusal = com.anpfuel.domain.enterprise.sync.OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${id(600)}",""" +
            """"payload_digest":"${TripStoreFixture.start().payloadDigest}","result":"REJECTED","code":"MALFORMED","entity_revision":null,""" +
            """"received_at":"2026-11-15T14:00:00Z","review_item_id":null,"warnings":[]}""").toByteArray(), TripStoreFixture.start())
        val captured = attempt(f)
        assertTrue(queue.acknowledge(captured, TripStoreFixture.start(), refusal) { tx -> producer.acknowledge(tx, captured, TripStoreFixture.start(), refusal) })
        assertEquals(TripLocalState.START_REFUSED, store.read(attempt(f)).book!!.trips.single().state)
        // Another account on the same device opens nothing of A.
        val stale = attempt(f)
        f.identity = EnterpriseLocalIdentity(C, ORIGIN, C); f.fence.lock()
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { store.read(stale) } }
        f.confirm(partition(owner = C))
        assertNull(store.read(attempt(f, partition(owner = C))).book)
    }

    @Test fun `the producer refuses on the device and nothing is written`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition())
        val queue = ProtectedEnterpriseQueue(f.store); val store = ProtectedTripStore(f.store)
        TripStoreFixture.save(queue, attempt(f), TripStoreFixture.start())
        val before = store.read(attempt(f))
        // A second unfinished trip on the same asset, an event of an unknown trip and a non-trip operation change nothing.
        for (refused in listOf(TripStoreFixture.start(op = 602, trip = 611), TripStoreFixture.stop(op = 603, trip = 612, start = 602))) {
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { TripStoreFixture.save(queue, attempt(f), refused) } }
            assertNull(queue.entry(attempt(f), refused.operationId))
        }
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
            val captured = attempt(f); val validation = operation(operationId = id(604))
            queue.save(captured, validation) { tx -> EnterpriseTripProducers.all(TripStoreFixture.hints).first().save(tx, captured, validation) }
        } }
        assertArrayEquals(before.book!!.canonicalBytes, store.read(attempt(f)).book!!.canonicalBytes)
    }

    @Test fun `a generation written before this task opens without a book and keeps its entries`(): Unit = runBlocking {
        val vault = root.resolve("vault"); val provider = root.resolve("provider")
        val f = VaultFixture(vault, FileTestVaultKeys(provider)); f.confirm(partition())
        f.store.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(7)))
        val queue = ProtectedEnterpriseQueue(f.store)
        val legacy = operation(operationId = id(500))
        queue.save(attempt(f), legacy) { it.put(EnterpriseLocalRecord(partition(), B, byteArrayOf(8))) }
        val reopened = VaultFixture(vault, FileTestVaultKeys(provider))
        val store = ProtectedTripStore(reopened.store)
        assertNull(store.read(attempt(reopened)).book)
        TripStoreFixture.save(ProtectedEnterpriseQueue(reopened.store), attempt(reopened), TripStoreFixture.start())
        assertEquals(1, store.read(attempt(reopened)).book!!.trips.size)
        assertArrayEquals(byteArrayOf(7), reopened.store.record(partition(), A)!!.envelope)
        assertArrayEquals(byteArrayOf(8), reopened.store.record(partition(), B)!!.envelope)
        assertArrayEquals(legacy.canonicalBytes, ProtectedEnterpriseQueue(reopened.store).entry(attempt(reopened), id(500))!!.operation.canonicalBytes)
    }

    @Test fun `a foreign or unreadable book is never overwritten and reads as unavailable`(): Unit = runBlocking {
        for ((index, stored) in listOf(TripBook.empty(B, id(9)).canonicalBytes, TripBook.empty(id(8), C).canonicalBytes,
            "{\"format\":\"trip-book\",\"version\":2}".toByteArray()).withIndex()) {
            val f = VaultFixture(root.resolve("vault-$index")); f.confirm(partition())
            f.store.put(EnterpriseLocalRecord(partition(), EnterpriseTripProducers.BOOK_RECORD_ID, stored))
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { ProtectedTripStore(f.store).read(attempt(f)) } }
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
                TripStoreFixture.save(ProtectedEnterpriseQueue(f.store), attempt(f), TripStoreFixture.start()) } }
            assertArrayEquals(stored, f.store.record(partition(), EnterpriseTripProducers.BOOK_RECORD_ID)!!.envelope)
            assertNull(ProtectedEnterpriseQueue(f.store).entry(attempt(f), id(600)))
        }
    }

    @Test fun `the sealed generation holds no plaintext trip fact`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition())
        TripStoreFixture.hints.put(id(600), TripAssetLabel("SYNTHETIC-PRIVATE-TRUCK", "F-9", "PRV1A23"), false)
        TripStoreFixture.save(ProtectedEnterpriseQueue(f.store), attempt(f), TripStoreFixture.start())
        TripStoreFixture.hints.remove(id(600))
        TripStoreFixture.save(ProtectedEnterpriseQueue(f.store), attempt(f), TripStoreFixture.stop())
        Files.walk(root).use { files ->
            files.filter(Files::isRegularFile).forEach { file ->
                val text = String(Files.readAllBytes(file), Charsets.ISO_8859_1)
                assertFalse(text.contains("SYNTHETIC-PRIVATE-TRUCK") || text.contains("trip-book") || text.contains(id(610)) ||
                    text.contains("184320000") || text.contains("trip.start"), file.toString())
            }
        }
    }
}

/** Independent host process saving a trip start; halts at the requested durability boundary. */
object ProtectedTripProcessProbe {
    @JvmStatic fun main(args: Array<String>): Unit = runBlocking {
        val action = args[2]
        val keys = FileTestVaultKeys(Path.of(args[1])) { alias ->
            if (action == "after" && alias.endsWith("_h")) Runtime.getRuntime().halt(ProtectedQueueProcessProbe.AFTER_TRUSTED_HEAD)
        }
        val f = VaultFixture(Path.of(args[0]), keys) { real -> object : EnterpriseVaultFiles by real {
            override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                if (action == "before" && !descriptor) Runtime.getRuntime().halt(ProtectedQueueProcessProbe.BEFORE_DURABLE_REPLACE)
                real.replaceGeneration(token, generation, descriptor, ciphertext)
            }
        } }
        TripStoreFixture.save(ProtectedEnterpriseQueue(f.store), attempt(f), TripStoreFixture.stop())
        error("Synthetic process did not reach its requested crash boundary")
    }
}

class ProtectedTripProcessTest {
    @TempDir lateinit var root: Path

    private fun child(directory: Path, keys: Path, stage: String): Process {
        val classpath = linkedSetOf<String>()
        var loader: ClassLoader? = javaClass.classLoader
        while (loader != null) {
            if (loader is java.net.URLClassLoader) classpath += loader.getURLs().map { Path.of(it.toURI()).toString() }
            loader = loader.parent
        }
        classpath += requireNotNull(System.getProperty("java.class.path")).split(java.io.File.pathSeparator)
        return ProcessBuilder(requireNotNull(System.getProperty("java.home")) + "/bin/java", "-cp",
            classpath.joinToString(java.io.File.pathSeparator), ProtectedTripProcessProbe::class.java.name,
            directory.toString(), keys.toString(), stage)
            .redirectErrorStream(true).redirectOutput(root.resolve("$stage.log").toFile()).start()
    }

    @Test fun `process death while an event is saved leaves the operation and its index together or neither`(): Unit = runBlocking {
        for (stage in listOf("before", "after")) {
            val directory = root.resolve("$stage-vault"); val keyDirectory = root.resolve("$stage-provider")
            val f = VaultFixture(directory, FileTestVaultKeys(keyDirectory)); f.confirm(partition())
            TripStoreFixture.save(ProtectedEnterpriseQueue(f.store), attempt(f), TripStoreFixture.start())
            val process = child(directory, keyDirectory, stage)
            try {
                assertTrue(process.waitFor(30, java.util.concurrent.TimeUnit.SECONDS), "Synthetic trip child timed out")
                assertEquals(if (stage == "before") ProtectedQueueProcessProbe.BEFORE_DURABLE_REPLACE
                    else ProtectedQueueProcessProbe.AFTER_TRUSTED_HEAD, process.exitValue(), stage)
            } finally { if (process.isAlive) process.destroyForcibly() }
            val reopened = VaultFixture(directory, FileTestVaultKeys(keyDirectory))
            val snapshot = ProtectedTripStore(reopened.store).read(attempt(reopened))
            val trip = snapshot.book!!.trips.single()
            val entry = ProtectedEnterpriseQueue(reopened.store).entry(attempt(reopened), id(601))
            if (stage == "before") {
                assertEquals(listOf(id(600)), trip.operationIds, stage); assertNull(entry, stage)
            } else {
                assertEquals(listOf(id(600), id(601)), trip.operationIds, stage); assertNotNull(entry, stage)
            }
            assertEquals(trip.operationIds.toSet(), snapshot.entries.keys, "the index and the queue never diverge")
        }
    }
}
