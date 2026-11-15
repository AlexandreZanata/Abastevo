package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.ProtectedQueueFixture.operation
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.fleet.ProtectedReferenceCacheStore
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.fleet.ReferenceCache
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedRequest
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedRequestSnapshot
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedResponse
import java.nio.file.Files
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/** Synthetic reference caches bound like the vault fixture's grant: membership B, enrollment C. */
internal object ReferenceCacheFixture {
    fun cache(name: String, revision: Long = 1, enrollment: String = C, cursor: String = "cursor-$revision"): ReferenceCache {
        val request = ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(id(700), id(701), "synthetic-reference-access", B, enrollment, null))
        val raw = """{"schema_version":1,"request_id":"${id(700)}","membership_id":"$B","enrollment_id":"$enrollment","mode":"RESET","reason":"NEW_CURSOR","from_revision":null,"revision":$revision,"confirmed_at":"2026-11-15T12:00:00Z","cursor":"$cursor","entity_types":["asset"],"changes":[{"revision":1,"entity_type":"asset","entity_id":"${id(30)}","op":"UPSERT","asset":{"id":"${id(30)}","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"$name","fleet_number":"F-1","plate":"ABC1D23","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[],"assignment":{"kind":"PRIMARY","valid_to":null}}}]}"""
        return ReferenceCache.apply(null, request, ReferenceFeedResponse.parse(raw.toByteArray(), request, null),
            EnterpriseDeviceClock("boot-7", 1000, 10000))
    }
}

class ProtectedReferenceCacheStoreTest {
    @TempDir lateinit var root: Path

    @Test fun `the cache commits into the exact partition and nothing else can read it`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition()); f.confirm(partition(org = B))
        val store = ProtectedReferenceCacheStore(f.store)
        val own = attempt(f)
        assertNull(store.read(own))
        val first = ReferenceCacheFixture.cache("Caminhão privado")
        assertTrue(store.replace(own, null, first))
        assertTrue(first.sameState(store.read(attempt(f))))
        assertNull(store.read(attempt(f, partition(org = B))), "Another organization of the same account reads nothing")
        // Another account on the same device: its own partition is empty, and A's cannot be opened under it.
        val stale = attempt(f)
        f.identity = EnterpriseLocalIdentity(C, ORIGIN, C); f.fence.lock()
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { store.read(stale) } }
        f.confirm(partition(owner = C))
        assertNull(store.read(attempt(f, partition(owner = C))))
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { store.read(attempt(f, partition())) } }
    }

    @Test fun `compare and set refuses a stale expectation and keeps the newer cache`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition())
        val store = ProtectedReferenceCacheStore(f.store)
        val first = ReferenceCacheFixture.cache("Primeiro"); val second = ReferenceCacheFixture.cache("Segundo", 2)
        assertTrue(store.replace(attempt(f), null, first))
        assertFalse(store.replace(attempt(f), null, second), "An absent expectation cannot overwrite a stored cache")
        assertFalse(store.replace(attempt(f), second, first))
        assertTrue(store.replace(attempt(f), first, second))
        assertTrue(second.sameState(store.read(attempt(f))))
    }

    @Test fun `a generation written before this task opens with an empty cache and keeps its entries`(): Unit = runBlocking {
        val vault = root.resolve("vault"); val provider = root.resolve("provider")
        val f = VaultFixture(vault, FileTestVaultKeys(provider)); f.confirm(partition())
        f.store.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(7)))
        val queue = ProtectedEnterpriseQueue(f.store)
        queue.save(attempt(f), operation(operationId = id(500))) { it.put(EnterpriseLocalRecord(partition(), B, byteArrayOf(8))) }
        val reopened = VaultFixture(vault, FileTestVaultKeys(provider))
        val store = ProtectedReferenceCacheStore(reopened.store)
        assertNull(store.read(attempt(reopened)))
        assertTrue(store.replace(attempt(reopened), null, ReferenceCacheFixture.cache("Caminhão")))
        assertArrayEquals(byteArrayOf(7), reopened.store.record(partition(), A)!!.envelope)
        assertEquals(listOf(id(500)), ProtectedEnterpriseQueue(reopened.store).pending(attempt(reopened)).map { it.operation.operationId })
    }

    @Test fun `an unknown version or another enrollment reads as absent and the next full pull overwrites it`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition())
        val store = ProtectedReferenceCacheStore(f.store)
        val v2 = String(ReferenceCacheFixture.cache("Caminhão").canonicalBytes).replace("\"schema_version\":1", "\"schema_version\":2")
        f.store.putPrivateFile(partition(), ProtectedReferenceCacheStore.ENTRY_ID, v2.toByteArray(), cache = true)
        assertNull(store.read(attempt(f)))
        val fresh = ReferenceCacheFixture.cache("Caminhão")
        assertTrue(store.replace(attempt(f), null, fresh))
        assertTrue(fresh.sameState(store.read(attempt(f))))
        val foreign = ReferenceCacheFixture.cache("Outra matrícula", enrollment = id(55))
        f.store.putPrivateFile(partition(), ProtectedReferenceCacheStore.ENTRY_ID, foreign.canonicalBytes, cache = true)
        assertNull(store.read(attempt(f)), "A cache pulled under another enrollment is never shown")
        assertTrue(store.replace(attempt(f), null, fresh))
    }

    @Test fun `the cached asset never appears outside encryption`(): Unit = runBlocking {
        val f = VaultFixture(root); f.confirm(partition())
        assertTrue(ProtectedReferenceCacheStore(f.store).replace(attempt(f), null, ReferenceCacheFixture.cache("SYNTHETIC-PRIVATE-ASSET")))
        Files.walk(root).use { paths ->
            for (file in paths.filter(Files::isRegularFile).toList()) {
                val text = String(Files.readAllBytes(file), Charsets.ISO_8859_1)
                assertFalse(text.contains("SYNTHETIC-PRIVATE-ASSET") || text.contains(id(30)) || text.contains("cursor-1"), file.toString())
            }
        }
    }
}

