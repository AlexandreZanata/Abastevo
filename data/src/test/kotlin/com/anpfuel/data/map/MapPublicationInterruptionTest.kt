package com.anpfuel.data.map

import com.anpfuel.data.local.map.*
import java.io.File
import java.security.MessageDigest
import kotlinx.coroutines.*
import okhttp3.OkHttpClient
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class MapPublicationInterruptionTest {
    @TempDir lateinit var root: File
    private val oldBytes = "complete previous map".toByteArray()
    private val newBytes = "complete replacement map".toByteArray()
    private fun hash(bytes: ByteArray) = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }
    private class Store(val rows: List<MapPackageEntity>) : MapPackageStore {
        val generations = mutableMapOf(
            "a" to MapGenerationEntity("a", "a".repeat(64), true, 1, "2601"),
            "b" to MapGenerationEntity("b", "b".repeat(64), false, null, "2601"),
        )
        var active = "a"
        override suspend fun activeGeneration() = generations[active]
        override suspend fun generation(generation: String) = generations[generation]
        override suspend fun packages(generation: String) = rows.filter { it.generation == generation }
        override suspend fun activate(generation: String, activatedAtMillis: Long) {
            active = generation
            generations.replaceAll { id, value -> value.copy(active = id == generation) }
        }
        override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
        override suspend fun deleteInactive(generation: String) = Unit
        override suspend fun updatePackage(generation: String, packageId: String, status: String, downloadedBytes: Long, etag: String?, error: String?, updatedAtMillis: Long) = Unit
    }
    private fun setup(observer: MapActivationObserver = MapActivationObserver.None): Pair<MapPackageRepository, Store> {
        val rows = listOf("a" to oldBytes, "b" to newBytes).flatMap { (gen, bytes) ->
            listOf("rect.mwm", "World.mwm").map { name ->
                if (gen == "b") File(root, "staging/map-packages/b/2601/$name").apply { parentFile.mkdirs(); writeBytes(bytes) }
                File(root, "native/2601/$name").apply { parentFile.mkdirs(); writeBytes(oldBytes) }
                MapPackageEntity(name.removeSuffix(".mwm"), gen, "REGION", "2601/$name", bytes.size.toLong(), hash(bytes), null, PACKAGE_READY, bytes.size.toLong(), null, 1)
            }
        }
        val store = Store(rows)
        return MapPackageRepository(store, File(root, "staging"), OkHttpClient(), nativeRoot = File(root, "native"), activationObserver = observer) to store
    }
    @Test fun `cancellation after one swap restores every active file`() = runBlocking {
        val (repo, store) = setup()
        lateinit var publication: Job
        var swaps = 0
        repo.publishRenameHook = { _, _, _ ->
            if (++swaps == 2) { publication.cancel(); throw CancellationException("cancel during publication") }
        }
        publication = launch(start = CoroutineStart.LAZY) { repo.activate("b") }
        publication.start(); publication.join()
        assertTrue(publication.isCancelled)
        assertEquals("a", store.active)
        assertArrayEquals(oldBytes, File(root, "native/2601/rect.mwm").readBytes())
        assertArrayEquals(oldBytes, File(root, "native/2601/World.mwm").readBytes())
        assertTrue(repo.publishedState(forceVerify = true)!!.complete)
    }
    @Test fun `crash while backing up does not restore a partial backup over active data`() = runBlocking {
        val (repo, _) = setup()
        val journal = File(root, "native/2601/.rollback").apply { mkdirs() }
        File(journal, "generation.txt").writeText("b")
        File(journal, "preparing").writeText("preparing")
        File(journal, "backup/rect.mwm").apply { parentFile.mkdirs(); writeText("partial") }
        assertTrue(repo.publishedState(forceVerify = true)!!.complete)
        assertArrayEquals(oldBytes, File(root, "native/2601/rect.mwm").readBytes())
        assertFalse(journal.exists())
    }
    @Test fun `native readers are detached before replacement and restored afterwards`() = runBlocking {
        val events = mutableListOf<String>()
        val observer = object : MapActivationObserver {
            override suspend fun beforeActivation() {
                events += "before"
                assertArrayEquals(oldBytes, File(root, "native/2601/rect.mwm").readBytes())
            }
            override suspend fun afterActivation() {
                events += "after"
                assertArrayEquals(newBytes, File(root, "native/2601/rect.mwm").readBytes())
            }
        }
        val (repo, store) = setup(observer)
        assertTrue(repo.activate("b"))
        assertEquals("b", store.active)
        assertEquals(listOf("before", "after"), events)
    }
    @Test fun `cancellation while detaching native readers still restores the native storage`() = runBlocking {
        val events = mutableListOf<String>()
        val observer = object : MapActivationObserver {
            override suspend fun beforeActivation() { events += "detach"; throw CancellationException("cancel after detach") }
            override suspend fun afterActivation() { events += "restore" }
        }
        val (repo, store) = setup(observer)
        assertThrows(CancellationException::class.java) { runBlocking { repo.activate("b") } }
        assertEquals("a", store.active)
        assertEquals(listOf("detach", "restore"), events)
    }

    @Test fun `lazy snapshot readers retain A bytes after a same-path native swap`() = runBlocking {
        val (repo, _) = setup()
        for (name in listOf("rect.mwm", "World.mwm")) {
            File(root, "staging/map-packages/a/2601/$name").apply { parentFile.mkdirs(); writeBytes(oldBytes) }
        }
        repo.withActiveGenerationLease { generation, files ->
            assertEquals("a", generation)
            assertTrue(repo.activate("b"))
            // Open after the swap, rather than relying only on an already-open descriptor.
            files.forEach { assertArrayEquals(oldBytes, it.readBytes()) }
        }
    }

}
