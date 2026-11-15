package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.data.enterprise.ProtectedQueueFixture.ack
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.ProtectedQueueFixture.operation
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.attachment.ProtectedAttachmentStore
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.attachment.*
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import java.nio.file.Files
import java.nio.file.Path
import java.security.MessageDigest
import java.time.Instant
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/** Synthetic attachments of the vault fixture's grant: membership B, enrollment C. */
internal object AttachmentStoreFixture {
    val MARKER = "SYNTHETIC-SEALED-ATTACHMENT".toByteArray()
    fun jpeg(fill: Byte = 3): ByteArray = byteArrayOf(0xff.toByte(), 0xd8.toByte(), 0xff.toByte(), 0xe1.toByte()) + MARKER + ByteArray(64) { fill } +
        byteArrayOf(0xff.toByte(), 0xd9.toByte())
    fun sha(bytes: ByteArray) = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it.toInt() and 255) }
    fun item(n: Int, owner: Int = 500, bytes: ByteArray = jpeg()) = EnterpriseAttachment(id(n), id(n + 1), id(n + 2), id(owner),
        AttachmentTarget(DocumentTargetKind.METER_READING, id(n + 3)), sha(bytes), bytes.size.toLong(), Instant.parse("2026-11-15T12:00:00Z"))
    suspend fun queued(f: VaultFixture, owner: Int = 500) =
        ProtectedEnterpriseQueue(f.store).save(attempt(f), operation(operationId = id(owner))) { it.put(EnterpriseLocalRecord(partition(), id(owner + 1), byteArrayOf(1))) }
}

class ProtectedAttachmentStoreTest {
    @TempDir lateinit var root: Path
    private val x = AttachmentStoreFixture