/** Independent host process committing a reference cache; halts at the requested durability boundary. */
object ProtectedReferenceProcessProbe {
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
        val store = ProtectedReferenceCacheStore(f.store)
        val captured = attempt(f)
        val prior = store.read(captured)
        store.replace(captured, prior, ReferenceCacheFixture.cache("Depois", 2))
        error("Synthetic process did not reach its requested crash boundary")
    }
}

class ProtectedReferenceProcessTest {
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
            classpath.joinToString(java.io.File.pathSeparator), ProtectedReferenceProcessProbe::class.java.name,
            directory.toString(), keys.toString(), stage)
            .redirectErrorStream(true).redirectOutput(root.resolve("$stage.log").toFile()).start()
    }

    @Test fun `process death while the list refreshes leaves exactly the old or the new cache`(): Unit = runBlocking {
        for (stage in listOf("before", "after")) {
            val directory = root.resolve("$stage-vault"); val keyDirectory = root.resolve("$stage-provider")
            val f = VaultFixture(directory, FileTestVaultKeys(keyDirectory)); f.confirm(partition())
            val old = ReferenceCacheFixture.cache("Antes", 1)
            assertTrue(ProtectedReferenceCacheStore(f.store).replace(attempt(f), null, old))
            f.store.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(7)))
            val process = child(directory, keyDirectory, stage)
            try {
                assertTrue(process.waitFor(30, java.util.concurrent.TimeUnit.SECONDS), "Synthetic reference child timed out")
                assertEquals(if (stage == "before") ProtectedQueueProcessProbe.BEFORE_DURABLE_REPLACE
                    else ProtectedQueueProcessProbe.AFTER_TRUSTED_HEAD, process.exitValue(), stage)
            } finally { if (process.isAlive) process.destroyForcibly() }
            val reopened = VaultFixture(directory, FileTestVaultKeys(keyDirectory))
            val restored = ProtectedReferenceCacheStore(reopened.store).read(attempt(reopened))
            val expected = if (stage == "before") old else ReferenceCacheFixture.cache("Depois", 2)
            assertTrue(expected.sameState(restored), stage)
            assertArrayEquals(byteArrayOf(7), reopened.store.record(partition(), A)!!.envelope, "Other committed work is intact")
        }
    }
}
