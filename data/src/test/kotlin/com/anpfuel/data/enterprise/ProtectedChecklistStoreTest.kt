package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistProducers
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.checklist.ProtectedChecklistStore
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.maintenance.checklist.*
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import java.nio.file.Files
import java.nio.file.Path
import java.time.Instant
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/** Synthetic checklist book values bound like the vault fixture's grant: membership B, enrollment C. */
internal object ChecklistStoreFixture {
    val asset = id(720)
    val t0: Instant = Instant.parse("2026-11-15T11:40:00Z")
    val applicable = ChecklistApplicable(asset, ChecklistContext.DEPARTURE, ChecklistMatch.ORGANIZATION,
        ChecklistCachedTemplate(id(730), "SYNTHETIC-PRIVATE-CHECKLIST", 1, Instant.parse("2026-11-14T10:00:00Z"),
            listOf(ChecklistItem("Luzes", optional = true), ChecklistItem("SYNTHETIC-PRIVATE-BRAKES", critical = true), ChecklistItem("Pneus"))),
        ChecklistCachedPolicy(1, PolicyMode.BLOCK, 43_200, Instant.parse("2026-11-14T10:00:00Z")), t0.minusSeconds(60))
    private val producer = EnterpriseChecklistProducers.all().single()
    fun complete(p: EnterprisePartition, book: ChecklistBook, run: String, op: Int, at: Instant): EnterpriseOperation {
        val local = book.run(run)!!
        return ChecklistRunOperations.complete(ChecklistRunEnvelope(SyncBinding.of(p.ownerAccount, p.origin, p.organizationId), B, C, id(op),
            TimeDeclaration(at, "America/Cuiaba", -14_400)), run, local.assetId, "DEPARTURE", local.template.templateId, 1, 1,
            EnterpriseTime.formatInstant(local.startedAt), local.answers.map { ChecklistRunAnswerInput(it.answer!!, it.note) },
            book.evaluate(run, at).outcome.name, false)
    }
    suspend fun save(queue: ProtectedEnterpriseQueue, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation): Boolean =
        queue.save(attempt, operation) { tx -> producer.save(tx, attempt, operation) }
}

class ProtectedChecklistStoreTest {
    @TempDir lateinit var root: Path
    private val f = ChecklistStoreFixture

    @Test fun `answers are kept without an operation and the completion commits with its operation in the exact partition`(): Unit = runBlocking {
        val v = VaultFixture(root); v.confirm(partition()); v.confirm(partition(org = B))
        val store = ProtectedChecklistStore(v.store); val queue = ProtectedEnterpriseQueue(v.store)
        assertNull(store.read(attempt(v)).book)
        store.update(attempt(v)) { it.withApplicable(f.applicable) }
        val run = id(800)
        store.update(attempt(v)) { it.withStart(run, f.asset, ChecklistContext.DEPARTURE, f.t0) }
        store.update(attempt(v)) { it.withAnswer(run, 1, ChecklistAnswer.OK, null, f.t0) }
        assertEquals(ChecklistAnswer.OK, store.read(attempt(v)).book!!.run(run)!!.answers[0].answer)
        assertTrue(queue.pending(attempt(v)).isEmpty(), "an answer is no operation")
        store.update(attempt(v)) { it.withAnswer(run, 2, ChecklistAnswer.NOT_OK, "SYNTHETIC-PRIVATE-NOTE", f.t0).withAnswer(run, 3, ChecklistAnswer.OK, null, f.t0) }
        val book = store.read(attempt(v)).book!!
        val operation = f.complete(partition(), book, run, 810, f.t0.plusSeconds(60))
        assertTrue(f.save(queue, attempt(v), operation))
        assertFalse(f.save(queue, attempt(v), operation), "an identical retry is idempotent")
        val snapshot = store.read(attempt(v))
        assertEquals(ChecklistOutcome.BLOCKING, snapshot.book!!.run(run)!!.deviceOutcome)
        assertEquals(setOf(id(810)), snapshot.entries.keys)
        assertArrayEquals(operation.canonicalBytes, snapshot.entries.getValue(id(810)).operation.canonicalBytes)
        assertNull(store.read(attempt(v, partition(org = B))).book, "another organization of the same account reads nothing")
        // A local refusal inside the protected write keeps its own failure and writes nothing.
        val before = store.read(attempt(v)).book!!.canonicalBytes
        val refusal = assertThrows(ChecklistLocalException::class.java) { runBlocking {
            store.update(attempt(v)) { it.withAnswer(run, 1, ChecklistAnswer.NOT_OK, null, f.t0) } } }
        assertEquals(ChecklistLocalFailure.RUN_NOT_OPEN, refusal.failure)
        assertArrayEquals(before, store.read(attempt(v)).book!!.canonicalBytes)
        // Another account on the same device opens nothing of A.
        val stale = attempt(v)
        v.identity = EnterpriseLocalIdentity(C, ORIGIN, C); v.fence.lock()
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { store.read(stale) } }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { store.update(stale) { it } } }
    }

    @Test fun `the producer refuses an operation that is not the saved run and nothing is written`(): Unit = runBlocking {
        val v = VaultFixture(root); v.confirm(partition())
        val store = ProtectedChecklistStore(v.store); val queue = ProtectedEnterpriseQueue(v.store)
        val run = id(820)
        store.update(attempt(v)) { it.withApplicable(f.applicable).withStart(run, f.asset, ChecklistContext.DEPARTURE, f.t0) }
        store.update(attempt(v)) { (1..3).fold(it) { b, p -> b.withAnswer(run, p, ChecklistAnswer.OK, null, f.t0) } }
        val book = store.read(attempt(v)).book!!
        val other = book.withAnswer(run, 2, ChecklistAnswer.NOT_OK, null, f.t0)
        val mismatched = f.complete(partition(), other, run, 830, f.t0.plusSeconds(60))
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.save(queue, attempt(v), mismatched) } }
        assertNull(queue.entry(attempt(v), id(830)))
        assertArrayEquals(book.canonicalBytes, store.read(attempt(v)).book!!.canonicalBytes)
    }

    @Test fun `a foreign or unreadable book is never replaced and reads as unavailable`(): Unit = runBlocking {
        for ((index, stored) in listOf(ChecklistBook.empty(B, id(9)).canonicalBytes, ChecklistBook.empty(id(8), C).canonicalBytes,
            "{\"format\":\"checklist-book\",\"version\":2}".toByteArray()).withIndex()) {
            val v = VaultFixture(root.resolve("vault-$index")); v.confirm(partition())
            v.store.put(EnterpriseLocalRecord(partition(), EnterpriseChecklistProducers.BOOK_RECORD_ID, stored))
            val store = ProtectedChecklistStore(v.store)
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { store.read(attempt(v)) } }
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { store.update(attempt(v)) { it.withApplicable(f.applicable) } } }
            assertArrayEquals(stored, v.store.record(partition(), EnterpriseChecklistProducers.BOOK_RECORD_ID)!!.envelope)
        }
    }

    @Test fun `the sealed generation holds no plaintext checklist fact`(): Unit = runBlocking {
        val v = VaultFixture(root); v.confirm(partition())
        val store = ProtectedChecklistStore(v.store)
        val run = id(840)
        store.update(attempt(v)) { it.withApplicable(f.applicable).withStart(run, f.asset, ChecklistContext.DEPARTURE, f.t0)
            .withAnswer(run, 2, ChecklistAnswer.NOT_OK, "SYNTHETIC-PRIVATE-NOTE", f.t0) }
        Files.walk(root).use { files ->
            files.filter(Files::isRegularFile).forEach { file ->
                val text = String(Files.readAllBytes(file), Charsets.ISO_8859_1)
                assertFalse(text.contains("SYNTHETIC-PRIVATE") || text.contains("checklist-book") || text.contains(run) ||
                    text.contains(f.asset) || text.contains("NOT_OK"), file.toString())
            }
        }
    }
}

