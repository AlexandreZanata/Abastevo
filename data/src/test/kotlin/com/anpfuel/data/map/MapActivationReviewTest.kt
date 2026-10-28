package com.anpfuel.data.map

import android.os.StrictMode
import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import java.io.File
import java.io.IOException
import java.security.MessageDigest
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okio.Buffer
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class MapActivationReviewTest {
    @TempDir
    lateinit var root: File

    private fun hash(b: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(b).joinToString("") { "%02x".format(it) }

    @Test
    fun failedRoomCommitMustPreservePreviousNativeGeneration() = runBlocking {
        val a = "previous-native-generation".toByteArray()
        val b = "replacement-native-bytes".toByteArray()
        val gens = listOf(
            MapGenerationEntity("a", "a".repeat(64), true, 1, "2601"),
            MapGenerationEntity("b", "b".repeat(64), false, null, "2601"),
        )
        val pkgs = listOf(a, b).mapIndexed { i, bytes ->
            MapPackageEntity(
                "rect",
                if (i == 0) "a" else "b",
                "REGION",
                "2601/rect.mwm",
                bytes.size.toLong(),
                hash(bytes),
                null,
                PACKAGE_READY,
                bytes.size.toLong(),
                null,
                1,
            )
        }
        val store = object : MapPackageStore {
            override suspend fun activeGeneration() = gens[0]
            override suspend fun generation(generation: String) = gens.firstOrNull { it.generation == generation }
            override suspend fun packages(generation: String) = pkgs.filter { it.generation == generation }
            override suspend fun activate(generation: String, activatedAtMillis: Long) {
                throw IOException("simulated Room commit failure")
            }
            override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
            override suspend fun deleteInactive(generation: String) = Unit
            override suspend fun updatePackage(
                generation: String,
                packageId: String,
                status: String,
                downloadedBytes: Long,
                etag: String?,
                error: String?,
                updatedAtMillis: Long,
            ) = Unit
        }
        val staging = File(root, "staging")
        val native = File(root, "native")
        File(staging, "map-packages/b/2601/rect.mwm").apply {
            parentFile?.mkdirs()
            writeBytes(b)
        }
        val activeFile = File(native, "2601/rect.mwm").apply {
            parentFile?.mkdirs()
            writeBytes(a)
        }
        val repository = MapPackageRepository(store, staging, OkHttpClient(), nativeRoot = native)
        assertThrows(IOException::class.java) {
            runBlocking { repository.activate("b") }
        }
        assertEquals("a", store.activeGeneration()?.generation)
        assertArrayEquals(a, activeFile.readBytes(), "Failed B activation must preserve the native files backing active A")
        val state = repository.publishedState(forceVerify = true)
        assertNotNull(state)
        assertEquals("a", state?.generation)
        assertTrue(state!!.complete)
    }

    @Test
    fun failureAfterFirstFileOfMultiPackageGenerationRestoresAllActiveFiles() = runBlocking {
        val aRect = "active-a-rect-bytes".toByteArray()
        val aWorld = "active-a-world-bytes".toByteArray()
        val bRect = "candidate-b-rect-bytes".toByteArray()
        val bWorld = "candidate-b-world-bytes".toByteArray()
        val bExtra = "candidate-b-extra-bytes".toByteArray()

        val gens = mutableMapOf(
            "a" to MapGenerationEntity("a", "a".repeat(64), true, 1L, "2601"),
            "b" to MapGenerationEntity("b", "b".repeat(64), false, null, "2601"),
        )
        val pkgs = listOf(
            MapPackageEntity("rect", "a", "REGION", "2601/rect.mwm", aRect.size.toLong(), hash(aRect), null, PACKAGE_READY, aRect.size.toLong(), null, 1L),
            MapPackageEntity("world", "a", "OVERVIEW", "2601/World.mwm", aWorld.size.toLong(), hash(aWorld), null, PACKAGE_READY, aWorld.size.toLong(), null, 1L),
            MapPackageEntity("rect", "b", "REGION", "2601/rect.mwm", bRect.size.toLong(), hash(bRect), null, PACKAGE_READY, bRect.size.toLong(), null, 1L),
            MapPackageEntity("world", "b", "OVERVIEW", "2601/World.mwm", bWorld.size.toLong(), hash(bWorld), null, PACKAGE_READY, bWorld.size.toLong(), null, 1L),
            MapPackageEntity("extra", "b", "OVERVIEW", "2601/Extra.mwm", bExtra.size.toLong(), hash(bExtra), null, PACKAGE_READY, bExtra.size.toLong(), null, 1L),
        )
        val store = object : MapPackageStore {
            override suspend fun activeGeneration() = gens.values.firstOrNull { it.active }
            override suspend fun generation(generation: String) = gens[generation]
            override suspend fun packages(generation: String) = pkgs.filter { it.generation == generation }
            override suspend fun activate(generation: String, activatedAtMillis: Long) {
                gens.replaceAll { k, v -> v.copy(active = k == generation, activatedAtMillis = if (k == generation) activatedAtMillis else v.activatedAtMillis) }
            }
            override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
            override suspend fun deleteInactive(generation: String) = Unit
            override suspend fun updatePackage(
                generation: String,
                packageId: String,
                status: String,
                downloadedBytes: Long,
                etag: String?,
                error: String?,
                updatedAtMillis: Long,
            ) = Unit
        }
        val staging = File(root, "staging")
        val native = File(root, "native")
        File(native, "2601/rect.mwm").apply { parentFile?.mkdirs(); writeBytes(aRect) }
        File(native, "2601/World.mwm").apply { parentFile?.mkdirs(); writeBytes(aWorld) }
        File(staging, "map-packages/b/2601/rect.mwm").apply { parentFile?.mkdirs(); writeBytes(bRect) }
        File(staging, "map-packages/b/2601/World.mwm").apply { parentFile?.mkdirs(); writeBytes(bWorld) }
        File(staging, "map-packages/b/2601/Extra.mwm").apply { parentFile?.mkdirs(); writeBytes(bExtra) }

        val repository = MapPackageRepository(store, staging, OkHttpClient(), nativeRoot = native)
        var renames = 0
        repository.publishRenameHook = { _, _, _ ->
            renames++
            if (renames == 2) {
                throw IOException("simulated failure before second file swap")
            }
        }
        assertThrows(IOException::class.java) {
            runBlocking { repository.activate("b") }
        }
        repository.publishRenameHook = null

        assertEquals("a", store.activeGeneration()?.generation)
        assertArrayEquals(aRect, File(native, "2601/rect.mwm").readBytes())
        assertArrayEquals(aWorld, File(native, "2601/World.mwm").readBytes())
        assertFalse(File(native, "2601/Extra.mwm").exists())
        assertFalse(File(native, "2601/.rollback").exists())
    }

    @Test
    fun crashBetweenFileSwapAndRoomCommitRecoversActiveGenerationOnRestart() = runBlocking {
        val aRect = "active-a-rect-before-crash".toByteArray()
        val bRect = "uncommitted-b-rect-after-crash".toByteArray()
        val bNew = "uncommitted-b-new-file".toByteArray()

        val gens = mapOf(
            "a" to MapGenerationEntity("a", "a".repeat(64), true, 1L, "2601"),
            "b" to MapGenerationEntity("b", "b".repeat(64), false, null, "2601"),
        )
        val pkgs = listOf(
            MapPackageEntity("rect", "a", "REGION", "2601/rect.mwm", aRect.size.toLong(), hash(aRect), null, PACKAGE_READY, aRect.size.toLong(), null, 1L),
            MapPackageEntity("rect", "b", "REGION", "2601/rect.mwm", bRect.size.toLong(), hash(bRect), null, PACKAGE_READY, bRect.size.toLong(), null, 1L),
            MapPackageEntity("new", "b", "OVERVIEW", "2601/new.mwm", bNew.size.toLong(), hash(bNew), null, PACKAGE_READY, bNew.size.toLong(), null, 1L),
        )
        val store = object : MapPackageStore {
            override suspend fun activeGeneration() = gens["a"]
            override suspend fun generation(generation: String) = gens[generation]
            override suspend fun packages(generation: String) = pkgs.filter { it.generation == generation }
            override suspend fun activate(generation: String, activatedAtMillis: Long) = Unit
            override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
            override suspend fun deleteInactive(generation: String) = Unit
            override suspend fun updatePackage(
                generation: String,
                packageId: String,
                status: String,
                downloadedBytes: Long,
                etag: String?,
                error: String?,
                updatedAtMillis: Long,
            ) = Unit
        }
        val staging = File(root, "staging")
        val native = File(root, "native")
        val versionDir = File(native, "2601").apply { mkdirs() }
        // Simulate state left on disk if the process died after swapping B's files
        // into native/2601/ but before Room committed "b" as active.
        val rollbackDir = File(versionDir, ".rollback").apply { mkdirs() }
        File(rollbackDir, "generation.txt").writeText("b")
        File(rollbackDir, "created.txt").writeText("new.mwm")
        File(rollbackDir, "backup").apply { mkdirs() }
        File(rollbackDir, "backup/rect.mwm").writeBytes(aRect)
        File(versionDir, "rect.mwm").writeBytes(bRect)
        File(versionDir, "new.mwm").writeBytes(bNew)

        // New repository instance on app restart:
        val restarted = MapPackageRepository(store, staging, OkHttpClient(), nativeRoot = native)
        val state = restarted.publishedState(forceVerify = true)
        assertNotNull(state)
        assertEquals("a", state!!.generation)
        assertTrue(state.complete)
        assertArrayEquals(aRect, File(versionDir, "rect.mwm").readBytes())
        assertFalse(File(versionDir, "new.mwm").exists())
        assertFalse(rollbackDir.exists())
    }

    @Test
    fun readerLeasePreservesPriorVersionDirectoryAcrossActivationUntilReleased() = runBlocking {
        val aBytes = "generation-a-2601-bytes".toByteArray()
        val bBytes = "generation-b-2602-bytes".toByteArray()
        val gens = mutableMapOf(
            "a" to MapGenerationEntity("a", "a".repeat(64), true, 1L, "2601"),
            "b" to MapGenerationEntity("b", "b".repeat(64), false, null, "2602"),
        )
        val pkgs = listOf(
            MapPackageEntity("rect-a", "a", "REGION", "2601/rect-a.mwm", aBytes.size.toLong(), hash(aBytes), null, PACKAGE_READY, aBytes.size.toLong(), null, 1L),
            MapPackageEntity("rect-b", "b", "REGION", "2602/rect-b.mwm", bBytes.size.toLong(), hash(bBytes), null, PACKAGE_READY, bBytes.size.toLong(), null, 1L),
        )
        val store = object : MapPackageStore {
            override suspend fun activeGeneration() = gens.values.firstOrNull { it.active }
            override suspend fun generation(generation: String) = gens[generation]
            override suspend fun packages(generation: String) = pkgs.filter { it.generation == generation }
            override suspend fun activate(generation: String, activatedAtMillis: Long) {
                gens.replaceAll { k, v -> v.copy(active = k == generation, activatedAtMillis = if (k == generation) activatedAtMillis else v.activatedAtMillis) }
            }
            override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
            override suspend fun deleteInactive(generation: String) = Unit
            override suspend fun updatePackage(
                generation: String,
                packageId: String,
                status: String,
                downloadedBytes: Long,
                etag: String?,
                error: String?,
                updatedAtMillis: Long,
            ) = Unit
        }
        val staging = File(root, "staging")
        val native = File(root, "native")
        File(staging, "map-packages/a/2601/rect-a.mwm").apply { parentFile?.mkdirs(); writeBytes(aBytes) }
        File(staging, "map-packages/b/2602/rect-b.mwm").apply { parentFile?.mkdirs(); writeBytes(bBytes) }
        File(native, "2601/rect-a.mwm").apply { parentFile?.mkdirs(); writeBytes(aBytes) }

        val repository = MapPackageRepository(store, staging, OkHttpClient(), nativeRoot = native)
        repository.acquireGenerationLease("a")
        try {
            assertTrue(repository.activate("b"))
            assertEquals("b", store.activeGeneration()?.generation)
            // While lease on "a" is held, a reader can still open files in native/2601/
            assertTrue(File(native, "2601/rect-a.mwm").isFile)
            assertArrayEquals(aBytes, File(native, "2601/rect-a.mwm").readBytes())
        } finally {
            repository.releaseGenerationLease("a")
        }
        repository.reclaimRetiredGenerations()
        assertFalse(File(native, "2601").exists(), "Retired version directory must be reclaimed once reader lease is released")
        assertTrue(File(native, "2602/rect-b.mwm").isFile)
    }

    @Test
    fun publishedStateExecutesOnIoDispatcherAndAvoidsRepeatingFullHashOnUiPolls() = runBlocking {
        val repBytes = ByteArray(512 * 1024) { (it * 31 + 7).toByte() }
        val gen = MapGenerationEntity("a", "a".repeat(64), true, 1L, "2601")
        val pkg = MapPackageEntity("rect", "a", "REGION", "2601/rect.mwm", repBytes.size.toLong(), hash(repBytes), null, PACKAGE_READY, repBytes.size.toLong(), null, 1L)
        val store = object : MapPackageStore {
            override suspend fun activeGeneration() = gen
            override suspend fun generation(generation: String) = gen.takeIf { it.generation == generation }
            override suspend fun packages(generation: String) = if (generation == "a") listOf(pkg) else emptyList()
            override suspend fun activate(generation: String, activatedAtMillis: Long) = Unit
            override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
            override suspend fun deleteInactive(generation: String) = Unit
            override suspend fun updatePackage(
                generation: String,
                packageId: String,
                status: String,
                downloadedBytes: Long,
                etag: String?,
                error: String?,
                updatedAtMillis: Long,
            ) = Unit
        }
        val staging = File(root, "staging")
        val native = File(root, "native")
        File(staging, "map-packages/a/2601/rect.mwm").apply { parentFile?.mkdirs(); writeBytes(repBytes) }

        val ioDispatches = AtomicInteger(0)
        val countingIoDispatcher = object : CoroutineDispatcher() {
            override fun dispatch(context: kotlin.coroutines.CoroutineContext, block: Runnable) {
                ioDispatches.incrementAndGet()
                Dispatchers.IO.dispatch(context, block)
            }
        }
        val repository = MapPackageRepository(
            store = store,
            filesRoot = staging,
            http = OkHttpClient(),
            nativeRoot = native,
            ioDispatcher = countingIoDispatcher,
        )
        assertTrue(repository.activate("a"))
        val hashesAfterActivate = repository.fullSha256ComputationCount()
        assertTrue(hashesAfterActivate >= 1)

        val beforePolls = ioDispatches.get()
        repeat(5) {
            val state = repository.publishedState()
            assertEquals(true, state?.complete)
        }
        assertTrue(ioDispatches.get() >= beforePolls + 5, "publishedState must dispatch to ioDispatcher")
        assertEquals(
            hashesAfterActivate,
            repository.fullSha256ComputationCount(),
            "Repeated UI availability polls must not re-hash the full .mwm file when stamp is unchanged",
        )
    }

    @Test
    fun fetchEnvelopeEnforcesOneMiBLimitDuringStreamWithoutContentLengthAndSupportsCancellation() = runBlocking {
        val server = MockWebServer()
        try {
            val oversized = ByteArray(1024 * 1024 + 64 * 1024) { 'x'.code.toByte() }
            server.dispatcher = object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    return if (request.path?.contains("slow") == true) {
                        MockResponse()
                            .setResponseCode(200)
                            .setChunkedBody(Buffer().write(ByteArray(512 * 1024) { 'a'.code.toByte() }), 16 * 1024)
                            .throttleBody(16 * 1024, 100, java.util.concurrent.TimeUnit.MILLISECONDS)
                    } else {
                        // Chunked transfer encoding omits Content-Length header
                        MockResponse()
                            .setResponseCode(200)
                            .setChunkedBody(Buffer().write(oversized), 64 * 1024)
                    }
                }
            }
            server.start()
            val baseUrl = server.url("/").toString().trimEnd('/')
            val dist = MapDistribution(
                http = OkHttpClient(),
                config = MapDistributionConfig(
                    baseUrl = baseUrl,
                    region = "brazil-parana-east",
                    trustedKeys = mapOf("k1" to ByteArray(32) { 1 }),
                    keysInvalid = false,
                ),
            )
            val err = assertThrows(MapDownloadException::class.java) {
                runBlocking { dist.fetchEnvelope("brazil-parana-east") }
            }
            assertEquals("map-manifest-too-large", err.message)

            val slowDist = MapDistribution(
                http = OkHttpClient(),
                config = MapDistributionConfig(
                    baseUrl = baseUrl,
                    region = "slow-region",
                    trustedKeys = mapOf("k1" to ByteArray(32) { 1 }),
                    keysInvalid = false,
                ),
            )
            val job = launch(Dispatchers.IO) {
                slowDist.fetchEnvelope("slow-region")
            }
            delay(80)
            job.cancelAndJoin()
            assertTrue(job.isCancelled)
        } finally {
            server.shutdown()
        }
    }
}
