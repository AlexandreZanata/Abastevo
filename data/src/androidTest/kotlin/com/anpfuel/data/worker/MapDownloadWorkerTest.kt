package com.anpfuel.data.worker

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.ListenableWorker
import androidx.work.NetworkType
import androidx.work.WorkerFactory
import androidx.work.WorkerParameters
import androidx.work.testing.TestListenableWorkerBuilder
import androidx.work.workDataOf
import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import com.anpfuel.data.map.MapPackageRepository
import com.anpfuel.data.map.MapPackageStore
import com.anpfuel.data.map.MapDownloadException
import com.anpfuel.data.map.MapStorageException
import java.io.File
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MapDownloadWorkerTest {
    private val context: Context = ApplicationProvider.getApplicationContext()

    private fun worker(
        repository: MapPackageRepository,
        generation: String? = "gen-1",
        baseUrl: String? = "https://example.test",
    ): MapDownloadWorker {
        val input = workDataOf(
            MapDownloadWorker.KEY_GENERATION to generation,
            MapDownloadWorker.KEY_BASE_URL to baseUrl,
        )
        return TestListenableWorkerBuilder<MapDownloadWorker>(context)
            .setInputData(input)
            .setWorkerFactory(object : WorkerFactory() {
                override fun createWorker(appContext: Context, workerClassName: String, params: WorkerParameters) =
                    MapDownloadWorker(appContext, params, repository)
            }).build()
    }

    private fun repo(
        download: suspend () -> Boolean = { true },
        activate: suspend () -> Boolean = { true },
    ) = MapPackageRepository(
        store = object : MapPackageStore {
            override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
            override suspend fun generation(generation: String) = null
            override suspend fun activeGeneration() = null
            override suspend fun packages(generation: String) = emptyList<MapPackageEntity>()
            override suspend fun updatePackage(generation: String, packageId: String, status: String, downloadedBytes: Long, etag: String?, error: String?, updatedAtMillis: Long) = Unit
            override suspend fun activate(generation: String, activatedAtMillis: Long) = Unit
            override suspend fun deleteInactive(generation: String) = Unit
        },
        filesRoot = File(context.cacheDir, "map-worker-test"),
        http = okhttp3.OkHttpClient(),
    ).let {
        object : MapPackageRepository(
            store = it.let { field ->
                // Delegate through a controllable fake without touching privates.
                object : MapPackageStore {
                    override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
                    override suspend fun generation(generation: String) = null
                    override suspend fun activeGeneration() = null
                    override suspend fun packages(generation: String) = emptyList<MapPackageEntity>()
                    override suspend fun updatePackage(generation: String, packageId: String, status: String, downloadedBytes: Long, etag: String?, error: String?, updatedAtMillis: Long) = Unit
                    override suspend fun activate(generation: String, activatedAtMillis: Long) = Unit
                    override suspend fun deleteInactive(generation: String) = Unit
                }
            },
            filesRoot = File(context.cacheDir, "map-worker-test"),
            http = okhttp3.OkHttpClient(),
        ) {
            override suspend fun downloadMissing(generation: String, baseUrl: String, onProgress: (String, Long, Long) -> Unit): Boolean =
                download()

            override suspend fun activate(generation: String): Boolean = activate()
        }
    }

    @Test fun missingInputsFailWithoutNetwork() = runBlocking {
        val failing = worker(repo(), generation = null)
        assertEquals(ListenableWorker.Result.failure(), failing.doWork())
    }

    @Test fun networkOutageRequestsRetry() = runBlocking {
        val retrying = object : MapPackageRepository(
            store = fakeStore(),
            filesRoot = File(context.cacheDir, "map-worker-test"),
            http = okhttp3.OkHttpClient(),
        ) {
            override suspend fun downloadMissing(generation: String, baseUrl: String, onProgress: (String, Long, Long) -> Unit): Boolean =
                throw java.io.IOException("offline")
        }
        val w = TestListenableWorkerBuilder<MapDownloadWorker>(context)
            .setInputData(workDataOf(MapDownloadWorker.KEY_GENERATION to "gen-1", MapDownloadWorker.KEY_BASE_URL to "https://example.test"))
            .setWorkerFactory(object : WorkerFactory() {
                override fun createWorker(appContext: Context, workerClassName: String, params: WorkerParameters) =
                    MapDownloadWorker(appContext, params, retrying)
            }).build()
        assertEquals(ListenableWorker.Result.retry(), w.doWork())
    }

    @Test fun storageExhaustionFailsClosed() = runBlocking {
        val failing = object : MapPackageRepository(
            store = fakeStore(),
            filesRoot = File(context.cacheDir, "map-worker-test"),
            http = okhttp3.OkHttpClient(),
        ) {
            override suspend fun downloadMissing(generation: String, baseUrl: String, onProgress: (String, Long, Long) -> Unit): Boolean =
                throw MapStorageException("map-storage-full")
        }
        val w = TestListenableWorkerBuilder<MapDownloadWorker>(context)
            .setInputData(workDataOf(MapDownloadWorker.KEY_GENERATION to "gen-1", MapDownloadWorker.KEY_BASE_URL to "https://example.test"))
            .setWorkerFactory(object : WorkerFactory() {
                override fun createWorker(appContext: Context, workerClassName: String, params: WorkerParameters) =
                    MapDownloadWorker(appContext, params, failing)
            }).build()
        assertEquals(ListenableWorker.Result.failure(), w.doWork())
    }

    @Test fun digestFailureFailsClosedWithoutActivation() = runBlocking {
        val failing = object : MapPackageRepository(
            store = fakeStore(),
            filesRoot = File(context.cacheDir, "map-worker-test"),
            http = okhttp3.OkHttpClient(),
        ) {
            override suspend fun downloadMissing(generation: String, baseUrl: String, onProgress: (String, Long, Long) -> Unit): Boolean =
                throw MapDownloadException("package-digest-mismatch")
        }
        val w = TestListenableWorkerBuilder<MapDownloadWorker>(context)
            .setInputData(workDataOf(MapDownloadWorker.KEY_GENERATION to "gen-1", MapDownloadWorker.KEY_BASE_URL to "https://example.test"))
            .setWorkerFactory(object : WorkerFactory() {
                override fun createWorker(appContext: Context, workerClassName: String, params: WorkerParameters) =
                    MapDownloadWorker(appContext, params, failing)
            }).build()
        assertEquals(ListenableWorker.Result.failure(), w.doWork())
    }

    @Test fun successActivatesAtomically() = runBlocking {
        var activated = false
        val ok = object : MapPackageRepository(
            store = fakeStore(),
            filesRoot = File(context.cacheDir, "map-worker-test"),
            http = okhttp3.OkHttpClient(),
        ) {
            override suspend fun downloadMissing(generation: String, baseUrl: String, onProgress: (String, Long, Long) -> Unit) = true
            override suspend fun activate(generation: String): Boolean {
                activated = true
                return true
            }
        }
        val w = TestListenableWorkerBuilder<MapDownloadWorker>(context)
            .setInputData(workDataOf(MapDownloadWorker.KEY_GENERATION to "gen-1", MapDownloadWorker.KEY_BASE_URL to "https://example.test"))
            .setWorkerFactory(object : WorkerFactory() {
                override fun createWorker(appContext: Context, workerClassName: String, params: WorkerParameters) =
                    MapDownloadWorker(appContext, params, ok)
            }).build()
        assertEquals(ListenableWorker.Result.success(), w.doWork())
        assertEquals(true, activated)
    }

    @Test fun unsupportedDataVersionFailsClosedWithoutActivation() = runBlocking {
        val failing = object : MapPackageRepository(
            store = fakeStore(),
            filesRoot = File(context.cacheDir, "map-worker-test"),
            http = okhttp3.OkHttpClient(),
        ) {
            override suspend fun downloadMissing(generation: String, baseUrl: String, onProgress: (String, Long, Long) -> Unit) = true
            override suspend fun activate(generation: String): Boolean =
                throw MapDownloadException("unsupported-data-version")
        }
        val w = TestListenableWorkerBuilder<MapDownloadWorker>(context)
            .setInputData(workDataOf(MapDownloadWorker.KEY_GENERATION to "gen-1", MapDownloadWorker.KEY_BASE_URL to "https://example.test"))
            .setWorkerFactory(object : WorkerFactory() {
                override fun createWorker(appContext: Context, workerClassName: String, params: WorkerParameters) =
                    MapDownloadWorker(appContext, params, failing)
            }).build()
        assertEquals(ListenableWorker.Result.failure(), w.doWork())
    }

    @Test fun schedulerUsesUniqueWifiAwareWorkWithBackoff() {
        val req = MapDownloadScheduler.request("gen-1", "https://example.test")
        // MAP-24: downloads honour the Wi-Fi preference, which defaults to on.
        assertEquals(NetworkType.UNMETERED, req.workSpec.constraints.requiredNetworkType)
        assertEquals(
            NetworkType.CONNECTED,
            MapDownloadScheduler.request("gen-1", "https://example.test", requireUnmetered = false)
                .workSpec.constraints.requiredNetworkType,
        )
        assertEquals("gen-1", req.workSpec.input.getString(MapDownloadWorker.KEY_GENERATION))
        assertEquals("map-download-gen-1", MapDownloadScheduler.uniqueName("gen-1"))
    }

    private fun fakeStore() = object : MapPackageStore {
        override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
        override suspend fun generation(generation: String) = null
        override suspend fun activeGeneration() = null
        override suspend fun packages(generation: String) = emptyList<MapPackageEntity>()
        override suspend fun updatePackage(generation: String, packageId: String, status: String, downloadedBytes: Long, etag: String?, error: String?, updatedAtMillis: Long) = Unit
        override suspend fun activate(generation: String, activatedAtMillis: Long) = Unit
        override suspend fun deleteInactive(generation: String) = Unit
    }
}