/** Independent host process saving an answer; halts at the requested durability boundary. */
object ProtectedChecklistProcessProbe {
    @JvmStatic fun main(args: Array<String>): Unit = runBlocking {
        val action = args[2]
        val keys = FileTestVaultKeys(Path.of(args[1])) { alias ->
            if (action == "after" && alias.endsWith("_h")) Runtime.getRuntime().halt(ProtectedQueueProcessProbe.AFTER_TRUSTED_HEAD)
        }
        val v = VaultFixture(Path.of(args[0]), keys) { real -> object : EnterpriseVaultFiles by real {
            override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                if (action == "before" && !descriptor) Runtime.getRuntime().halt(ProtectedQueueProcessProbe.BEFORE_DURABLE_REPLACE)
                real.replaceGeneration(token, generation, descriptor, ciphertext)
            }
        } }
        ProtectedChecklistStore(v.store).update(attempt(v)) { it.withAnswer(id(850), 2, ChecklistAnswer.NOT_OK, "Freios rangendo", ChecklistStoreFixture.t0) }
        error("Synthetic process did not reach its requested crash boundary")
    }
}

class ProtectedChecklistProcessTest {
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
            classpath.joinToString(java.io.File.pathSeparator), ProtectedChecklistProcessProbe::class.java.name,
            directory.toString(), keys.toString(), stage)
            .redirectErrorStream(true).redirectOutput(root.resolve("$stage.log").toFile()).start()
    }

    @Test fun `process death while an answer is saved keeps the run in progress with the answer or without it, never torn`(): Unit = runBlocking {
        for (stage in listOf("before", "after")) {
            val directory = root.resolve("$stage-vault"); val keyDirectory = root.resolve("$stage-provider")
            val v = VaultFixture(directory, FileTestVaultKeys(keyDirectory)); v.confirm(partition())
            ProtectedChecklistStore(v.store).update(attempt(v)) {
                it.withApplicable(ChecklistStoreFixture.applicable).withStart(id(850), ChecklistStoreFixture.asset, ChecklistContext.DEPARTURE, ChecklistStoreFixture.t0)
                    .withAnswer(id(850), 1, ChecklistAnswer.OK, null, ChecklistStoreFixture.t0)
            }
            val process = child(directory, keyDirectory, stage)
            try {
                assertTrue(process.waitFor(30, java.util.concurrent.TimeUnit.SECONDS), "Synthetic checklist child timed out")
                assertEquals(if (stage == "before") ProtectedQueueProcessProbe.BEFORE_DURABLE_REPLACE
                    else ProtectedQueueProcessProbe.AFTER_TRUSTED_HEAD, process.exitValue(), stage)
            } finally { if (process.isAlive) process.destroyForcibly() }
            val reopened = VaultFixture(directory, FileTestVaultKeys(keyDirectory))
            val run = ProtectedChecklistStore(reopened.store).read(attempt(reopened)).book!!.run(id(850))!!
            assertFalse(run.completed, stage)
            assertEquals(ChecklistAnswer.OK, run.answers[0].answer, "the earlier answer survives ($stage)")
            if (stage == "before") assertNull(run.answers[1].answer, stage) else assertEquals(ChecklistAnswer.NOT_OK, run.answers[1].answer, stage)
        }
    }
}
