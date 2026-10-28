package com.anpfuel.data.map

import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import java.io.File
import java.security.KeyPair
import java.security.KeyPairGenerator
import java.security.Signature
import java.util.Base64
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okio.Buffer
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/**
 * Native-layout publication, retention and transfer hardening proved against
 * the Go-signed committed fixture (`contracts/testdata/map/dist-kotlin-v1`,
 * real Go crypto/ed25519 output). Locally-signed manifests cover only
 * multi-generation retention/swap cases; they are never presented as Go data.
 */
class MapPublishTest {
    @TempDir lateinit var root: File
    private val server = MockWebServer()

    @AfterEach fun stop() { server.shutdown() }

    private fun fixtureFile(name: String): File {
        var dir = File(System.getProperty("user.dir"))
        repeat(8) {
            val candidate = File(dir, "contracts/testdata/map/dist-kotlin-v1/$name")
            if (candidate.isFile) return candidate
            dir = dir.parentFile ?: return@repeat
        }
        throw IllegalStateException("committed Go fixture not found")
    }

    private fun goEnvelope(): String =
        File(fixtureDir(), "distribution.json").readText()

    private fun fixtureDir(): File {
        var dir = File(System.getProperty("user.dir"))
        repeat(8) {
            if (File(dir, "contracts/testdata/map/dist-kotlin-v1/distribution.json").isFile) {
                return File(dir, "contracts/testdata/map/dist-kotlin-v1")
            }
            dir = dir.parentFile ?: return@repeat
        }
        throw IllegalStateException("committed Go fixture not found")
    }

    private fun goPubKey(): ByteArray {
        val raw = File(fixtureDir(), "test-key.pub").readText().trim()
        return Base64.getUrlDecoder().decode(raw + "=".repeat((4 - raw.length % 4) % 4))
    }

    private fun goPackageBytes(path: String): ByteArray =
        File(fixtureDir(), path).readBytes()

    private fun sha(b: ByteArray) = java.security.MessageDigest.getInstance("SHA-256").digest(b)
        .joinToString("") { "%02x".format(it) }

    private val localKeys = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()

    private fun rawPub(kp: KeyPair = localKeys): ByteArray =
        kp.public.encoded.let { it.copyOfRange(it.size - 32, it.size) }

    private fun manifestB(
        generation: String,
        pkgId: String,
        path: String,
        bytes: ByteArray,
        formatVersion: String = "2601",
    ): String = """
        {
          "format":"abastevo.native-map-manifest-v1","version":1,"generation":"$generation",
          "engine":{"name":"comaps","upstream_sha":"4587b8ad4d12c9f59982709fdaf03cc925948827","generator_sha":"4587b8ad4d12c9f59982709fdaf03cc925948827","format_version":"$formatVersion"},
          "region":{"id":"test-rect","name":"Synthetic rect","bbox":{"min_lon_deg":-1.0,"min_lat_deg":-1.0,"max_lon_deg":1.0,"max_lat_deg":1.0},"overview":"world"},
          "packages":[{"id":"$pkgId","role":"REGION","path":"$path","bytes":${bytes.size},"sha256":"${sha(bytes)}"}],
          "dependencies":[],
          "sources":[{"id":"synthetic","license":"CC-BY-4.0","attribution":"Synthetic test content","sha256":"${"0".repeat(64)}"}],
          "rights":{"license":"CC-BY-4.0","attribution":"Synthetic test content","commercial":true,"offline":true,"redistribution":true},
          "generated_at":"2026-10-25T12:00:00Z"
        }
    """.trimIndent()

    private fun envelopeFor(manifest: String, keyId: String = "k2", kp: KeyPair = localKeys): String {
        val sig = Signature.getInstance("Ed25519").run {
            initSign(kp.private); update(manifest.toByteArray()); sign()
        }
        fun b64(b: ByteArray) = Base64.getUrlEncoder().withoutPadding().encodeToString(b)
        return """{"format":"abastevo.map-distribution-v1","version":1,"key_id":"$keyId","algorithm":"Ed25519","manifest":"${b64(manifest.toByteArray())}","manifest_sha256":"${sha(manifest.toByteArray())}","signature":"${b64(sig)}"}"""
    }

