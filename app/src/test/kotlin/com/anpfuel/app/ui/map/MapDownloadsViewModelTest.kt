package com.anpfuel.app.ui.map

import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import com.anpfuel.data.map.MapDistribution
import com.anpfuel.data.map.MapDistributionConfig
import com.anpfuel.data.map.MapDownloadException
import com.anpfuel.data.map.MapPackageRepository
import com.anpfuel.data.map.MapPackageStore
import com.anpfuel.data.map.PACKAGE_FAILED
import androidx.lifecycle.SavedStateHandle
import java.io.File
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MapDownloadsViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    private class FakeStore : MapPackageStore {
        var generations: MutableMap<String, MapGenerationEntity> = mutableMapOf()
        var packages: MutableList<MapPackageEntity> = mutableListOf()
        var active: MapGenerationEntity? = null
        override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) {
            generations[generation.generation] = generation
            this.packages.removeAll { it.generation == generation.generation }
            this.packages.addAll(packages)
        }
        override suspend fun generation(generation: String) =
            if (active?.generation == generation) active else generations[generation]
        override suspend fun activeGeneration() = active
        override suspend fun packages(generation: String) = packages.filter { it.generation == generation }
        override suspend fun updatePackage(generation: String, packageId: String, status: String, downloadedBytes: Long, etag: String?, error: String?, updatedAtMillis: Long) {
            packages = packages.map { if (it.generation == generation && it.packageId == packageId) it.copy(status = status, downloadedBytes = downloadedBytes, etag = etag, error = error) else it }.toMutableList()
        }
        override suspend fun activate(generation: String, activatedAtMillis: Long) {
            val gen = generations[generation] ?: return
            active = gen.copy(active = true, activatedAtMillis = activatedAtMillis)
        }
        override suspend fun deleteInactive(generation: String) {
            generations.remove(generation)
        }
        override suspend fun inactiveGenerations(): List<String> =
            generations.keys.filter { it != active?.generation }.sorted()
    }

    private class FakeWork : MapWorkControl {
        val enqueued = mutableListOf<String>()
        val enqueuedUnmetered = mutableListOf<Boolean>()
        val cancelled = mutableListOf<String>()
        override fun enqueue(generation: String, requireUnmetered: Boolean) {
            enqueued += generation
            enqueuedUnmetered += requireUnmetered
        }
        override fun cancel(generation: String) {
            cancelled += generation
        }
    }

    private class FakePrefs(var wifiOnly: Boolean = true) : com.anpfuel.domain.repository.UserPreferencesRepository {
        override suspend fun getPreferences() = com.anpfuel.domain.model.UserPreferences(autoSyncOnWifi = wifiOnly)
        override suspend fun savePreferences(preferences: com.anpfuel.domain.model.UserPreferences) {
            wifiOnly = preferences.autoSyncOnWifi
        }
    }

    private fun config(keys: Map<String, ByteArray> = mapOf("k" to ByteArray(32))) =
        MapDistributionConfig("https://example.test", "test-rect", keys, keysInvalid = false)

    private fun vm(
        store: FakeStore,
        fetch: (suspend () -> ByteArray)? = null,
        work: FakeWork = FakeWork(),
        cfg: MapDistributionConfig = config(),
        prefs: FakePrefs = FakePrefs(),
    ): Pair<MapDownloadsViewModel, FakeWork> {
        val distribution = object : MapDistribution(okhttp3.OkHttpClient(), cfg) {
            override suspend fun fetchEnvelope(regionSlug: String): ByteArray = fetch?.invoke() ?: error("no fetch stub")
        }
        val repository = object : MapPackageRepository(store, File(System.getProperty("java.io.tmpdir"), "map-vm-test-${System.nanoTime()}"), okhttp3.OkHttpClient(), ioDispatcher = dispatcher) {
            override suspend fun importManifest(envelopeJson: String, trustedKeys: Map<String, ByteArray>) =
                throw UnsupportedOperationException("unused in ViewModel test")
        }
        return MapDownloadsViewModel(cfg, distribution, repository, store, work, prefs) to work
    }

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
    }

    @AfterEach fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test fun `unconfigured shows the specific pendency without network`() = runTest {
        val store = FakeStore()
        val (viewModel, work) = vm(
            store,
            cfg = MapDistributionConfig("", "", emptyMap(), keysInvalid = false),
        )
        dispatcher.scheduler.advanceUntilIdle()
        val ui = viewModel.ui.value
        assertTrue(ui.unconfigured)
        assertTrue(ui.missingUrl)
        assertTrue(ui.missingKeys)
        assertTrue(work.enqueued.isEmpty())
    }

    @Test fun `network failure is retryable and never a no-package state`() = runTest {
        val store = FakeStore()
        val (viewModel, work) = vm(store, fetch = { throw IOException("offline") })
        viewModel.checkForUpdate()
        dispatcher.scheduler.advanceUntilIdle()
        val ui = viewModel.ui.value
        assertEquals("network-offline", ui.error)
        assertTrue(ui.retryable)
        assertEquals(null, ui.candidateGeneration)
        assertTrue(work.enqueued.isEmpty())
    }

    @Test fun `check imports envelope and shows the catalog`() = runTest {
        val store = FakeStore()
        // Real repository path: import writes Room rows the UI then lists.
        val realRepo = MapPackageRepository(store, File(System.getProperty("java.io.tmpdir"), "map-vm-test-${System.nanoTime()}"), okhttp3.OkHttpClient(), ioDispatcher = dispatcher)
        val manifest = catalogManifest()
        val kp = java.security.KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val rawPub = kp.public.encoded.let { it.copyOfRange(it.size - 32, it.size) }
        val envelope = signEnvelope(manifest, kp)
        val distribution = object : MapDistribution(okhttp3.OkHttpClient(), config(mapOf("k" to rawPub))) {
            override suspend fun fetchEnvelope(regionSlug: String): ByteArray = envelope.toByteArray()
        }
        val viewModel = MapDownloadsViewModel(
            config(mapOf("k" to rawPub)), distribution, realRepo, store, FakeWork(), FakePrefs(),
        )
        viewModel.checkForUpdate()
        dispatcher.scheduler.advanceUntilIdle()
        val ui = viewModel.ui.value
        assertEquals("g-catalog", ui.candidateGeneration)
        assertEquals(100, ui.candidateBytes)
        assertEquals(listOf("a"), ui.candidatePackages.map { it.packageId })
        assertEquals(null, ui.error)
    }

    @Test fun `download enqueues unique work and cancel stops observation`() = runTest {
        val store = FakeStore()
        val manifest = catalogManifest()
        val kp = java.security.KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val rawPub = kp.public.encoded.let { it.copyOfRange(it.size - 32, it.size) }
        val envelope = signEnvelope(manifest, kp)
        val realRepo = MapPackageRepository(store, File(System.getProperty("java.io.tmpdir"), "map-vm-test-${System.nanoTime()}"), okhttp3.OkHttpClient(), ioDispatcher = dispatcher)
        val distribution = object : MapDistribution(okhttp3.OkHttpClient(), config(mapOf("k" to rawPub))) {
            override suspend fun fetchEnvelope(regionSlug: String): ByteArray = envelope.toByteArray()
        }
        val work = FakeWork()
        val viewModel = MapDownloadsViewModel(config(mapOf("k" to rawPub)), distribution, realRepo, store, work, FakePrefs())
        viewModel.checkForUpdate()
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("g-catalog", viewModel.ui.value.candidateGeneration)
        viewModel.download()
        assertEquals(listOf("g-catalog"), work.enqueued)
        dispatcher.scheduler.advanceTimeBy(600)
        assertTrue(viewModel.ui.value.downloading)
        viewModel.cancel()
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(listOf("g-catalog"), work.cancelled)
        assertFalse(viewModel.ui.value.downloading)
    }

    @Test fun `failed package during download surfaces retryable error`() = runTest {
        val store = FakeStore()
        val manifest = catalogManifest()
        val kp = java.security.KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val rawPub = kp.public.encoded.let { it.copyOfRange(it.size - 32, it.size) }
        val envelope = signEnvelope(manifest, kp)
        val realRepo = MapPackageRepository(store, File(System.getProperty("java.io.tmpdir"), "map-vm-test-${System.nanoTime()}"), okhttp3.OkHttpClient(), ioDispatcher = dispatcher)
        val distribution = object : MapDistribution(okhttp3.OkHttpClient(), config(mapOf("k" to rawPub))) {
            override suspend fun fetchEnvelope(regionSlug: String): ByteArray = envelope.toByteArray()
        }
        val viewModel = MapDownloadsViewModel(config(mapOf("k" to rawPub)), distribution, realRepo, store, FakeWork(), FakePrefs())
        viewModel.checkForUpdate()
        dispatcher.scheduler.advanceUntilIdle()
        viewModel.download()
        store.updatePackage("g-catalog", "a", PACKAGE_FAILED, 10, null, "digest-mismatch", 1L)
        dispatcher.scheduler.advanceTimeBy(1200)
        dispatcher.scheduler.advanceUntilIdle()
        val ui = viewModel.ui.value
        assertFalse(ui.downloading)
        assertEquals("digest-mismatch", ui.error)
        assertTrue(ui.retryable)
    }

    private fun catalogManifest(): String = """
        {
          "format":"abastevo.native-map-manifest-v1","version":1,"generation":"g-catalog",
          "engine":{"name":"comaps","upstream_sha":"${"4".repeat(40)}","generator_sha":"${"5".repeat(40)}","format_version":"2601"},
          "region":{"id":"test-rect","name":"Rect","bbox":{"min_lon_deg":-1.0,"min_lat_deg":-1.0,"max_lon_deg":1.0,"max_lat_deg":1.0},"overview":"world"},
          "packages":[{"id":"a","role":"REGION","path":"a.mwm","bytes":100,"sha256":"${"a".repeat(64)}"}],
          "dependencies":[],
          "sources":[{"id":"s","license":"MIT","attribution":"t","sha256":"${"b".repeat(64)}"}],
          "rights":{"license":"MIT","attribution":"t","commercial":true,"offline":true,"redistribution":true},
          "generated_at":"2026-10-25T12:00:00Z"
        }
    """.trimIndent()

    private fun signEnvelope(manifest: String, kp: java.security.KeyPair): String {
        val sig = java.security.Signature.getInstance("Ed25519").run {
            initSign(kp.private); update(manifest.toByteArray()); sign()
        }
        fun b64(b: ByteArray) = java.util.Base64.getUrlEncoder().withoutPadding().encodeToString(b)
        return """{"format":"abastevo.map-distribution-v1","version":1,"key_id":"k","algorithm":"Ed25519","manifest":"${b64(manifest.toByteArray())}","manifest_sha256":"${sha(manifest.toByteArray())}","signature":"${b64(sig)}"}"""
    }

    private fun sha(b: ByteArray) = java.security.MessageDigest.getInstance("SHA-256").digest(b)
        .joinToString("") { "%02x".format(it) }

    @Test fun `activateActive reuses the active generation without a new check`() = runTest {
        val store = FakeStore()
        store.active = MapGenerationEntity("g1", "h", true, 1L, "2601")
        store.packages.add(
            MapPackageEntity("a", "g1", "REGION", "a.mwm", 100, "a".repeat(64), null, "READY", 100, null, 1L),
        )
        val work = FakeWork()
        val viewModel = MapDownloadsViewModel(config(), object : MapDistribution(okhttp3.OkHttpClient(), config()) {
            override suspend fun fetchEnvelope(regionSlug: String): ByteArray = error("must not fetch")
        }, MapPackageRepository(store, File(System.getProperty("java.io.tmpdir"), "map-vm-test-${System.nanoTime()}"), okhttp3.OkHttpClient(), ioDispatcher = dispatcher), store, work, FakePrefs())
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("g1", viewModel.ui.value.activeGeneration)
        viewModel.activateActive()
        dispatcher.scheduler.advanceTimeBy(600)
        assertEquals(listOf("g1"), work.enqueued)
        assertTrue(viewModel.ui.value.downloading)
    }

    @Test fun `same generation check shows current instead of failing`() = runTest {
        val store = FakeStore()
        val manifest = catalogManifest()
        val kp = java.security.KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val rawPub = kp.public.encoded.let { it.copyOfRange(it.size - 32, it.size) }
        val envelope = signEnvelope(manifest, kp)
        val realRepo = MapPackageRepository(store, File(System.getProperty("java.io.tmpdir"), "map-vm-test-${System.nanoTime()}"), okhttp3.OkHttpClient(), ioDispatcher = dispatcher)
        val distribution = object : MapDistribution(okhttp3.OkHttpClient(), config(mapOf("k" to rawPub))) {
            override suspend fun fetchEnvelope(regionSlug: String): ByteArray = envelope.toByteArray()
        }
        val viewModel = MapDownloadsViewModel(config(mapOf("k" to rawPub)), distribution, realRepo, store, FakeWork(), FakePrefs())
        viewModel.checkForUpdate()
        dispatcher.scheduler.advanceUntilIdle()
        // Simulate activation by the worker, then check again: same generation
        // must surface Current, not an active-generation-reimport failure.
        store.active = MapGenerationEntity("g-catalog", "h", true, 1L, "2601")
        viewModel.checkForUpdate()
        dispatcher.scheduler.advanceUntilIdle()
        val ui = viewModel.ui.value
        assertEquals("g-catalog", ui.activeGeneration)
        assertEquals(null, ui.candidateGeneration)
        assertEquals(null, ui.error)
    }

    @Test fun `wifi preference selects unmetered work and refresh surfaces it`() = runTest {
        val store = FakeStore()
        val manifest = catalogManifest()
        val kp = java.security.KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val rawPub = kp.public.encoded.let { it.copyOfRange(it.size - 32, it.size) }
        val envelope = signEnvelope(manifest, kp)
        val realRepo = MapPackageRepository(store, File(System.getProperty("java.io.tmpdir"), "map-vm-test-${System.nanoTime()}"), okhttp3.OkHttpClient(), ioDispatcher = dispatcher)
        val distribution = object : MapDistribution(okhttp3.OkHttpClient(), config(mapOf("k" to rawPub))) {
            override suspend fun fetchEnvelope(regionSlug: String): ByteArray = envelope.toByteArray()
        }
        val work = FakeWork()
        val viewModel = MapDownloadsViewModel(
            config(mapOf("k" to rawPub)), distribution, realRepo, store, work, FakePrefs(wifiOnly = false),
        )
        viewModel.checkForUpdate()
        dispatcher.scheduler.advanceUntilIdle()
        assertFalse(viewModel.ui.value.wifiOnly)
        viewModel.download()
        dispatcher.scheduler.advanceTimeBy(600)
        assertEquals(listOf("g-catalog"), work.enqueued)
        assertEquals(listOf(false), work.enqueuedUnmetered)
        assertTrue(viewModel.ui.value.downloading)
        viewModel.cancel()
        dispatcher.scheduler.advanceUntilIdle()
    }

    @Test fun `delete inactive refuses the active generation with explicit in-use state`() = runTest {
        val store = FakeStore()
        store.generations["g-active"] = MapGenerationEntity("g-active", "h", false, 1L, "2601")
        store.generations["g-stale"] = MapGenerationEntity("g-stale", "h", false, 1L, "2601")
        store.active = MapGenerationEntity("g-active", "h", true, 1L, "2601")
        val repository = MapPackageRepository(store, File(System.getProperty("java.io.tmpdir"), "map-vm-test-${System.nanoTime()}"), okhttp3.OkHttpClient(), ioDispatcher = dispatcher)
        val viewModel = MapDownloadsViewModel(
            config(), object : MapDistribution(okhttp3.OkHttpClient(), config()) {
                override suspend fun fetchEnvelope(regionSlug: String): ByteArray = error("unused")
            }, repository, store, FakeWork(), FakePrefs(),
        )
        dispatcher.scheduler.advanceUntilIdle()
        viewModel.deleteInactive("g-active")
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("in-use", viewModel.ui.value.error)
        assertTrue(store.generations.containsKey("g-active"))
        viewModel.deleteInactive("g-stale")
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(null, viewModel.ui.value.error)
        assertFalse(store.generations.containsKey("g-stale"))
    }

    private class Signed(val rawPub: ByteArray, val envelope: String) {
        val requested = mutableListOf<String>()
    }

    private fun signed(manifest: String = catalogManifest()): Signed {
        val kp = java.security.KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val rawPub = kp.public.encoded.let { it.copyOfRange(it.size - 32, it.size) }
        return Signed(rawPub, signEnvelope(manifest, kp))
    }

    private fun regionalVm(
        signed: Signed,
        store: FakeStore,
        selection: MapRegionSelection,
        work: FakeWork = FakeWork(),
        defaultRegion: String = "",
        savedState: SavedStateHandle = SavedStateHandle(),
    ): MapDownloadsViewModel {
        val cfg = MapDistributionConfig("https://example.test", defaultRegion, mapOf("k" to signed.rawPub), keysInvalid = false)
        val distribution = object : MapDistribution(okhttp3.OkHttpClient(), cfg) {
            override suspend fun fetchEnvelope(regionSlug: String): ByteArray {
                signed.requested += regionSlug
                return signed.envelope.toByteArray()
            }
        }
        val repo = MapPackageRepository(store, File(System.getProperty("java.io.tmpdir"), "map-vm-test-${System.nanoTime()}"), okhttp3.OkHttpClient(), ioDispatcher = dispatcher)
        return MapDownloadsViewModel(cfg, distribution, repo, store, work, FakePrefs(), selection, savedState)
    }

    private fun regionManifest(region: String) = catalogManifest().replace("\"id\":\"test-rect\"", "\"id\":\"$region\"")

    @Test fun `no build region and no opened city shows a clear region message, not unconfigured`() = runTest {
        val signed = signed()
        val viewModel = regionalVm(signed, FakeStore(), MapRegionSelection())
        dispatcher.scheduler.advanceUntilIdle()
        val ui = viewModel.ui.value
        assertFalse(ui.unconfigured)
        assertTrue(ui.regionUnknown)
        assertEquals(null, ui.region)
        viewModel.checkForUpdate()
        dispatcher.scheduler.advanceUntilIdle()
        assertTrue(signed.requested.isEmpty())
        assertTrue(viewModel.ui.value.regionUnknown)
    }

    @Test fun `opened city region overrides the build default and is fetched`() = runTest {
        val signed = signed(regionManifest("brazil-mato-grosso"))
        val selection = MapRegionSelection().apply { select("brazil-mato-grosso") }
        val viewModel = regionalVm(signed, FakeStore(), selection, defaultRegion = "brazil-bahia")
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("Mato Grosso", viewModel.ui.value.region?.name)
        assertFalse(viewModel.ui.value.regionUnknown)
        viewModel.checkForUpdate()
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(listOf("brazil-mato-grosso"), signed.requested)
        assertEquals("g-catalog", viewModel.ui.value.candidateGeneration)
    }

    @Test fun `manifest of another region is refused before any row is written`() = runTest {
        val signed = signed(regionManifest("brazil-bahia"))
        val store = FakeStore()
        val selection = MapRegionSelection().apply { select("brazil-mato-grosso") }
        val viewModel = regionalVm(signed, store, selection)
        viewModel.checkForUpdate()
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("map-region-mismatch", viewModel.ui.value.error)
        assertEquals(null, viewModel.ui.value.candidateGeneration)
        assertTrue(store.packages.isEmpty() && store.generations.isEmpty())
    }

    @Test fun `suggestion download request checks and starts that region once`() = runTest {
        val signed = signed(regionManifest("brazil-mato-grosso"))
        val selection = MapRegionSelection().apply { requestDownload("brazil-mato-grosso") }
        val work = FakeWork()
        val viewModel = regionalVm(signed, FakeStore(), selection, work)
        dispatcher.scheduler.advanceTimeBy(100)
        dispatcher.scheduler.runCurrent()
        assertEquals(listOf("brazil-mato-grosso"), signed.requested)
        assertEquals(listOf("g-catalog"), work.enqueued)
        assertEquals(listOf(true), work.enqueuedUnmetered)
        assertFalse(selection.consumeDownloadRequest("brazil-mato-grosso"), "request is one-shot")
        viewModel.cancel()
        dispatcher.scheduler.advanceUntilIdle()
        // A second screen instance does not start again.
        regionalVm(signed, FakeStore(), selection, work)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(listOf("g-catalog"), work.enqueued)
    }

    @Test fun `region survives process recreation through saved state`() = runTest {
        val signed = signed(regionManifest("brazil-parana-east"))
        val saved = SavedStateHandle()
        regionalVm(signed, FakeStore(), MapRegionSelection().apply { select("brazil-parana-east") }, savedState = saved)
        dispatcher.scheduler.advanceUntilIdle()
        val restored = regionalVm(signed, FakeStore(), MapRegionSelection(), savedState = saved)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("brazil-parana-east", restored.ui.value.region?.slug)
        assertEquals("Paraná – Leste", restored.ui.value.region?.name)
    }
}
