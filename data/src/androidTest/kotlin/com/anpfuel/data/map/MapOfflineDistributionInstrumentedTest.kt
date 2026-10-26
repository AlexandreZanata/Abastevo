package com.anpfuel.data.map

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.AnpFuelDatabase
import java.io.File
import java.security.MessageDigest
import kotlinx.coroutines.runBlocking
import java.util.concurrent.TimeUnit
import okhttp3.ConnectionPool
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * MAP-24 offline distribution on the owned emulator: the production path
 * (MapDistribution.fetchEnvelope → importManifest → downloadMissing → activate)
 * against a local authorized proof server, then restart recovery, traffic
 * observability with no new endpoint contact, corruption refusal, lease-
 * respecting deletion and disk-full fences. Synthetic transfer bytes keep the
 * suite fast; byte-exactness (not map parsing) is the data-layer proof. Real
 * MWM consumption by the pinned engine is proven by the app offline journey.
 */
@RunWith(AndroidJUnit4::class)
class MapOfflineDistributionInstrumentedTest {
    private lateinit var context: Context
    private lateinit var origin: String
    private lateinit var base: File
    private lateinit var stagingRoot: File
    private lateinit var nativeRoot: File
    private lateinit var dbFile: File
    private var db: AnpFuelDatabase? = null
    // Retries off so traffic observability stays exact: a dropped connection
    // fails closed instead of silently doubling a request. No idle pooling, so a
    // socket the proof server already closed can never be reused (that race made
    // the second request of a test intermittently fail with an empty response).
    private val http = OkHttpClient.Builder()
        .retryOnConnectionFailure(false)
        .connectionPool(ConnectionPool(0, 1, TimeUnit.NANOSECONDS))
        .build()
    private lateinit var trusted: Map<String, ByteArray>

    @Before fun setup() {
        context = ApplicationProvider.getApplicationContext()
        // Default is the adb-reverse loopback; the emulator's built-in NAT alias
        // (-e mapProofOrigin http://10.0.2.2:18766) avoids the reverse tunnel,
        // which can accept a connection and then drop it before any byte.
        origin = InstrumentationRegistry.getArguments().getString("mapProofOrigin")
            ?: "http://127.0.0.1:18766"
        base = File(context.cacheDir, "map-offline-${System.nanoTime()}").apply { mkdirs() }
        stagingRoot = File(base, "staging").apply { mkdirs() }
        nativeRoot = File(base, "native").apply { mkdirs() }
        dbFile = File(base, "maps.db")
        trusted = mapOf("map24-proof" to rawPub(fetchBytes("/proof/pubkey")))
    }

    @After fun tearDown() {
        try {
            db?.close()
        } catch (_: Exception) {
        }
        base.deleteRecursively()
    }

    private fun fetchBytes(path: String): ByteArray {
        val request = Request.Builder().url(origin.trimEnd('/') + path).build()
        http.newCall(request).execute().use { response ->
            if (!response.isSuccessful) error("proof server ${response.code}")
            return response.body!!.bytes()
        }
    }

    private fun unpad(text: String): String = text + "=".repeat((4 - text.length % 4) % 4)

    private fun rawPub(body: ByteArray): ByteArray {
        val text = body.decodeToString().trim()
        return java.util.Base64.getUrlDecoder().decode(unpad(text))
    }

    /**
     * Production-path requests only (manifest and package fetches). The proof
     * server logs its own control queries too, including this very read; those
     * are excluded so counts are exact instead of drifting by one per query.
     */
    private fun proofRequestCount(): Int {
        val requests = JSONObject(fetchBytes("/proof/requests").decodeToString()).getJSONArray("requests")
        return (0 until requests.length()).count { requests.getString(it).startsWith("/v1/") }
    }

    private fun generationOf(envelopeJson: String): String {
        val manifestB64 = JSONObject(envelopeJson).getString("manifest")
        val manifest = java.util.Base64.getUrlDecoder().decode(unpad(manifestB64)).decodeToString()
        return JSONObject(manifest).getString("generation")
    }

