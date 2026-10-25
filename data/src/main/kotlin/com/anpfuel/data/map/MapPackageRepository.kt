package com.anpfuel.data.map

import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import com.anpfuel.domain.map.MapContractException
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.io.RandomAccessFile
import java.security.MessageDigest
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicInteger
import kotlin.coroutines.coroutineContext
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request

class MapDownloadException(message: String, cause: Throwable? = null) : Exception(message, cause)

class MapStorageException(message: String) : Exception(message)

private val NATIVE_DATA_VERSION = Regex("^[0-9]{1,6}$")
private val CONTENT_RANGE = Regex("^bytes (\\d+)-(\\d+)/(\\d+)$")

/** Numeric SDK data-version directory (`FindAllLocalMapsAndCleanup` parity). */
internal fun isNativeDataVersion(value: String): Boolean = NATIVE_DATA_VERSION.matches(value)

/**
 * Native file name inside the version directory. The manifest path may carry
 * the `<version>/` prefix (probe layout with flat files directly under the
 * numeric directory); anything else nested cannot be registered by the pinned
 * engine, which only scans immediate files of the numeric version directory.
 * Returns null when the path cannot be published flat.
 */
internal fun publishedName(version: String, path: String): String? {
    if (path.isBlank() || path.contains("..") || path.startsWith("/") || path.contains('\\')) return null
    val stripped = if (path.startsWith("$version/")) path.removePrefix("$version/") else path
    if (stripped.isEmpty() || stripped.contains('/')) return null
    return stripped
}

/**
 * Validates a 206 `Content-Range` against the resume offset we requested.
 * A missing/mismatched range with a non-empty `.part` file fails closed so a
 * server that ignores `Range` can never corrupt the staged bytes.
 */
internal fun checkResumeRange(contentRange: String?, resumedFrom: Long, totalBytes: Long) {
    if (resumedFrom <= 0) return
    val match = contentRange?.let { CONTENT_RANGE.matchEntire(it.trim()) }
        ?: throw MapDownloadException("package-range-mismatch")
    val start = match.groupValues[1].toLongOrNull()
    val total = match.groupValues[3].toLongOrNull()
    if (start != resumedFrom || total != totalBytes) throw MapDownloadException("package-range-mismatch")
}

private data class VerifiedFileStamp(
    val length: Long,
    val lastModified: Long,
    val sampleSha256: String,
    val fullSha256: String,
)

