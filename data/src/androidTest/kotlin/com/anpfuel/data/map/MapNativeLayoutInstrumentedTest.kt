package com.anpfuel.data.map

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.AnpFuelDatabase
import java.io.File
import java.security.KeyPairGenerator
import java.security.Signature
import java.util.Base64
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okio.Buffer
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Native-layout proof on device: activation publishes verified bytes to the
 * `<filesDir>/<numericVersion>/` layout the pinned engine scans; a restart
 * keeps Room + filesystem coherent; an open reader survives the atomic swap.
 * The engine itself is proven against this exact layout by MAP-14 (probe
 * seeded versioned `.mwm` files under filesDir, render/search/route +
 * restarts); here the app side proves it writes that layout.
 */
@RunWith(AndroidJUnit4::class)
class MapNativeLayoutInstrumentedTest {
    private lateinit var context: Context
    private lateinit var server: MockWebServer
    private lateinit var dbFile: File
    private lateinit var stagingRoot: File
    private lateinit var nativeRoot: File
    private var db: AnpFuelDatabase? = null

    private val keyPair = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()

    @Before fun setup() {
        context = ApplicationProvider.getApplicationContext()
        server = MockWebServer()
        val base = File(context.cacheDir, "map-layout-${System.nanoTime()}").apply { mkdirs() }
        stagingRoot = File(base, "staging").apply { mkdirs() }
        nativeRoot = File(base, "native").apply { mkdirs() }
        dbFile = File(base, "maps.db")
    }

    @After fun tearDown() {
        try {
            server.shutdown()
        } catch (_: Exception) {
        }
        try {
            db?.close()
        } catch (_: Exception) {
        }
        dbFile.parentFile?.deleteRecursively()
    }

    private fun repo(): MapPackageRepository {
        db?.close()
        db = Room.databaseBuilder(context, AnpFuelDatabase::class.java, dbFile.absolutePath).build()
        return MapPackageRepository(
            store = RoomMapPackageStore(db!!.mapPackageDao()),
            filesRoot = stagingRoot,
            http = OkHttpClient(),
            nativeRoot = nativeRoot,
        )
    }

    private fun sha(b: ByteArray) = java.security.MessageDigest.getInstance("SHA-256").digest(b)
        .joinToString("") { "%02x".format(it) }

    private fun manifest(generation: String, path: String, bytes: ByteArray): String = """
        {
          "format":"abastevo.native-map-manifest-v1","version":1,"generation":"$generation",
          "engine":{"name":"comaps","upstream_sha":"4587b8ad4d12c9f59982709fdaf03cc925948827","generator_sha":"4587b8ad4d12c9f59982709fdaf03cc925948827","format_version":"2601"},
          "region":{"id":"test-rect","name":"Rect","bbox":{"min_lon_deg":-1.0,"min_lat_deg":-1.0,"max_lon_deg":1.0,"max_lat_deg":1.0},"overview":"world"},
          "packages":[{"id":"rect","role":"REGION","path":"$path","bytes":${bytes.size},"sha256":"${sha(bytes)}"}],
          "dependencies":[],
          "sources":[{"id":"synthetic","license":"CC-BY-4.0","attribution":"Synthetic on-device layout proof","sha256":"${"0".repeat(64)}"}],
          "rights":{"license":"CC-BY-4.0","attribution":"Synthetic on-device layout proof","commercial":true,"offline":true,"redistribution":true},
          "generated_at":"2026-10-25T12:00:00Z"
        }
    """.trimIndent()

    private fun envelope(manifest: String, keyId: String = "dev-key"): String {
        val sig = Signature.getInstance("Ed25519").run {
            initSign(keyPair.private); update(manifest.toByteArray()); sign()
        }
        fun b64(b: ByteArray) = Base64.getUrlEncoder().withoutPadding().encodeToString(b)
        return """{"format":"abastevo.map-distribution-v1","version":1,"key_id":"$keyId","algorithm":"Ed25519","manifest":"${b64(manifest.toByteArray())}","manifest_sha256":"${sha(manifest.toByteArray())}","signature":"${b64(sig)}"}"""
    }

    private fun rawPub(): ByteArray = MonocypherEd25519.rawPublicKey(keyPair.public)!!

    private fun serve(vararg parts: Pair<String, ByteArray>) {
        val byPath = parts.toMap()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val id = request.path!!.substringAfterLast("/").substringBefore("?")
                val bytes = byPath[id] ?: return MockResponse().setResponseCode(404)
                return MockResponse().setResponseCode(200).setBody(Buffer().write(bytes))
            }
        }
        try {
            server.start()
        } catch (_: IllegalArgumentException) {
        }
    }

    @Test fun publishRestartAndOpenReaderSwap() = runBlocking {
        val bytesA = ByteArray(2048) { (it % 251).toByte() }
        val repository = repo()
        repository.importManifest(envelope(manifest("dev-1", "2601/dev-rect.mwm", bytesA)), mapOf("dev-key" to rawPub()))
        serve("rect" to bytesA)
        val base = server.url("/").toString().trimEnd('/')
        assertTrue(repository.downloadMissing("dev-1", base))
        assertTrue(repository.activate("dev-1"))
        assertArrayEquals(bytesA, File(nativeRoot, "2601/dev-rect.mwm").readBytes())
        assertTrue(repository.containsNativeMapData())
        assertEquals(true, repository.publishedState()?.complete)

        // Restart: new repository + reopened database over the same files.
        val restarted = repo()
        assertEquals("dev-1", restarted.activeMapFiles().let {
            assertEquals(1, it.size)
            restarted.publishedState()?.generation
        })
        assertArrayEquals(bytesA, File(nativeRoot, "2601/dev-rect.mwm").readBytes())

        // Same-path swap while a reader holds the old file open.
        val bytesB = ByteArray(2048) { ((it * 7 + 3) % 251).toByte() }
        restarted.importManifest(envelope(manifest("dev-2", "2601/dev-rect.mwm", bytesB)), mapOf("dev-key" to rawPub()))
        serve("rect" to bytesB)
        assertTrue(restarted.downloadMissing("dev-2", base))
        File(nativeRoot, "2601/dev-rect.mwm").inputStream().use { reader ->
            assertTrue(restarted.activate("dev-2"))
            assertArrayEquals(bytesA, reader.readBytes())
        }
        assertArrayEquals(bytesB, File(nativeRoot, "2601/dev-rect.mwm").readBytes())
        assertEquals("dev-2", restarted.publishedState()?.generation)
        assertEquals(true, restarted.publishedState()?.complete)
        assertFalse(File(nativeRoot, "2601").listFiles()!!.any { it.extension == "part" })
    }
}
