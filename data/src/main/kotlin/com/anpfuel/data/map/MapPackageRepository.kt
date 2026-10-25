package com.anpfuel.data.map

import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import com.anpfuel.domain.map.MapContractException
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.security.MessageDigest
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlin.coroutines.coroutineContext
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

open class MapPackageRepository(
    private val store: MapPackageStore,
    private val filesRoot: File,
    private val http: OkHttpClient,
    private val clock: () -> Long = { System.currentTimeMillis() },
    private val freeBytes: (File) -> Long = { it.usableSpace },
    private val reserveBytes: Long = 32L * 1024 * 1024,
    private val nativeRoot: File = filesRoot,
) {
    private val publishMutex = Mutex()

    open suspend fun importManifestWithPlatformKeys(
        envelopeJson: String,
        trustedKeys: Map<String, java.security.PublicKey>,
    ): VerifiedMapManifest {
        val verified = MapManifestVerifier.verify(envelopeJson, trustedKeys)
        return insertVerified(verified)
    }

    /**
     * Production import against raw 32-byte pinned keys (works without the
     * platform Ed25519 provider). Unknown key ids and wrong key sizes fail
     * closed before any row is written.
     */
    open suspend fun importManifest(
        envelopeJson: String,
        trustedKeys: Map<String, ByteArray>,
    ): VerifiedMapManifest {
        val verified = MapManifestVerifier.verify(envelopeJson, trustedKeys)
        return insertVerified(verified)
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
    ): Boolean {
        val packages = store.packages(generation)
        if (packages.isEmpty()) throw MapDownloadException("unknown-generation")
        var allReady = true
        for (pkg in packages) {
            coroutineContext.ensureActive()
            if (pkg.status == PACKAGE_READY && finalFile(generation, pkg).let { it.isFile && it.length() == pkg.bytes && sha256(it) == pkg.sha256 }) {
                continue
            }
            allReady = false
            downloadPackage(generation, pkg, baseUrl, onProgress) || return false
        }
        return allReady || store.packages(generation).all { it.status == PACKAGE_READY }
    }

    open suspend fun activate(generation: String): Boolean {
        val packages = store.packages(generation)
        if (packages.isEmpty() || packages.any { it.status != PACKAGE_READY }) return false
        val manifestVersion = store.generation(generation)?.dataVersion.orEmpty()
        if (!isNativeDataVersion(manifestVersion)) throw MapDownloadException("unsupported-data-version")
        return publishMutex.withLock {
            // Re-verify under the mutex: a concurrent activation may have
            // recycled this staging area after the pre-check above.
            val fresh = store.packages(generation)
            if (fresh.isEmpty() || fresh.any { it.status != PACKAGE_READY }) return@withLock false
            for (pkg in fresh) {
                val file = finalFile(generation, pkg)
                if (!file.isFile || file.length() != pkg.bytes || sha256(file) != pkg.sha256) {
                    store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, file.length(), pkg.etag, "missing-final", clock())
                    return@withLock false
                }
            }
            publishPackages(generation, manifestVersion, fresh)
            store.activate(generation, clock())
            val active = store.activeGeneration() ?: return@withLock false
            cleanInactive(
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
    suspend fun activeMapFiles(): List<File> {
        val state = publishedState() ?: return emptyList()
        if (!state.complete) return emptyList()
        val versionDir = nativeRoot.resolve(state.version)
        return store.packages(state.generation).mapNotNull {
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

    /** Read-only publish check: no writes, safe to call from UI refresh. */
    open suspend fun publishedState(): MapPublishedState? {
        val active = store.activeGeneration() ?: return null
        if (!isNativeDataVersion(active.dataVersion)) {
            return MapPublishedState(active.generation, active.dataVersion, 0, 0)
        }
        val packages = store.packages(active.generation)
        val versionDir = nativeRoot.resolve(active.dataVersion)
        var ok = 0
        for (pkg in packages) {
            val name = publishedName(active.dataVersion, pkg.fileName) ?: continue
            val file = versionDir.resolve(name)
            if (file.isFile && file.length() == pkg.bytes && sha256(file) == pkg.sha256) ok++
        }
        return MapPublishedState(active.generation, active.dataVersion, packages.size, ok)
    }

    /** Mimics the SDK's version-directory `.mwm` discovery for activation proof. */
    fun containsNativeMapData(): Boolean {
        val root = nativeRoot
        return root.listFiles()?.any { dir ->
            dir.isDirectory && isNativeDataVersion(dir.name) &&
                dir.listFiles()?.any { it.isFile && it.extension == "mwm" } == true
        } == true
    }

    suspend fun deleteGeneration(generation: String): Boolean {
        if (store.activeGeneration()?.generation == generation) return false
        store.deleteInactive(generation)
        generationDir(generation).deleteRecursively()
        return true
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
        try {
            http.newCall(request).execute().use { response ->
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
            store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, part.length(), pkg.etag, "network", clock())
            throw error
        }
    }

    /**
     * Exposes a complete file atomically: bytes land in a sibling
     * `.publish-part` and only a verified rename makes them visible. A crash
     * mid-copy can never present a partial file as published.
     */
    private fun atomicPublish(source: File, destination: File) {
        destination.parentFile?.mkdirs()
        val tmp = File(destination.parent, "${destination.name}.publish-part")
        tmp.delete()
        source.copyTo(tmp, overwrite = true)
        if (!tmp.renameTo(destination)) {
            tmp.delete()
            throw MapStorageException("map-publish-failed")
        }
    }

    private suspend fun publishPackages(generation: String, version: String, packages: List<MapPackageEntity>) {
        val versionDir = nativeRoot.resolve(version).apply { mkdirs() }
        for (pkg in packages) {
            val name = publishedName(version, pkg.fileName)
                ?: throw MapDownloadException("unsupported-package-layout")
            val src = finalFile(generation, pkg)
            val dst = versionDir.resolve(name)
            if (dst.isFile && dst.length() == pkg.bytes && sha256(dst) == pkg.sha256) continue
            val tmp = File(versionDir, "${dst.name}.publish-part")
            tmp.delete()
            src.copyTo(tmp, overwrite = true)
            if (tmp.length() != pkg.bytes || sha256(tmp) != pkg.sha256) {
                tmp.delete()
                throw MapDownloadException("package-publish-mismatch")
            }
            if (!tmp.renameTo(dst)) {
                tmp.delete()
                throw MapStorageException("map-publish-failed")
            }
        }
    }

    private suspend fun cleanInactive(
        activeGeneration: String,
        activeVersion: String,
        expectedPaths: Set<String>,
    ) {
        // Staging retention minimum: keep the active generation and any
        // generation with a non-terminal (PENDING/DOWNLOADING) package so a
        // concurrent download is never deleted. Terminal inactive staging is
        // recycled after publication; re-activation re-downloads.
        val root = filesRoot.resolve("map-packages")
        if (root.isDirectory) {
            root.listFiles()?.forEach { dir ->
                if (!dir.isDirectory || dir.name == activeGeneration) return@forEach
                val pkgs = store.packages(dir.name)
                val inProgress = pkgs.any { it.status == PACKAGE_PENDING || it.status == PACKAGE_DOWNLOADING }
                if (pkgs.isEmpty() || !inProgress) dir.deleteRecursively()
            }
        }
        // Native version dirs: drop versions that are not active. Only
        // numeric directory names are ever touched.
        nativeRoot.listFiles()?.forEach { dir ->
            if (dir.isDirectory && isNativeDataVersion(dir.name) && dir.name != activeVersion) {
                dir.deleteRecursively()
            }
        }
        // Stale files inside the active version dir (never directories, never
        // anything outside the numeric dir).
        val versionDir = nativeRoot.resolve(activeVersion)
        if (versionDir.isDirectory) {
            versionDir.walkTopDown().filter { it.isFile }.forEach { file ->
                val relative = file.relativeTo(versionDir).invariantSeparatorsPath
                if (relative !in expectedPaths) file.delete()
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

    private fun sha256(file: File): String {
        val digest = MessageDigest.getInstance("SHA-256")
        file.inputStream().use { input ->
            val buffer = ByteArray(256 * 1024)
            while (true) {
                val read = input.read(buffer)
                if (read < 0) break
                digest.update(buffer, 0, read)
            }
        }
        return digest.digest().joinToString("") { "%02x".format(it) }
    }
}