    @Test fun `capture seals the index and the bytes together and only beside a queued operation of the partition`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition()); f.confirm(partition(org = B))
        val store = ProtectedAttachmentStore(f.store)
        assertNull(store.read(attempt(f)).book)
        assertEquals(AttachmentLocalFailure.UNKNOWN_OPERATION, assertThrows(AttachmentLocalException::class.java) {
            runBlocking { store.capture(attempt(f), x.item(100), x.jpeg()) } }.failure)
        assertNull(store.read(attempt(f)).book, "a refused capture writes nothing")
        x.queued(f)
        store.capture(attempt(f), x.item(100), x.jpeg())
        val snapshot = store.read(attempt(f))
        assertTrue(snapshot.book!!.item(id(100))!!.sameAs(x.item(100)))
        assertEquals(setOf(id(500)), snapshot.owners.keys)
        assertArrayEquals(operation(operationId = id(500)).canonicalBytes, snapshot.owners.getValue(id(500)).operation.canonicalBytes)
        assertArrayEquals(x.jpeg(), store.bytes(attempt(f), id(100)))
        assertEquals(AttachmentLocalFailure.DUPLICATE, assertThrows(AttachmentLocalException::class.java) {
            runBlocking { store.capture(attempt(f), x.item(100), x.jpeg()) } }.failure)
        // Bytes must equal their declaration.
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { store.capture(attempt(f), x.item(110), x.jpeg(fill = 9)) } }
        assertNull(store.read(attempt(f)).book!!.item(id(110)))
        // BR-OFFL-003: another organization of the same account reads nothing, and its own operation is not an owner here.
        assertNull(store.read(attempt(f, partition(org = B))).book)
        assertNull(store.bytes(attempt(f, partition(org = B)), id(100)))
        // The owner receipt is read from the queue, never copied into the index.
        val captured = attempt(f)
        ProtectedEnterpriseQueue(f.store).acknowledge(captured, operation(operationId = id(500)), ack(operation(operationId = id(500)))) { }
        assertNotNull(store.read(attempt(f)).owners.getValue(id(500)).acknowledgement)
        // Another account on the same device opens nothing.
        val stale = attempt(f)
        f.identity = EnterpriseLocalIdentity(C, ORIGIN, C); f.fence.lock()
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { store.read(stale) } }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { store.bytes(stale, id(100)) } }
        f.confirm(partition(owner = C))
        assertNull(store.read(attempt(f, partition(owner = C))).book)
    }

    @Test fun `transitions are compare-and-set and verified bytes leave the generation in the same write`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition())
        val store = ProtectedAttachmentStore(f.store)
        x.queued(f)
        val item = x.item(100)
        store.capture(attempt(f), item, x.jpeg())
        val declared = item.copy(state = AttachmentState.DECLARED, storageRevision = 1)
        assertTrue(store.transition(attempt(f), item, declared))
        assertFalse(store.transition(attempt(f), item, declared), "a stale expectation writes nothing")
        val refused = declared.copy(state = AttachmentState.REFUSED, refusal = AttachmentRefusal.FORBIDDEN)
        assertTrue(store.transition(attempt(f), declared, refused))
        assertArrayEquals(x.jpeg(), store.bytes(attempt(f), id(100)), "refused bytes are kept")
        x.queued(f, owner = 510)
        val second = x.item(200, owner = 510)
        store.capture(attempt(f), second, x.jpeg())
        val stored = second.copy(state = AttachmentState.STORED, storageRevision = 3)
        assertTrue(store.transition(attempt(f), second, stored))
        assertNull(store.bytes(attempt(f), id(200)), "verified bytes are removed")
        assertEquals(AttachmentState.STORED, store.read(attempt(f)).book!!.item(id(200))!!.state)
    }

    @Test fun `the unsettled byte budget holds in real sealed generations and verified bytes free it`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition())
        val store = ProtectedAttachmentStore(f.store)
        fun large(fill: Byte) = x.jpeg(fill).let { small ->
            ByteArray(AttachmentPolicy.MAX_BYTES.toInt()) { fill }.also { small.copyInto(it, 0, 0, 4); it[it.size - 2] = 0xff.toByte(); it[it.size - 1] = 0xd9.toByte() }
        }
        val timings = mutableListOf<Long>()
        val items = (0 until 3).map { n ->
            x.queued(f, owner = 500 + n * 10)
            val bytes = large(n.toByte())
            val item = x.item(100 + n * 10, owner = 500 + n * 10, bytes = bytes)
            val started = System.nanoTime()
            store.capture(attempt(f), item, bytes)
            timings += (System.nanoTime() - started) / 1_000_000
            item
        }
        x.queued(f, owner = 600)
        val fourth = large(9)
        assertEquals(AttachmentLocalFailure.BUDGET, assertThrows(AttachmentLocalException::class.java) {
            runBlocking { store.capture(attempt(f), x.item(200, owner = 600, bytes = fourth), fourth) } }.failure)
        // A small retry-state write re-seals the whole 15 MiB generation (EDR-267 write cost).
        var started = System.nanoTime()
        val declared = items[0].copy(state = AttachmentState.DECLARED, storageRevision = 1)
        assertTrue(store.transition(attempt(f), items[0], declared))
        timings += (System.nanoTime() - started) / 1_000_000
        started = System.nanoTime()
        assertTrue(store.transition(attempt(f), declared, declared.copy(state = AttachmentState.STORED, storageRevision = 3)))
        timings += (System.nanoTime() - started) / 1_000_000
        store.capture(attempt(f), x.item(200, owner = 600, bytes = fourth), fourth)
        assertEquals(3, store.read(attempt(f)).book!!.items.count { it.holdsBytes })
        println("EMP-OFFL-005 host sealed-write milliseconds (5 MiB captures at 0/5/10 MiB, transition at 15 MiB, verification at 15 MiB): $timings")
    }

    @Test fun `a foreign or unreadable index is never overwritten and reads as unavailable`(): Unit = runBlocking {
        for ((index, stored) in listOf(AttachmentBook.empty(B, id(9)).canonicalBytes, AttachmentBook.empty(id(8), C).canonicalBytes,
            "{\"format\":\"attachment-book\",\"version\":2}".toByteArray()).withIndex()) {
            val f = VaultFixture(root.resolve("vault-$index")); f.confirm(partition())
            x.queued(f)
            f.store.put(EnterpriseLocalRecord(partition(), AttachmentBook.RECORD_ID, stored))
            val store = ProtectedAttachmentStore(f.store)
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { store.read(attempt(f)) } }
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { store.capture(attempt(f), x.item(100), x.jpeg()) } }
            assertArrayEquals(stored, f.store.record(partition(), AttachmentBook.RECORD_ID)!!.envelope)
            assertNull(store.bytes(attempt(f), id(100)))
        }
    }

    @Test fun `the sealed generation holds no attachment byte, digest or identifier in plaintext`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition())
        x.queued(f)
        val store = ProtectedAttachmentStore(f.store)
        store.capture(attempt(f), x.item(100), x.jpeg())
        val digest = x.sha(x.jpeg())
        Files.walk(root).use { files ->
            files.filter(Files::isRegularFile).forEach { file ->
                val text = String(Files.readAllBytes(file), Charsets.ISO_8859_1)
                assertFalse(text.contains(String(x.MARKER, Charsets.ISO_8859_1)) || text.contains(digest) || text.contains(id(100)) ||
                    text.contains("attachment-book") || text.contains("METER_READING"), file.toString())
            }
        }
    }
}