    private fun repo(freeBytes: Long = Long.MAX_VALUE): MapPackageRepository {
        db?.close()
        db = Room.databaseBuilder(context, AnpFuelDatabase::class.java, dbFile.absolutePath).build()
        return MapPackageRepository(
            store = RoomMapPackageStore(db!!.mapPackageDao()),
            filesRoot = stagingRoot,
            http = http,
            freeBytes = { freeBytes },
            nativeRoot = nativeRoot,
        )
    }

    private fun distribution(): MapDistribution {
        val config = MapDistributionConfig(
            baseUrl = origin,
            region = "test-rect",
            trustedKeys = trusted,
            keysInvalid = false,
        )
        return MapDistribution(http, config)
    }

    private fun expectedBytes(id: String): ByteArray {
        // Must match the proof server's deterministic pattern.
        val size = when (id) {
            "east" -> 262144
            "world" -> 131072
            else -> 65536
        }
        val seed = when (id) {
            "east" -> 1
            "world" -> 2
            else -> 3
        }
        return ByteArray(size) { ((it * 31 + seed) % 251).toByte() }
    }

    @Test fun offlineDistributionActivatesSurvivesRestartWithNoNewContact() = runBlocking {
        val repository = repo()
        val before = proofRequestCount()
        val fetched = distribution().fetchEnvelope().decodeToString()
        assertEquals(before + 1, proofRequestCount())
        val generation = generationOf(fetched)
        repository.importManifest(fetched, trusted)
        assertTrue(repository.downloadMissing(generation, origin))
        // One manifest fetch plus three package downloads through production HTTP.
        assertEquals(before + 4, proofRequestCount())
        assertTrue(repository.activate(generation))
        val state = repository.publishedState()
        assertEquals(true, state?.complete)
        assertEquals(generation, state?.generation)
        assertArrayEquals(expectedBytes("east"), File(nativeRoot, "261004/east.mwm").readBytes())
        assertArrayEquals(expectedBytes("world"), File(nativeRoot, "261004/world.mwm").readBytes())
        assertArrayEquals(expectedBytes("world-coasts"), File(nativeRoot, "261004/world-coasts.mwm").readBytes())

        // Clean restart: new repository + reopened database over the same files.
        val restarted = repo()
        val frozen = proofRequestCount()
        val afterRestart = restarted.publishedState()
        assertEquals(true, afterRestart?.complete)
        assertEquals(generation, afterRestart?.generation)
        assertEquals(3, restarted.activeMapFiles().size)
        // Serving last-good performs zero new endpoint contact.
        assertEquals(frozen, proofRequestCount())

        // Unknown generation refuses locally without touching last-good.
        try {
            restarted.downloadMissing("gen-does-not-exist", origin)
            fail("unknown generation must fail")
        } catch (_: MapDownloadException) {
        }
        assertEquals(generation, restarted.publishedState()?.generation)
        assertEquals(frozen, proofRequestCount())
    }

    @Test fun corruptedTransferAndLeasedActiveGenerationRefuseDeletion() = runBlocking {
        val repository = repo()
        val fetched = distribution().fetchEnvelope().decodeToString()
        val generation = generationOf(fetched)
        repository.importManifest(fetched, trusted)
        assertTrue(repository.downloadMissing(generation, origin))
        assertTrue(repository.activate(generation))

        // A leased (navigation-held) active generation refuses deletion.
        repository.acquireGenerationLease(generation)
        try {
            assertFalse(repository.deleteGeneration(generation))
            assertEquals(generation, repository.publishedState()?.generation)
        } finally {
            repository.releaseGenerationLease(generation)
        }

        // Corrupted bytes on the wire fail closed and keep last-good published.
        fetchBytes("/proof/corrupt?id=east")
        try {
            fetchBytes("/proof/advance")
            val fetchedB = distribution().fetchEnvelope().decodeToString()
            val generationB = generationOf(fetchedB)
            repository.importManifest(fetchedB, trusted)
            try {
                repository.downloadMissing(generationB, origin)
                fail("corrupt transfer must fail")
            } catch (_: MapDownloadException) {
            }
            assertEquals(generation, repository.publishedState()?.generation)
            assertEquals(true, repository.publishedState()?.complete)
            assertArrayEquals(expectedBytes("east"), File(nativeRoot, "261004/east.mwm").readBytes())
        } finally {
            fetchBytes("/proof/corrupt?clear=1")
        }
    }

