package com.anpfuel.data.map

import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import java.io.File
import java.security.KeyPairGenerator
import java.security.Signature
import java.util.Base64
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class MapPackageRepositoryTest {
    @TempDir lateinit var root: File
    private val server = MockWebServer()
    private val keyPair = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
    private val packageBytes = "map-package-bytes-for-migration-test".toByteArray()
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

    private fun envelope(
        manifest: String,
        correctSignature: Boolean = true,
        keyId: String = "test-key",
        signer: java.security.PrivateKey = keyPair.private,
    ): String {
        val signature = Signature.getInstance("Ed25519").run {
            initSign(signer)
            update(manifest.toByteArray())
            sign()
        }.let { if (correctSignature) it else it.copyOf().also { bad -> bad[0] = (bad[0].toInt() xor 1).toByte() } }
        fun b64(bytes: ByteArray) = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)
        return """{"format":"abastevo.map-distribution-v1","version":1,"key_id":"$keyId","algorithm":"Ed25519","manifest":"${b64(manifest.toByteArray())}","manifest_sha256":"${sha(manifest.toByteArray())}","signature":"${b64(signature)}"}"""
    }

    private fun store() = FakeStore()
    private fun repo(store: FakeStore, free: Long = Long.MAX_VALUE) = MapPackageRepository(
        store = store,
        filesRoot = root,
        http = OkHttpClient(),
        clock = { 1_000L },
        freeBytes = { free },
        reserveBytes = 10,
    )

    @Test fun `manifest signature and generation are verified before storage`() {
        val store = store()
        val verified = kotlinx.coroutines.runBlocking {
            repo(store).importManifest(envelope(manifest()), mapOf("test-key" to keyPair.public))
        }
        assertEquals("gen-1", verified.manifest.generation)
        assertEquals(1, store.packages.size)
        assertThrows(MapSignatureException::class.java) {
            kotlinx.coroutines.runBlocking { repo(store).importManifest(envelope(manifest("gen-2"), correctSignature = false), mapOf("test-key" to keyPair.public)) }
        }
    }

    @Test fun `trusted key rotation accepts a new key id`() {
        val rotated = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val store = store()
        val verified = kotlinx.coroutines.runBlocking {
            repo(store).importManifest(
                envelope(manifest("gen-rotate"), keyId = "test-key-2", signer = rotated.private),
                mapOf("test-key" to keyPair.public, "test-key-2" to rotated.public),
            )
        }
        assertEquals("gen-rotate", verified.manifest.generation)
        assertEquals("test-key-2", verified.keyId)
    }

    @Test fun `resumable download verifies digest and activates atomically`() {
        val store = store()
        val repository = repo(store)
        kotlinx.coroutines.runBlocking { repository.importManifest(envelope(manifest()), mapOf("test-key" to keyPair.public)) }
        val dir = File(root, "map-packages/gen-1").apply { mkdirs() }
        File(dir, "region.mwm.part").writeBytes(packageBytes.copyOfRange(0, 4))
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val range = request.getHeader("Range")
                return if (range == "bytes=4-") MockResponse().setResponseCode(206)
                    .setHeader("ETag", "\"etag-1\"")
                    .setBody(okio.Buffer().write(packageBytes.copyOfRange(4, packageBytes.size)))
                else MockResponse().setResponseCode(200).setHeader("ETag", "\"etag-1\"").setBody(okio.Buffer().write(packageBytes))
            }
        }
        server.start()
        val base = server.url("/").toString().trimEnd('/')
        assertTrue(kotlinx.coroutines.runBlocking { repository.downloadMissing("gen-1", base) })
        assertTrue(kotlinx.coroutines.runBlocking { repository.activate("gen-1") })
        assertEquals(packageBytes.toList(), File(dir, "region.mwm").readBytes().toList())
        assertEquals(PACKAGE_READY, store.packages.single().status)
        assertEquals("gen-1", store.active?.generation)
    }

    @Test fun `digest mismatch does not activate or delete the previous generation`() {
        val store = store()
        val repository = repo(store)
        kotlinx.coroutines.runBlocking {
            repository.importManifest(envelope(manifest("gen-1")), mapOf("test-key" to keyPair.public))
            store.packages = store.packages.map { it.copy(status = PACKAGE_READY) }
            store.active = MapGenerationEntity("gen-1", "old", true, 1L)
            repository.importManifest(envelope(manifest("gen-2")), mapOf("test-key" to keyPair.public))
        }
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest) = MockResponse().setResponseCode(200).setBody("wrong-bytes")
        }
        server.start()
        assertThrows(MapDownloadException::class.java) {
            kotlinx.coroutines.runBlocking { repository.downloadMissing("gen-2", server.url("/").toString().trimEnd('/')) }
        }
        assertEquals("gen-1", store.active?.generation)
        assertEquals(PACKAGE_READY, store.packages.first { it.generation == "gen-1" }.status)
    }

    @Test fun `disk budget refuses before any network request`() {
        val store = store()
        val repository = repo(store, free = 5)
        kotlinx.coroutines.runBlocking { repository.importManifest(envelope(manifest()), mapOf("test-key" to keyPair.public)) }
        server.start()
        val before = server.requestCount
        assertThrows(MapStorageException::class.java) {
            kotlinx.coroutines.runBlocking { repository.downloadMissing("gen-1", server.url("/").toString().trimEnd('/')) }
        }
        assertEquals(before, server.requestCount)
    }

    @Test fun `active generation cannot be re-imported as pending`() {
        val store = store()
        val repository = repo(store)
        kotlinx.coroutines.runBlocking { repository.importManifest(envelope(manifest()), mapOf("test-key" to keyPair.public)) }
        store.active = MapGenerationEntity("gen-1", "hash", true, 1L)
        assertThrows(MapDownloadException::class.java) {
            kotlinx.coroutines.runBlocking { repository.importManifest(envelope(manifest()), mapOf("test-key" to keyPair.public)) }
        }
        assertEquals("gen-1", store.active?.generation)
    }

    @Test fun `active generation cannot be deleted`() {
        val store = store()
        val repository = repo(store)
        kotlinx.coroutines.runBlocking { repository.importManifest(envelope(manifest()), mapOf("test-key" to keyPair.public)) }
        store.active = MapGenerationEntity("gen-1", "hash", true, 1L)
        assertFalse(kotlinx.coroutines.runBlocking { repository.deleteGeneration("gen-1") })
    }

    private fun sha(bytes: ByteArray) = java.security.MessageDigest.getInstance("SHA-256").digest(bytes)
        .joinToString("") { "%02x".format(it) }
}

private class FakeStore : MapPackageStore {
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