open class MapPackageRepository(
    private val store: MapPackageStore,
    private val filesRoot: File,
    private val http: OkHttpClient,
    private val clock: () -> Long = { System.currentTimeMillis() },
    private val freeBytes: (File) -> Long = { it.usableSpace },
    private val reserveBytes: Long = 32L * 1024 * 1024,
    private val nativeRoot: File = filesRoot,
    private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO,
    private val onGenerationActivated: suspend (generation: String, dataVersion: String) -> Unit = { _, _ -> },
) {
    private val publishMutex = Mutex()
    private val leaseLock = Any()
    private val generationLeases = mutableMapOf<String, Int>()
    private val verifiedStamps = ConcurrentHashMap<String, VerifiedFileStamp>()
    private val fullHashCounter = AtomicInteger(0)
    private val activationListeners = CopyOnWriteArrayList<suspend (generation: String, dataVersion: String) -> Unit>()

    internal var publishRenameHook: ((packageId: String, tmp: File, dst: File) -> Unit)? = null

    fun registerActivationListener(
        listener: suspend (generation: String, dataVersion: String) -> Unit,
    ): AutoCloseable {
        activationListeners.add(listener)
        return AutoCloseable { activationListeners.remove(listener) }
    }

    fun acquireGenerationLease(generation: String) {
        synchronized(leaseLock) {
            generationLeases[generation] = (generationLeases[generation] ?: 0) + 1
        }
    }

    fun releaseGenerationLease(generation: String) {
        synchronized(leaseLock) {
            val current = generationLeases[generation] ?: return
            if (current <= 1) {
                generationLeases.remove(generation)
            } else {
                generationLeases[generation] = current - 1
            }
        }
    }

    fun hasGenerationLease(generation: String): Boolean = synchronized(leaseLock) {
        (generationLeases[generation] ?: 0) > 0
    }

    private fun leasedGenerationsSnapshot(): Set<String> = synchronized(leaseLock) {
        generationLeases.filterValues { it > 0 }.keys.toSet()
    }

    private suspend fun <T> onIo(block: suspend kotlinx.coroutines.CoroutineScope.() -> T): T {
        val callerInterceptor = coroutineContext[kotlin.coroutines.ContinuationInterceptor]
        val effectiveContext = if (
            ioDispatcher === Dispatchers.IO &&
            callerInterceptor != null &&
            callerInterceptor.javaClass.name.startsWith("kotlinx.coroutines.test.")
        ) {
            callerInterceptor
        } else {
            ioDispatcher
        }
        return withContext(effectiveContext, block)
    }

    suspend fun <T> withActiveGenerationLease(
        block: suspend (generation: String, files: List<File>) -> T,
    ): T? = onIo {
        val activeGen = publishMutex.withLock {
            recoverUncommittedPublishLocked()
            val active = store.activeGeneration() ?: return@withLock null
            acquireGenerationLease(active.generation)
            active.generation
        } ?: return@onIo null
        try {
            val files = activeMapFiles()
            if (files.isEmpty()) return@onIo null
            block(activeGen, files)
        } finally {
            releaseGenerationLease(activeGen)
        }
    }

    suspend fun reclaimRetiredGenerations() = onIo {
        publishMutex.withLock {
            recoverUncommittedPublishLocked()
            val active = store.activeGeneration() ?: return@withLock
            if (!isNativeDataVersion(active.dataVersion)) return@withLock
            val activePackages = store.packages(active.generation)
            cleanInactiveLocked(
                activeGeneration = active.generation,
                activeVersion = active.dataVersion,
                expectedPaths = activePackages.mapNotNullTo(mutableSetOf()) {
                    publishedName(active.dataVersion, it.fileName)
                },
            )
        }
    }

    fun fullSha256ComputationCount(): Int = fullHashCounter.get()

    open suspend fun importManifestWithPlatformKeys(
        envelopeJson: String,
        trustedKeys: Map<String, java.security.PublicKey>,
    ): VerifiedMapManifest = onIo {
        val verified = MapManifestVerifier.verify(envelopeJson, trustedKeys)
        insertVerified(verified)
    }

    /**
     * Production import against raw 32-byte pinned keys (works without the
     * platform Ed25519 provider). Unknown key ids and wrong key sizes fail
     * closed before any row is written.
     */
    open suspend fun importManifest(
        envelopeJson: String,
        trustedKeys: Map<String, ByteArray>,
    ): VerifiedMapManifest = onIo {
        val verified = MapManifestVerifier.verify(envelopeJson, trustedKeys)
        insertVerified(verified)
    }

    private suspend fun insertVerified(verified: VerifiedMapManifest): VerifiedMapManifest {
        val manifest = verified.manifest
        val existing = store.generation(manifest.generation)
        if (existing?.active == true && existing.dataVersion.isNotBlank()) {
            throw MapDownloadException("active-generation-reimport")
        }
        // Legacy rows predate native-layout publication (empty data_version).
        // Re-importing the same active generation repairs them in place and
        // keeps the active flag; anything else stays refused.
        val keepActive = existing?.active == true
        store.insertGeneration(
            MapGenerationEntity(
                generation = manifest.generation,
                manifestSha256 = verified.manifestSha256,
                active = keepActive,
                activatedAtMillis = existing?.activatedAtMillis,
                dataVersion = manifest.formatVersion,
            ),
            manifest.packageRows(manifest.generation, clock()),
        )
        return verified
    }

    open suspend fun downloadMissing(
        generation: String,
        baseUrl: String,
        onProgress: (packageId: String, downloaded: Long, total: Long) -> Unit = { _, _, _ -> },
    ): Boolean = onIo {
        val packages = store.packages(generation)
        if (packages.isEmpty()) throw MapDownloadException("unknown-generation")
        var allReady = true
        for (pkg in packages) {
            coroutineContext.ensureActive()
            if (pkg.status == PACKAGE_READY && finalFile(generation, pkg).let {
                    it.isFile && it.length() == pkg.bytes && sha256(it) == pkg.sha256
                }
            ) {
                continue
            }
            allReady = false
            downloadPackage(generation, pkg, baseUrl, onProgress) || return@onIo false
        }
        allReady || store.packages(generation).all { it.status == PACKAGE_READY }
    }

    open suspend fun activate(generation: String): Boolean = onIo {
        val packages = store.packages(generation)
        if (packages.isEmpty() || packages.any { it.status != PACKAGE_READY }) return@onIo false
        val manifestVersion = store.generation(generation)?.dataVersion.orEmpty()
        if (!isNativeDataVersion(manifestVersion)) throw MapDownloadException("unsupported-data-version")
        publishMutex.withLock {
            recoverUncommittedPublishLocked()
            // Re-verify under the mutex: a concurrent activation may have
            // recycled this staging area after the pre-check above.
            val fresh = store.packages(generation)
            if (fresh.isEmpty() || fresh.any { it.status != PACKAGE_READY }) return@withLock false
            for (pkg in fresh) {
                coroutineContext.ensureActive()
                publishedName(manifestVersion, pkg.fileName)
                    ?: throw MapDownloadException("unsupported-package-layout")
                val file = finalFile(generation, pkg)
                if (!file.isFile || file.length() != pkg.bytes || sha256(file) != pkg.sha256) {
                    store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, file.length(), pkg.etag, "missing-final", clock())
                    return@withLock false
                }
            }
            publishAndActivateTransactionalLocked(generation, manifestVersion, fresh)
            val active = store.activeGeneration() ?: return@withLock false
            if (active.generation != generation) return@withLock false
            notifyGenerationActivated(generation, manifestVersion)
            cleanInactiveLocked(
                active.generation,
                manifestVersion,
                fresh.mapNotNullTo(mutableSetOf()) { publishedName(manifestVersion, it.fileName) },
            )
            true
        }
    }

    /**
     * Verified published files of the active generation in the native
     * `<dataVersion>/` layout the SDK scans. Empty when nothing active is
     * fully published.
     */
    suspend fun activeMapFiles(): List<File> = onIo {
        val state = publishedState() ?: return@onIo emptyList()
        if (!state.complete) return@onIo emptyList()
        val versionDir = nativeRoot.resolve(state.version)
        store.packages(state.generation).mapNotNull {
            publishedName(state.version, it.fileName)?.let { name -> versionDir.resolve(name) }
        }
    }

    data class MapPublishedState(
        val generation: String,
        val version: String,
        val expected: Int,
        val published: Int,
    ) {
        val complete: Boolean get() = expected > 0 && published == expected
    }

    /**
     * Checks published files of the active generation on [ioDispatcher].
     * Reuses verified stamps for unchanged files during UI polling while
     * recovering any uncommitted crash state first.
     */
    open suspend fun publishedState(forceVerify: Boolean = false): MapPublishedState? = onIo {
        publishMutex.withLock {
            recoverUncommittedPublishLocked()
        }
        val active = store.activeGeneration() ?: return@onIo null
        if (!isNativeDataVersion(active.dataVersion)) {
            return@onIo MapPublishedState(active.generation, active.dataVersion, 0, 0)
        }
        val packages = store.packages(active.generation)
        val versionDir = nativeRoot.resolve(active.dataVersion)
        var ok = 0
        for (pkg in packages) {
            coroutineContext.ensureActive()
            val name = publishedName(active.dataVersion, pkg.fileName) ?: continue
            val file = versionDir.resolve(name)
            if (isFileVerified(file, pkg.bytes, pkg.sha256, forceVerify)) {
                ok++
            }
        }
        MapPublishedState(active.generation, active.dataVersion, packages.size, ok)
    }

    /** Mimics the SDK's version-directory `.mwm` discovery for activation proof. */
    fun containsNativeMapData(): Boolean {
        val root = nativeRoot
        return root.listFiles()?.any { dir ->
            dir.isDirectory && isNativeDataVersion(dir.name) &&
                dir.listFiles()?.any { it.isFile && it.extension == "mwm" } == true
        } == true
    }

    suspend fun deleteGeneration(generation: String): Boolean = onIo {
        publishMutex.withLock {
            if (store.activeGeneration()?.generation == generation) return@withLock false
            if (hasGenerationLease(generation)) return@withLock false
            store.deleteInactive(generation)
            generationDir(generation).deleteRecursively()
            true
        }
    }

    private suspend fun downloadPackage(
        generation: String,
        pkg: MapPackageEntity,
        baseUrl: String,
        onProgress: (String, Long, Long) -> Unit,
    ): Boolean {
        val dir = generationDir(generation).apply { mkdirs() }
        val part = File(dir, "${pkg.fileName}.part")
        part.parentFile?.mkdirs()
        val final = finalFile(generation, pkg)
        if (final.isFile && final.length() == pkg.bytes && sha256(final) == pkg.sha256) {
            store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_READY, pkg.bytes, pkg.etag, null, clock())
            return true
        }
        // Drop stale publish leftovers; they are never valid content.
        File(dir, "${pkg.fileName}.publish-part").delete()
        if (part.length() > pkg.bytes) part.delete()
        val needed = pkg.bytes - part.length()
        if (needed > 0 && freeBytes(dir) - needed < reserveBytes) {
            throw MapStorageException("map-storage-full")
        }
        val resumedFrom = part.length()
        val request = Request.Builder()
            .url("${baseUrl.trimEnd('/')}/v1/map/packages/${pkg.packageId}")
            .apply {
                if (resumedFrom > 0) {
                    header("Range", "bytes=$resumedFrom-")
                    pkg.etag?.let { header("If-Range", it) }
                }
            }
            .build()
        store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_DOWNLOADING, part.length(), pkg.etag, null, clock())
        val call = http.newCall(request)
        val completionHandle = coroutineContext[Job]?.invokeOnCompletion { cause ->
            if (cause is CancellationException) {
                call.cancel()
            }
        }
        try {
            call.execute().use { response ->
                if (response.code == 416) {
                    part.delete()
                    store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, 0, pkg.etag, "range-mismatch", clock())
                    throw MapDownloadException("package-range-mismatch")
                }
                if (response.code == 206) {
                    if (resumedFrom > 0) checkResumeRange(response.header("Content-Range"), resumedFrom, pkg.bytes)
                } else if (response.isSuccessful) {
                    if (resumedFrom > 0) {
                        // Server ignored Range and resent the whole file:
                        // re-check space for the full size before writing.
                        if (freeBytes(dir) - pkg.bytes < reserveBytes) {
                            throw MapStorageException("map-storage-full")
                        }
                    }
                    part.delete()
                } else {
                    throw MapDownloadException("http-${response.code}")
                }
                val etag = response.header("ETag") ?: pkg.etag
                val context = coroutineContext
                response.body?.byteStream()?.use { input ->
                    FileOutputStream(part, true).use { output ->
                        val buffer = ByteArray(256 * 1024)
                        var downloaded = part.length()
                        while (true) {
                            context.ensureActive()
                            val read = input.read(buffer)
                            if (read < 0) break
                            downloaded += read
                            // Enforce the signed size cap during the transfer,
                            // not only after EOF.
                            if (downloaded > pkg.bytes) {
                                throw MapDownloadException("package-too-large")
                            }
                            output.write(buffer, 0, read)
                            onProgress(pkg.packageId, downloaded, pkg.bytes)
                        }
                    }
                }
                if (part.length() != pkg.bytes || sha256(part) != pkg.sha256) {
                    part.delete()
                    store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, 0, etag, "digest-mismatch", clock())
                    throw MapDownloadException("package-digest-mismatch")
                }
                atomicPublish(part, final)
                part.delete()
                store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_READY, pkg.bytes, etag, null, clock())
                return true
            }
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: MapStorageException) {
            store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, part.length(), pkg.etag, error.message, clock())
            throw error
        } catch (error: MapDownloadException) {
            // Oversize, digest and range failures invalidate the staged bytes:
            // drop the part so the next attempt restarts cleanly instead of
            // reusing bytes a failing server already disowned.
            if (error.message == "package-too-large" || error.message == "package-digest-mismatch" ||
                error.message == "package-range-mismatch"
            ) {
                part.delete()
            }
            store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, part.length(), pkg.etag, error.message, clock())
            throw error
        } catch (error: IOException) {
            coroutineContext.ensureActive()
            store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, part.length(), pkg.etag, "network", clock())
            throw error
        } finally {
            completionHandle?.dispose()
        }
    }

    /**
     * Exposes a complete file atomically: bytes land in a sibling
     * `.publish-part` and only a verified rename makes them visible. A crash
     * mid-copy can never present a partial file as published.
     */
    private suspend fun atomicPublish(source: File, destination: File) {
        destination.parentFile?.mkdirs()
        val tmp = File(destination.parent, "${destination.name}.publish-part")
        tmp.delete()
        copyFileCancellable(source, tmp)
        if (!tmp.renameTo(destination)) {
            tmp.delete()
            throw MapStorageException("map-publish-failed")
        }
    }

    private data class StagedPublishItem(
        val pkg: MapPackageEntity,
        val tmp: File,
        val dst: File,
    )

    /**
     * Stages all candidate files first, journals pre-existing native files in
     * `<versionDir>/.rollback`, swaps all files into place, and commits the
     * Room active generation. Any failure during file swap or Room activation
     * restores the backed-up native files immediately.
     */
    private suspend fun publishAndActivateTransactionalLocked(
        generation: String,
        version: String,
        packages: List<MapPackageEntity>,
    ) {
        val versionDir = nativeRoot.resolve(version).apply { mkdirs() }
        // Step 1: Stage and verify all .publish-part files before modifying any live .mwm file.
        val staged = mutableListOf<StagedPublishItem>()
        try {
            for (pkg in packages) {
                coroutineContext.ensureActive()
                val name = publishedName(version, pkg.fileName)
                    ?: throw MapDownloadException("unsupported-package-layout")
                val src = finalFile(generation, pkg)
                val dst = versionDir.resolve(name)
                val tmp = File(versionDir, "${dst.name}.publish-part")
                tmp.delete()
                copyFileCancellable(src, tmp)
                if (tmp.length() != pkg.bytes || sha256(tmp) != pkg.sha256) {
                    tmp.delete()
                    throw MapDownloadException("package-publish-mismatch")
                }
                staged += StagedPublishItem(pkg, tmp, dst)
            }
        } catch (t: Throwable) {
            staged.forEach { it.tmp.delete() }
            throw t
        }

        // Step 2: Record rollback journal of existing files in versionDir before renaming.
        val rollbackDir = versionDir.resolve(".rollback")
        rollbackDir.deleteRecursively()
        val backupDir = rollbackDir.resolve("backup").apply { mkdirs() }
        val createdNames = mutableListOf<String>()
        try {
            rollbackDir.resolve("generation.txt").writeText(generation)
            for (item in staged) {
                if (item.dst.isFile) {
                    copyFileCancellable(item.dst, backupDir.resolve(item.dst.name))
                } else {
                    createdNames += item.dst.name
                }
            }
            rollbackDir.resolve("created.txt").writeText(createdNames.joinToString("\n"))
        } catch (t: Throwable) {
            staged.forEach { it.tmp.delete() }
            rollbackDir.deleteRecursively()
            throw t
        }

        // Step 3: Rename all staged files and commit Room activation; rollback on any failure.
        try {
            for (item in staged) {
                publishRenameHook?.invoke(item.pkg.packageId, item.tmp, item.dst)
                if (!item.tmp.renameTo(item.dst)) {
                    throw MapStorageException("map-publish-failed")
                }
            }
            store.activate(generation, clock())
        } catch (t: Throwable) {
            staged.forEach { it.tmp.delete() }
            restoreRollbackLocked(versionDir, rollbackDir)
            throw t
        }

        // Step 4: Room committed; remove rollback journal and record verified stamps.
        rollbackDir.deleteRecursively()
        for (item in staged) {
            recordVerifiedStamp(item.dst, item.pkg.sha256)
        }
    }

    private suspend fun recoverUncommittedPublishLocked() {
        val root = nativeRoot
        if (!root.isDirectory) return
        val active = store.activeGeneration()
        root.listFiles()?.forEach { versionDir ->
            if (!versionDir.isDirectory || !isNativeDataVersion(versionDir.name)) return@forEach
            val rollbackDir = versionDir.resolve(".rollback")
            if (!rollbackDir.isDirectory) return@forEach
            val uncommittedGen = rollbackDir.resolve("generation.txt")
                .takeIf { it.isFile }
                ?.readText()
                ?.trim()
            if (uncommittedGen == null || active?.generation != uncommittedGen) {
                restoreRollbackLocked(versionDir, rollbackDir)
            } else {
                // Room committed the activation before process death; clean up journal & parts.
                versionDir.listFiles()?.forEach { child ->
                    if (child.isFile && child.name.endsWith(".publish-part")) {
                        child.delete()
                    }
                }
                rollbackDir.deleteRecursively()
            }
        }
    }

    private suspend fun restoreRollbackLocked(versionDir: File, rollbackDir: File) {
        verifiedStamps.clear()
        val backupDir = rollbackDir.resolve("backup")
        val backedUpNames = mutableSetOf<String>()
        if (backupDir.isDirectory) {
            backupDir.listFiles()?.forEach { backupFile ->
                if (!backupFile.isFile) return@forEach
                backedUpNames += backupFile.name
                val target = versionDir.resolve(backupFile.name)
                val restoreTmp = versionDir.resolve("${backupFile.name}.restore-part")
                restoreTmp.delete()
                copyFileCancellable(backupFile, restoreTmp)
                if (!restoreTmp.renameTo(target)) {
                    copyFileCancellable(backupFile, target)
                    restoreTmp.delete()
                }
            }
        }
        val createdFile = rollbackDir.resolve("created.txt")
        if (createdFile.isFile) {
            createdFile.readLines()
                .map { it.trim() }
                .filter { it.isNotEmpty() && it !in backedUpNames }
                .forEach { createdName ->
                    versionDir.resolve(createdName).delete()
                }
        }
        versionDir.listFiles()?.forEach { child ->
            if (child.isFile && (child.name.endsWith(".publish-part") || child.name.endsWith(".restore-part"))) {
                child.delete()
            }
        }
        rollbackDir.deleteRecursively()
    }

    private suspend fun notifyGenerationActivated(generation: String, dataVersion: String) {
        onGenerationActivated(generation, dataVersion)
        for (listener in activationListeners) {
            listener(generation, dataVersion)
        }
    }

    private suspend fun cleanInactiveLocked(
        activeGeneration: String,
        activeVersion: String,
        expectedPaths: Set<String>,
    ) {
        val leasedGens = leasedGenerationsSnapshot()
        val leasedVersions = mutableSetOf<String>()
        val leasedSameVersionFiles = mutableSetOf<String>()
        for (leasedGen in leasedGens) {
            val v = store.generation(leasedGen)?.dataVersion.orEmpty()
            if (isNativeDataVersion(v)) {
                leasedVersions += v
                if (v == activeVersion) {
                    store.packages(leasedGen).forEach { pkg ->
                        publishedName(v, pkg.fileName)?.let { leasedSameVersionFiles += it }
                    }
                }
            }
        }

        // Staging retention: keep the active generation, any leased generation,
        // and any generation with a non-terminal (PENDING/DOWNLOADING) package
        // so a concurrent download is never deleted.
        val root = filesRoot.resolve("map-packages")
        if (root.isDirectory) {
            root.listFiles()?.forEach { dir ->
                if (!dir.isDirectory || dir.name == activeGeneration || dir.name in leasedGens) return@forEach
                val pkgs = store.packages(dir.name)
                val inProgress = pkgs.any { it.status == PACKAGE_PENDING || it.status == PACKAGE_DOWNLOADING }
                if (pkgs.isEmpty() || !inProgress) dir.deleteRecursively()
            }
        }
        // Native version dirs: drop versions that are neither active nor leased.
        nativeRoot.listFiles()?.forEach { dir ->
            if (dir.isDirectory && isNativeDataVersion(dir.name) &&
                dir.name != activeVersion && dir.name !in leasedVersions
            ) {
                dir.deleteRecursively()
            }
        }
        // Stale files inside the active version dir (keep active + leased files).
        val retainedNames = expectedPaths + leasedSameVersionFiles
        val versionDir = nativeRoot.resolve(activeVersion)
        if (versionDir.isDirectory) {
            versionDir.walkTopDown().filter { it.isFile }.forEach { file ->
                val relative = file.relativeTo(versionDir).invariantSeparatorsPath
                if (relative !in retainedNames) {
                    verifiedStamps.remove(file.absolutePath)
                    file.delete()
                }
            }
        }
    }

    private fun generationDir(generation: String): File {
        if (generation == "." || generation == ".." ||
            !generation.matches(Regex("^[A-Za-z0-9._-]{1,128}$"))
        ) {
            throw MapContractException("invalid-generation")
        }
        return filesRoot.resolve("map-packages").resolve(generation)
    }

    private fun finalFile(generation: String, pkg: MapPackageEntity): File {
        val name = pkg.fileName
        if (name.isBlank() || name.contains("..") || name.startsWith("/") || name.contains('\\')) {
            throw MapContractException("invalid-package-path")
        }
        return generationDir(generation).resolve(name)
    }

    private suspend fun copyFileCancellable(source: File, destination: File) {
        destination.parentFile?.mkdirs()
        source.inputStream().use { input ->
            FileOutputStream(destination, false).use { output ->
                val buffer = ByteArray(256 * 1024)
                while (true) {
                    coroutineContext.ensureActive()
                    val read = input.read(buffer)
                    if (read < 0) break
                    output.write(buffer, 0, read)
                }
                output.fd.sync()
            }
        }
    }

    private suspend fun isFileVerified(
        file: File,
        expectedBytes: Long,
        expectedSha256: String,
        forceVerify: Boolean,
    ): Boolean {
        if (!file.isFile) return false
        val length = file.length()
        if (length != expectedBytes) return false
        val key = file.absolutePath
        val lastModified = file.lastModified()
        if (!forceVerify) {
            val cached = verifiedStamps[key]
            if (cached != null &&
                cached.length == length &&
                cached.lastModified == lastModified &&
                cached.fullSha256 == expectedSha256 &&
                cached.sampleSha256 == sampleSha256(file, length)
            ) {
                return true
            }
        }
        val actualSha = sha256(file)
        if (actualSha != expectedSha256) {
            verifiedStamps.remove(key)
            return false
        }
        recordVerifiedStamp(file, actualSha)
        return true
    }

    private fun recordVerifiedStamp(file: File, fullSha256: String) {
        if (!file.isFile) return
        val length = file.length()
        val lastModified = file.lastModified()
        val sample = sampleSha256(file, length)
        verifiedStamps[file.absolutePath] = VerifiedFileStamp(
            length = length,
            lastModified = lastModified,
            sampleSha256 = sample,
            fullSha256 = fullSha256,
        )
    }

    private fun sampleSha256(file: File, length: Long): String {
        val digest = MessageDigest.getInstance("SHA-256")
        val chunkSize = 4096
        RandomAccessFile(file, "r").use { raf ->
            val head = ByteArray(minOf(length, chunkSize.toLong()).toInt())
            val headRead = raf.read(head)
            if (headRead > 0) digest.update(head, 0, headRead)
            if (length > chunkSize) {
                val tailOffset = maxOf(chunkSize.toLong(), length - chunkSize)
                raf.seek(tailOffset)
                val tail = ByteArray((length - tailOffset).toInt())
                val tailRead = raf.read(tail)
                if (tailRead > 0) digest.update(tail, 0, tailRead)
            }
        }
        return digest.digest().joinToString("") { "%02x".format(it) }
    }

    private suspend fun sha256(file: File): String {
        fullHashCounter.incrementAndGet()
        val digest = MessageDigest.getInstance("SHA-256")
        file.inputStream().use { input ->
            val buffer = ByteArray(256 * 1024)
            while (true) {
                coroutineContext.ensureActive()
                val read = input.read(buffer)
                if (read < 0) break
                digest.update(buffer, 0, read)
            }
        }
        return digest.digest().joinToString("") { "%02x".format(it) }
    }
}