    @Test fun diskFullFailsClosedWithoutTouchingActive() = runBlocking {
        val repository = repo()
        val fetched = distribution().fetchEnvelope().decodeToString()
        val generation = generationOf(fetched)
        repository.importManifest(fetched, trusted)
        assertTrue(repository.downloadMissing(generation, origin))
        assertTrue(repository.activate(generation))

        // Disk full refuses before any byte is staged; last-good preserved. A new
        // generation is required: re-importing the active one is refused by design.
        fetchBytes("/proof/advance")
        val fullRepo = repo(freeBytes = 0)
        val fetchedB = distribution().fetchEnvelope().decodeToString()
        val generationB = generationOf(fetchedB)
        fullRepo.importManifest(fetchedB, trusted)
        try {
            fullRepo.downloadMissing(generationB, origin)
            fail("disk-full must fail")
        } catch (_: MapStorageException) {
        }
        assertEquals(generation, repository.publishedState()?.generation)
        assertEquals(true, repository.publishedState()?.complete)
    }

    private fun resumeOffsets(id: String): List<Int> {
        val ranges = JSONObject(fetchBytes("/proof/requests").decodeToString()).getJSONArray("ranges")
        return (0 until ranges.length()).map { ranges.getJSONObject(it) }
            .filter { it.getString("id") == id }.map { it.getInt("from") }
    }

    @Test fun interruptedUpdateKeepsLastGoodThenResumesFromThePartialFile() = runBlocking {
        val repository = repo()
        val fetched = distribution().fetchEnvelope().decodeToString()
        val generation = generationOf(fetched)
        repository.importManifest(fetched, trusted)
        assertTrue(repository.downloadMissing(generation, origin))
        assertTrue(repository.activate(generation))
        val lastGoodWorld = File(nativeRoot, "261004/world.mwm").readBytes()

        // The new generation's world transfer is cut after 50000 of 131072 bytes.
        fetchBytes("/proof/advance")
        fetchBytes("/proof/truncate?id=world&bytes=50000")
        val fetchedB = distribution().fetchEnvelope().decodeToString()
        val generationB = generationOf(fetchedB)
        repository.importManifest(fetchedB, trusted)
        try {
            try {
                repository.downloadMissing(generationB, origin)
                fail("interrupted transfer must fail")
            } catch (_: java.io.IOException) {
            }
            // Last-good stays complete, active and byte-identical while the update is interrupted.
            assertEquals(generation, repository.publishedState()?.generation)
            assertEquals(true, repository.publishedState()?.complete)
            assertArrayEquals(lastGoodWorld, File(nativeRoot, "261004/world.mwm").readBytes())
            val parts = stagingRoot.walkTopDown().filter { it.name == "world.mwm.part" }.toList()
            assertEquals(1, parts.size)
            assertEquals(50000L, parts.single().length())
        } finally {
            fetchBytes("/proof/truncate?clear=1")
        }

        // Retry resumes after the staged bytes only (206), verifies and publishes atomically.
        val offsets = resumeOffsets("world")
        assertTrue(repository.downloadMissing(generationB, origin))
        assertEquals(offsets + 50000, resumeOffsets("world"))
        assertEquals(0, offsets.last())
        assertTrue(repository.activate(generationB))
        assertEquals(generationB, repository.publishedState()?.generation)
        assertArrayEquals(expectedBytes("world"), File(nativeRoot, "261004/world.mwm").readBytes())
    }
}