    private fun store() = PublishFakeStore()
    private fun repo(store: PublishFakeStore, free: Long = Long.MAX_VALUE) = MapPackageRepository(
        store = store, filesRoot = File(root, "staging"), http = OkHttpClient(),
        clock = { 1_000L }, freeBytes = { free }, reserveBytes = 10, nativeRoot = File(root, "native"),
    )

    private var serving = false
    private val requestPaths = mutableListOf<String>()

    private fun serveBytes(vararg parts: Pair<String, ByteArray>, code: Int = 200, contentRange: String? = null) {
        val byPath = parts.toMap()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                requestPaths += request.path!!
                val id = request.path!!.substringAfterLast("/").substringBefore("?")
                val bytes = byPath[id] ?: return MockResponse().setResponseCode(404)
                val resp = MockResponse().setResponseCode(code).setHeader("ETag", "\"e1\"")
                if (contentRange != null) resp.setHeader("Content-Range", contentRange)
                return resp.setBody(Buffer().write(bytes))
            }
        }
        if (!serving) {
            server.start()
            serving = true
        }
    }

    private fun baseUrl() = server.url("/").toString().trimEnd('/')

    @Test fun `go-signed fixture verifies and publishes to the native layout`() {
        val store = store()
        val repository = repo(store)
        val verified = runBlocking { repository.importManifest(goEnvelope(), mapOf("test-key" to goPubKey())) }
        assertEquals("test-20261025.1", verified.manifest.generation)
        serveBytes("test-rect" to goPackageBytes("2601/test-rect.mwm"), "world" to goPackageBytes("2601/World.mwm"))
        assertTrue(runBlocking { repository.downloadMissing("test-20261025.1", baseUrl()) })
        assertTrue(
            requestPaths.any { it.startsWith("/dados/mapa-dist/test-20261025.1/") && it.endsWith("test-rect") },
            "package download must use the static generation path: $requestPaths",
        )
        assertTrue(runBlocking { repository.activate("test-20261025.1") })
        val versionDir = File(root, "native/2601")
        assertArrayEquals(goPackageBytes("2601/test-rect.mwm"), File(versionDir, "test-rect.mwm").readBytes())
        assertArrayEquals(goPackageBytes("2601/World.mwm"), File(versionDir, "World.mwm").readBytes())
        val state = runBlocking { repository.publishedState() }
        assertEquals(true, state?.complete)
        assertEquals(2, runBlocking { repository.activeMapFiles() }.size)
        assertTrue(repository.containsNativeMapData())
    }

    @Test fun `go fixture rejects tampered manifest wrong key and bad sizes`() {
        val store = store()
        val repository = repo(store)
        val raw = goEnvelope()
        val root = org.json.JSONObject(raw)
        // Unknown key id fails closed.
        assertThrows(MapSignatureException::class.java) {
            runBlocking { repository.importManifest(raw, mapOf("other" to goPubKey())) }
        }
        // Wrong key bytes fail as invalid signature.
        val wrong = ByteArray(32) { (it + 1).toByte() }
        assertThrows(MapSignatureException::class.java) {
            runBlocking { repository.importManifest(raw, mapOf("test-key" to wrong)) }
        }
        // Truncated signature fails closed.
        val tampered = org.json.JSONObject(raw).apply {
            put("signature", getString("signature").dropLast(4))
        }.toString()
        assertThrows(Exception::class.java) {
            runBlocking { repository.importManifest(tampered, mapOf("test-key" to goPubKey())) }
        }
        assertTrue(root.getString("manifest_sha256").length == 64)
    }

    @Test fun `failed B preserves published A`() {
        val store = store()
        val repository = repo(store)
        runBlocking { repository.importManifest(goEnvelope(), mapOf("test-key" to goPubKey())) }
        serveBytes("test-rect" to goPackageBytes("2601/test-rect.mwm"), "world" to goPackageBytes("2601/World.mwm"))
        assertTrue(runBlocking { repository.downloadMissing("test-20261025.1", baseUrl()) })
        assertTrue(runBlocking { repository.activate("test-20261025.1") })

        val bBytes = "b-package-bytes".toByteArray()
        runBlocking { repository.importManifest(envelopeFor(manifestB("gen-b", "b-rect", "2601/b-rect.mwm", bBytes)), mapOf("k2" to rawPub())) }
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest) =
                MockResponse().setResponseCode(200).setBody(Buffer().write("corrupted!!".toByteArray()))
        }
        assertThrows(MapDownloadException::class.java) {
            runBlocking { repository.downloadMissing("gen-b", baseUrl()) }
        }
        assertFalse(runBlocking { repository.activate("gen-b") })
        assertEquals("test-20261025.1", store.active?.generation)
        val state = runBlocking { repository.publishedState() }!!
        assertTrue(state.complete)
        assertFalse(File(root, "native/2601/b-rect.mwm").exists())
        assertArrayEquals(goPackageBytes("2601/test-rect.mwm"), File(root, "native/2601/test-rect.mwm").readBytes())
    }

    @Test fun `concurrent activations converge on one coherent generation`() {
        val store = store()
        val repository = repo(store)
        runBlocking { repository.importManifest(goEnvelope(), mapOf("test-key" to goPubKey())) }
        val bBytes = "b-coherent-bytes-0123456789".toByteArray()
        runBlocking { repository.importManifest(envelopeFor(manifestB("gen-b", "b-rect", "2601/b-rect.mwm", bBytes)), mapOf("k2" to rawPub())) }
        serveBytes(
            "test-rect" to goPackageBytes("2601/test-rect.mwm"),
            "world" to goPackageBytes("2601/World.mwm"),
            "b-rect" to bBytes,
        )
        assertTrue(runBlocking { repository.downloadMissing("test-20261025.1", baseUrl()) })
        assertTrue(runBlocking { repository.downloadMissing("gen-b", baseUrl()) })
        runBlocking {
            val a = async { repository.activate("test-20261025.1") }
            val b = async { repository.activate("gen-b") }
            assertTrue(a.await() || b.await())
        }
        val finalActive = store.active!!.generation
        val state = runBlocking { repository.publishedState() }!!
        assertTrue(state.complete)
        assertEquals(finalActive, state.generation)
        // Version dir matches the winner exactly: no partials, no loser files.
        val names = File(root, "native/2601").listFiles()!!.map { it.name }.toSet()
        if (finalActive == "gen-b") {
            assertEquals(setOf("b-rect.mwm"), names)
            assertArrayEquals(bBytes, File(root, "native/2601/b-rect.mwm").readBytes())
        } else {
            assertEquals(setOf("test-rect.mwm", "World.mwm"), names)
        }
        assertTrue(File(root, "native/2601").listFiles()!!.none { it.extension == "part" })
    }

    @Test fun `switching region leaves no stale country file under the native version`() {
        // CoMaps registers <countryId>.mwm by file name: a leftover file of the
        // previous region would still load as that country after a switch.
        val store = store()
        val repository = repo(store)
        val mt = "mato-grosso-bytes".toByteArray()
        val pr = "parana-east-bytes".toByteArray()
        runBlocking {
            repository.importManifest(
                envelopeFor(manifestB("gen-mt", "brazil-mato-grosso", "261004/Brazil_Mato Grosso.mwm", mt, "261004")),
                mapOf("k2" to rawPub()),
            )
        }
        serveBytes("brazil-mato-grosso" to mt, "brazil-parana-east" to pr)
        assertTrue(runBlocking { repository.downloadMissing("gen-mt", baseUrl()) })
        assertTrue(runBlocking { repository.activate("gen-mt") })
        val versionDir = File(root, "native/261004")
        assertArrayEquals(mt, File(versionDir, "Brazil_Mato Grosso.mwm").readBytes())

        runBlocking {
            repository.importManifest(
                envelopeFor(manifestB("gen-pr", "brazil-parana-east", "261004/Brazil_Parana_East.mwm", pr, "261004")),
                mapOf("k2" to rawPub()),
            )
        }
        assertTrue(runBlocking { repository.downloadMissing("gen-pr", baseUrl()) })
        assertTrue(runBlocking { repository.activate("gen-pr") })
        assertEquals(setOf("Brazil_Parana_East.mwm"), versionDir.listFiles()!!.map { it.name }.toSet())
        assertEquals(listOf(File(versionDir, "Brazil_Parana_East.mwm")), runBlocking { repository.activeMapFiles() })
        assertFalse(File(root, "staging/map-packages/gen-mt").exists(), "previous region staging is recycled")
    }

    @Test fun `open readers keep old bytes across a same-path swap`() {
        val store = store()
        val repository = repo(store)
        runBlocking { repository.importManifest(goEnvelope(), mapOf("test-key" to goPubKey())) }
        serveBytes("test-rect" to goPackageBytes("2601/test-rect.mwm"), "world" to goPackageBytes("2601/World.mwm"))
        assertTrue(runBlocking { repository.downloadMissing("test-20261025.1", baseUrl()) })
        assertTrue(runBlocking { repository.activate("test-20261025.1") })
        val published = File(root, "native/2601/test-rect.mwm")
        published.inputStream().use { reader ->
            val newBytes = goPackageBytes("2601/test-rect.mwm").copyOf().also { it[0] = (it[0].toInt() xor 0xFF).toByte() }
            // Same path, new generation, different bytes.
            runBlocking { repository.importManifest(envelopeFor(manifestB("gen-b2", "test-rect", "2601/test-rect.mwm", newBytes)), mapOf("k2" to rawPub())) }
            serveBytes("test-rect" to newBytes)
            assertTrue(runBlocking { repository.downloadMissing("gen-b2", baseUrl()) })
            assertTrue(runBlocking { repository.activate("gen-b2") })
            val oldRead = reader.readBytes()
            assertArrayEquals(goPackageBytes("2601/test-rect.mwm"), oldRead)
        }
        // Stream closed; published file now carries the new bytes.
        assertArrayEquals(
            goPackageBytes("2601/test-rect.mwm").copyOf().also { it[0] = (it[0].toInt() xor 0xFF).toByte() },
            File(root, "native/2601/test-rect.mwm").readBytes(),
        )
    }

    @Test fun `partial publish heals on re-activation`() {
        val store = store()
        val repository = repo(store)
        runBlocking { repository.importManifest(goEnvelope(), mapOf("test-key" to goPubKey())) }
        serveBytes("test-rect" to goPackageBytes("2601/test-rect.mwm"), "world" to goPackageBytes("2601/World.mwm"))
        assertTrue(runBlocking { repository.downloadMissing("test-20261025.1", baseUrl()) })
        // Simulate a crash between publish and flip: one file published, one
        // publish-part left behind, Room still inactive.
        val versionDir = File(root, "native/2601").apply { mkdirs() }
        File(versionDir, "test-rect.mwm").writeBytes(goPackageBytes("2601/test-rect.mwm"))
        File(versionDir, "World.mwm.publish-part").writeBytes(ByteArray(10))
        assertTrue(runBlocking { repository.activate("test-20261025.1") })
        assertArrayEquals(goPackageBytes("2601/World.mwm"), File(versionDir, "World.mwm").readBytes())
        assertFalse(File(versionDir, "World.mwm.publish-part").exists())
        assertEquals(true, runBlocking { repository.publishedState() }?.complete)
    }

    @Test fun `oversize body fails closed without activation`() {
        val store = store()
        val repository = repo(store)
        val small = "tiny".toByteArray()
        runBlocking { repository.importManifest(envelopeFor(manifestB("gen-o", "o-rect", "o.mwm", small)), mapOf("k2" to rawPub())) }
        val big = ByteArray(small.size + 16) { 7 }
        serveBytes("o-rect" to big)
        val err = assertThrows(MapDownloadException::class.java) {
            runBlocking { repository.downloadMissing("gen-o", baseUrl()) }
        }
        assertEquals("package-too-large", err.message)
        assertFalse(File(root, "staging/map-packages/gen-o/o.mwm").exists())
        assertFalse(File(root, "staging/map-packages/gen-o/o.mwm.part").exists())
        assertEquals(null, store.active?.generation)
    }

    @Test fun `range mismatch fails closed then full resend recovers`() {
        val store = store()
        val repository = repo(store)
        val bytes = "range-recovery-bytes-0123".toByteArray()
        runBlocking { repository.importManifest(envelopeFor(manifestB("gen-r", "r-rect", "r.mwm", bytes)), mapOf("k2" to rawPub())) }
        val dir = File(root, "staging/map-packages/gen-r").apply { mkdirs() }
        File(dir, "r.mwm.part").writeBytes(bytes.copyOfRange(0, 4))
        var calls = 0
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                calls++
                return if (calls == 1) {
                    // Protocol violation: 206 without Content-Range.
                    MockResponse().setResponseCode(206).setBody(Buffer().write(bytes.copyOfRange(4, bytes.size)))
                } else {
                    MockResponse().setResponseCode(200).setBody(Buffer().write(bytes))
                }
            }
        }
        server.start()
        val err = assertThrows(MapDownloadException::class.java) {
            runBlocking { repository.downloadMissing("gen-r", baseUrl()) }
        }
        assertEquals("package-range-mismatch", err.message)
        assertFalse(File(dir, "r.mwm.part").exists())
        assertTrue(runBlocking { repository.downloadMissing("gen-r", baseUrl()) })
    }

    @Test fun `wrong content-range start fails closed`() {
        val store = store()
        val repository = repo(store)
        val bytes = "range-start-bytes-012345".toByteArray()
        runBlocking { repository.importManifest(envelopeFor(manifestB("gen-w", "w-rect", "w.mwm", bytes)), mapOf("k2" to rawPub())) }
        val dir = File(root, "staging/map-packages/gen-w").apply { mkdirs() }
        File(dir, "w.mwm.part").writeBytes(bytes.copyOfRange(0, 4))
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse =
                MockResponse().setResponseCode(206)
                    .setHeader("Content-Range", "bytes 0-${bytes.size - 1}/${bytes.size}")
                    .setBody(Buffer().write(bytes))
        }
        server.start()
        assertThrows(MapDownloadException::class.java) {
            runBlocking { repository.downloadMissing("gen-w", baseUrl()) }
        }
    }

    @Test fun `server ignoring range rechecks space for the full file`() {
        val store = store()
        // Pre-check passes for the delta but the full resend does not fit.
        val bytes = "0123456789abcdef".toByteArray() // 16 bytes
        val part = bytes.copyOfRange(0, 4)
        val free = bytes.size + 9L // pre: free-(16-4)=13>=10 ok; full: free-16=9<10 fail
        val repository = repo(store, free = free)
        runBlocking { repository.importManifest(envelopeFor(manifestB("gen-f", "f-rect", "f.mwm", bytes)), mapOf("k2" to rawPub())) }
        val dir = File(root, "staging/map-packages/gen-f").apply { mkdirs() }
        File(dir, "f.mwm.part").writeBytes(part)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse =
                MockResponse().setResponseCode(200).setBody(Buffer().write(bytes))
        }
        server.start()
        assertThrows(MapStorageException::class.java) {
            runBlocking { repository.downloadMissing("gen-f", baseUrl()) }
        }
    }

    @Test fun `unsupported data version fails closed without touching active`() {
        val store = store()
        val repository = repo(store)
        val bytes = "versioned".toByteArray()
        runBlocking { repository.importManifest(envelopeFor(manifestB("gen-x", "x-rect", "x.mwm", bytes, formatVersion = "xx")), mapOf("k2" to rawPub())) }
        serveBytes("x-rect" to bytes)
        assertTrue(runBlocking { repository.downloadMissing("gen-x", baseUrl()) })
        val err = assertThrows(MapDownloadException::class.java) {
            runBlocking { repository.activate("gen-x") }
        }
        assertEquals("unsupported-data-version", err.message)
        assertEquals(null, store.active?.generation)
        assertFalse(File(root, "native").exists())
    }

    @Test fun `stale staging of terminal generations is recycled, in-progress kept`() {
        val store = store()
        val repository = repo(store)
        runBlocking { repository.importManifest(goEnvelope(), mapOf("test-key" to goPubKey())) }
        val bBytes = "b-retention-0123456789ab".toByteArray()
        runBlocking { repository.importManifest(envelopeFor(manifestB("gen-b", "b-rect", "2601/b-rect.mwm", bBytes)), mapOf("k2" to rawPub())) }
        // B1: imported only (PENDING) with a partial file — must survive
        // someone else's activation so resume stays possible.
        runBlocking { repository.importManifest(envelopeFor(manifestB("gen-pending", "p-rect", "p.mwm", bBytes)), mapOf("k2" to rawPub())) }
        File(root, "staging/map-packages/gen-pending").apply { mkdirs() }
        File(root, "staging/map-packages/gen-pending/p.mwm.part").writeBytes(ByteArray(4))
        // Orphan dir with no Room rows — must be recycled.
        File(root, "staging/map-packages/orphan").apply { mkdirs() }
        File(root, "staging/map-packages/orphan/junk.bin").writeBytes(ByteArray(4))
        // FAILED terminal staging — recycled after publication.
        runBlocking { repository.importManifest(envelopeFor(manifestB("gen-failed", "f-rect", "f.mwm", bBytes)), mapOf("k2" to rawPub())) }
        serveBytes(
            "test-rect" to goPackageBytes("2601/test-rect.mwm"),
            "world" to goPackageBytes("2601/World.mwm"),
            "b-rect" to bBytes,
        )
        assertTrue(runBlocking { repository.downloadMissing("test-20261025.1", baseUrl()) })
        assertTrue(runBlocking { repository.downloadMissing("gen-b", baseUrl()) })
        val failedDir = File(root, "staging/map-packages/gen-failed").apply { mkdirs() }
        File(failedDir, "f.mwm.part").writeBytes(ByteArray(4))
        runBlocking { store.updatePackage("gen-failed", "f-rect", PACKAGE_FAILED, 4, null, "digest-mismatch", 1L) }
        assertTrue(runBlocking { repository.activate("gen-b") })
        assertTrue(File(root, "staging/map-packages/gen-pending/p.mwm.part").isFile)
        assertFalse(File(root, "staging/map-packages/orphan").exists())
        assertFalse(File(root, "staging/map-packages/gen-failed").exists())
        assertFalse(File(root, "staging/map-packages/test-20261025.1").exists())
    }

    @Test fun `legacy row without data version repairs on same-generation reimport`() {
        val store = store()
        val repository = repo(store)
        val bytes = ByteArray(128) { (it * 3 + 1).toByte() }
        val manifest = manifestB("legacy", "l-rect", "2601/l-rect.mwm", bytes)
        // Simulate a pre-layout active row (no data_version), then re-import
        // the same generation: repair replaces rows, keeps active, publishes.
        runBlocking {
            store.insertGeneration(
                MapGenerationEntity("legacy", "old-hash", true, 1L, ""),
                listOf(
                    MapPackageEntity("l-rect", "legacy", "REGION", "l.mwm", 1, "b".repeat(64), null, PACKAGE_FAILED, 0, "stale", 1L),
                ),
            )
        }
        val verified = runBlocking { repository.importManifest(envelopeFor(manifest), mapOf("k2" to rawPub())) }
        assertEquals("legacy", verified.manifest.generation)
        assertEquals(true, runBlocking { store.generation("legacy") }?.active)
        assertEquals("2601", runBlocking { store.generation("legacy") }?.dataVersion)
        serveBytes("l-rect" to bytes)
        assertTrue(runBlocking { repository.downloadMissing("legacy", baseUrl()) })
        assertTrue(runBlocking { repository.activate("legacy") })
        assertEquals(true, runBlocking { repository.publishedState() }?.complete)
        assertArrayEquals(bytes, File(root, "native/2601/l-rect.mwm").readBytes())
    }

    @Test fun `checkResumeRange accepts fresh and exact resumes, refuses the rest`() {        checkResumeRange(null, 0, 10)
        checkResumeRange("bytes 4-9/10", 4, 10)
        assertThrows(MapDownloadException::class.java) { checkResumeRange(null, 4, 10) }
        assertThrows(MapDownloadException::class.java) { checkResumeRange("bytes 0-9/10", 4, 10) }
        assertThrows(MapDownloadException::class.java) { checkResumeRange("bytes 4-9/11", 4, 10) }
        assertThrows(MapDownloadException::class.java) { checkResumeRange("garbage", 4, 10) }
    }
}

private class PublishFakeStore : MapPackageStore {
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
        val gen = generations[generation] ?: return
        generations.values.forEach { generations[it.generation] = it.copy(active = false) }
        val updated = gen.copy(active = true, activatedAtMillis = activatedAtMillis)
        generations[generation] = updated
        active = updated
    }
    override suspend fun deleteInactive(generation: String) { generations.remove(generation) }
}
