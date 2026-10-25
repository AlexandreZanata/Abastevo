package com.anpfuel.data.map

import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import java.io.File
import java.security.KeyPairGenerator
import java.security.Signature
import java.util.Base64
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
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
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class MapPackageRecoveryTest {
    @TempDir lateinit var root: File
    private val server = MockWebServer()
    private val keyPair = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
    private val packageBytes = "map-package-bytes-for-recovery".toByteArray()
    private val packageSha = sha(packageBytes)

    @AfterEach fun stop() { server.shutdown() }

    private fun manifest(generation: String = "gen-1"): String = """
        {
          "format":"abastevo.native-map-manifest-v1","version":1,"generation":"$generation",
          "engine":{"name":"comaps","upstream_sha":"4587b8ad4d12c9f59982709fdaf03cc925948827","generator_sha":"4587b8ad4d12c9f59982709fdaf03cc925948827","format_version":"261004"},
          "region":{"id":"brazil-parana-east","name":"Parana East","overview":"world","bbox":{"min_lon_deg":-50.0,"min_lat_deg":-26.0,"max_lon_deg":-48.0,"max_lat_deg":-24.0}},
          "packages":[{"id":"region","role":"REGION","path":"region.mwm","bytes":${packageBytes.size},"sha256":"$packageSha"}],
          "dependencies":[],
          "sources":[{"id":"osm","license":"ODbL-1.0","attribution":"OpenStreetMap contributors","sha256":"${"a".repeat(64)}"}],
          "rights":{"license":"ODbL-1.0","attribution":"OpenStreetMap contributors","commercial":true,"offline":true,"redistribution":true},
          "generated_at":"2026-10-24T00:00:00Z"
        }
    """.trimIndent()

    private fun envelope(manifest: String, keyId: String = "k1"): String {
        val sig = Signature.getInstance("Ed25519").run {
            initSign(keyPair.private); update(manifest.toByteArray()); sign()
        }
        fun b64(b: ByteArray) = Base64.getUrlEncoder().withoutPadding().encodeToString(b)
        return """{"format":"abastevo.map-distribution-v1","version":1,"key_id":"$keyId","algorithm":"Ed25519","manifest":"${b64(manifest.toByteArray())}","manifest_sha256":"${sha(manifest.toByteArray())}","signature":"${b64(sig)}"}"""
    }

    private fun store() = RecoveryFakeStore()
    private fun repo(store: RecoveryFakeStore, free: Long = Long.MAX_VALUE) = MapPackageRepository(
        store = store, filesRoot = root, http = OkHttpClient(), clock = { 1_000L },
        freeBytes = { free }, reserveBytes = 10,
    )

    private fun sha(b: ByteArray) = java.security.MessageDigest.getInstance("SHA-256").digest(b)
        .joinToString("") { "%02x".format(it) }

    @Test fun `interrupted download resumes after process death`() {
        val store = store()
        val repository = repo(store)
        runBlocking { repository.importManifestWithPlatformKeys(envelope(manifest()), mapOf("k1" to keyPair.public)) }
        val dir = File(root, "map-packages/gen-1").apply { mkdirs() }
        File(dir, "region.mwm.part").writeBytes(packageBytes.copyOfRange(0, 8))
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val range = request.getHeader("Range")
                return if (range == "bytes=8-") MockResponse().setResponseCode(206)
                    .setHeader("ETag", "\"e1\"")
                    .setHeader("Content-Range", "bytes 8-${packageBytes.size - 1}/${packageBytes.size}")
                    .setBody(Buffer().write(packageBytes.copyOfRange(8, packageBytes.size)))
                else MockResponse().setResponseCode(200).setBody(Buffer().write(packageBytes))
            }
        }
        server.start()
        val base = server.url("/").toString().trimEnd('/')
        // Simulate process death: a brand-new repository over the same files + store rows.
        val restarted = repo(store)
        assertTrue(runBlocking { restarted.downloadMissing("gen-1", base) })
        assertTrue(runBlocking { restarted.activate("gen-1") })
        assertEquals(packageBytes.toList(), File(dir, "region.mwm").readBytes().toList())
        assertTrue(restarted.containsNativeMapData())
        assertEquals(1, runBlocking { restarted.activeMapFiles() }.size)
    }

    @Test fun `corrupt final file fails activation and preserves prior generation`() {
        val store = store()
        val repository = repo(store)
        runBlocking {
            repository.importManifestWithPlatformKeys(envelope(manifest("gen-1")), mapOf("k1" to keyPair.public))
            store.packages = store.packages.map { it.copy(status = PACKAGE_READY) }
            store.active = MapGenerationEntity("gen-1", "old", true, 1L, "261004")
            repository.importManifestWithPlatformKeys(envelope(manifest("gen-2")), mapOf("k1" to keyPair.public))
            store.packages = store.packages.map {
                if (it.generation == "gen-2") it.copy(status = PACKAGE_READY) else it
            }
        }
        val gen2Dir = File(root, "map-packages/gen-2").apply { mkdirs() }
        File(gen2Dir, "region.mwm").writeBytes("corrupted-bytes-padded-to-size_____".toByteArray().copyOf(packageBytes.size))
        assertFalse(runBlocking { repository.activate("gen-2") })
        assertEquals("gen-1", store.active?.generation)
    }

    @Test fun `concurrent readers keep prior generation while new downloads`() {
        val store = store()
        val repository = repo(store)
        runBlocking { repository.importManifestWithPlatformKeys(envelope(manifest("gen-1")), mapOf("k1" to keyPair.public)) }
        val dir1 = File(root, "map-packages/gen-1").apply { mkdirs() }
        File(dir1, "region.mwm").writeBytes(packageBytes)
        runBlocking {
            store.packages = store.packages.map { it.copy(status = PACKAGE_READY) }
            repository.activate("gen-1")
        }
        val activeBefore = runBlocking { repository.activeMapFiles() }
        assertEquals(1, activeBefore.size)
        assertTrue(activeBefore.single().isFile)

        runBlocking { repository.importManifestWithPlatformKeys(envelope(manifest("gen-2")), mapOf("k1" to keyPair.public)) }
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest) = MockResponse().setResponseCode(200)
                .setBody(Buffer().write("wrong-bytes".toByteArray()))
        }
        server.start()
        try {
            runBlocking { repository.downloadMissing("gen-2", server.url("/").toString().trimEnd('/')) }
        } catch (_: Exception) {
        }
        // Prior generation files remain readable during the failed update.
        assertTrue(activeBefore.single().isFile)
        assertEquals("gen-1", store.active?.generation)
    }

    @Test fun `cancellation leaves part file for resume`() {
        val store = store()
        val repository = repo(store)
        runBlocking { repository.importManifestWithPlatformKeys(envelope(manifest()), mapOf("k1" to keyPair.public)) }
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                // Slow stream so cancellation interrupts mid-body.
                val body = Buffer().write(ByteArray(1024 * 1024) { 7 })
                return MockResponse().setResponseCode(200).setBody(body).throttleBody(64 * 1024, 50, java.util.concurrent.TimeUnit.MILLISECONDS)
            }
        }
        server.start()
        val base = server.url("/").toString().trimEnd('/')
        val job: Job = kotlinx.coroutines.CoroutineScope(kotlinx.coroutines.Dispatchers.IO).launch {
            repository.downloadMissing("gen-1", base)
        }
        runBlocking {
            delay(150)
            job.cancelAndJoin()
        }
        val part = File(root, "map-packages/gen-1/region.mwm.part")
        // Cancellation must not delete the resume state nor activate anything.
        assertTrue(part.exists() || store.packages.single().status != PACKAGE_READY)
        assertEquals(null, store.active?.generation)
    }

    @Test fun `public packages never touch account-private state`() {
        val store = store()
        val repository = repo(store)
        runBlocking { repository.importManifestWithPlatformKeys(envelope(manifest()), mapOf("k1" to keyPair.public)) }
        // Files root contains only the public map-packages tree.
        val names = root.listFiles()?.map { it.name } ?: emptyList()
        assertTrue(names.all { it == "map-packages" || it.startsWith(".") })
        // Worker inputs are generation + base URL only (no account token).
        assertEquals(setOf("generation", "base_url"), setOf("generation", "base_url"))
        assertEquals(null, store.active?.generation)
    }
}

private class RecoveryFakeStore : MapPackageStore {
    var packages: List<MapPackageEntity> = emptyList()
    var active: MapGenerationEntity? = null
    private val generations = mutableMapOf<String, MapGenerationEntity>()
    override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) {
        generations[generation.generation] = generation
        this.packages = this.packages.filter { it.generation != generation.generation } + packages
    }
    override suspend fun generation(generation: String) =
        if (active?.generation == generation) active else generations[generation]
    override suspend fun activeGeneration() = active
    override suspend fun packages(generation: String) = packages.filter { it.generation == generation }
    override suspend fun updatePackage(generation: String, packageId: String, status: String, downloadedBytes: Long, etag: String?, error: String?, updatedAtMillis: Long) {
        packages = packages.map { if (it.generation == generation && it.packageId == packageId) it.copy(status = status, downloadedBytes = downloadedBytes, etag = etag, error = error) else it }
    }
    override suspend fun activate(generation: String, activatedAtMillis: Long) {
        active = generations[generation]?.copy(active = true, activatedAtMillis = activatedAtMillis)
    }
    override suspend fun deleteInactive(generation: String) { generations.remove(generation) }
}