/** Independent host process capturing or verifying an attachment; halts at the requested durability boundary. */
object ProtectedAttachmentProcessProbe {
    @JvmStatic fun main(args: Array<String>): Unit = runBlocking {
        val action = args[2]
        val keys = FileTestVaultKeys(Path.of(args[1])) { alias ->
            if (action.endsWith("-after") && alias.endsWith("_h")) Runtime.getRuntime().halt(ProtectedQueueProcessProbe.AFTER_TRUSTED_HEAD)
        }
        val f = VaultFixture(Path.of(args[0]), keys) { real -> object : EnterpriseVaultFiles by real {
            override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                if (action.endsWith("-before") && !descriptor) Runtime.getRuntime().halt(ProtectedQueueProcessProbe.BEFORE_DURABLE_REPLACE)
                real.replaceGeneration(token, generation, descriptor, ciphertext)
            }
        } }
        val store = ProtectedAttachmentStore(f.store)
        val item = AttachmentStoreFixture.item(100)
        if (action.startsWith("capture-")) store.capture(attempt(f), item, AttachmentStoreFixture.jpeg())
        else store.transition(attempt(f), item, item.copy(state = AttachmentState.STORED, storageRevision = 3))
        error("Synthetic process did not reach its requested crash boundary")
    }
}

class ProtectedAttachmentProcessTest {
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
            classpath.joinToString(java.io.File.pathSeparator), ProtectedAttachmentProcessProbe::class.java.name,
            directory.toString(), keys.toString(), stage)
            .redirectErrorStream(true).redirectOutput(root.resolve("$stage.log").toFile()).start()
    }

    @Test fun `process death while an attachment is captured or verified keeps its index and bytes together`(): Unit = runBlocking {
        for (stage in listOf("capture-before", "capture-after", "store-before", "store-after")) {
            val directory = root.resolve("$stage-vault"); val keyDirectory = root.resolve("$stage-provider")
            val f = VaultFixture(directory, FileTestVaultKeys(keyDirectory)); f.confirm(partition())
            AttachmentStoreFixture.queued(f)
            if (stage.startsWith("store-")) ProtectedAttachmentStore(f.store).capture(attempt(f), AttachmentStoreFixture.item(100), AttachmentStoreFixture.jpeg())
            val process = child(directory, keyDirectory, stage)
            try {
                assertTrue(process.waitFor(30, java.util.concurrent.TimeUnit.SECONDS), "Synthetic attachment child timed out")
                assertEquals(if (stage.endsWith("-before")) ProtectedQueueProcessProbe.BEFORE_DURABLE_REPLACE
                    else ProtectedQueueProcessProbe.AFTER_TRUSTED_HEAD, process.exitValue(), stage)
            } finally { if (process.isAlive) process.destroyForcibly() }
            val reopened = VaultFixture(directory, FileTestVaultKeys(keyDirectory))
            val store = ProtectedAttachmentStore(reopened.store)
            val item = store.read(attempt(reopened)).book?.item(id(100))
            val bytes = store.bytes(attempt(reopened), id(100))
            when (stage) {
                "capture-before" -> { assertNull(item, stage); assertNull(bytes, stage) }
                "capture-after" -> { assertEquals(AttachmentState.WAITING_FOR_RECORD, item!!.state); assertArrayEquals(AttachmentStoreFixture.jpeg(), bytes) }
                "store-before" -> { assertEquals(AttachmentState.WAITING_FOR_RECORD, item!!.state, stage); assertArrayEquals(AttachmentStoreFixture.jpeg(), bytes) }
                else -> { assertEquals(AttachmentState.STORED, item!!.state, stage); assertNull(bytes, stage) }
            }
            // The queued operation is never touched by an attachment write.
            assertNotNull(ProtectedEnterpriseQueue(reopened.store).entry(attempt(reopened), id(500)))
        }
    }
}
