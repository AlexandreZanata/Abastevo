package com.anpfuel.data.map

import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import com.anpfuel.domain.map.MapContractException
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.security.MessageDigest
import kotlinx.coroutines.ensureActive
import kotlin.coroutines.coroutineContext
import okhttp3.OkHttpClient
import okhttp3.Request

class MapDownloadException(message: String, cause: Throwable? = null) : Exception(message, cause)

class MapStorageException(message: String) : Exception(message)

class MapPackageRepository(
    private val store: MapPackageStore,
    private val filesRoot: File,
    private val http: OkHttpClient,
    private val clock: () -> Long = { System.currentTimeMillis() },
    private val freeBytes: (File) -> Long = { it.usableSpace },
    private val reserveBytes: Long = 32L * 1024 * 1024,
) {
    suspend fun importManifest(
        envelopeJson: String,
        trustedKeys: Map<String, java.security.PublicKey>,
    ): VerifiedMapManifest {
        val verified = MapManifestVerifier.verify(envelopeJson, trustedKeys)
        val manifest = verified.manifest
        if (store.generation(manifest.generation)?.active == true) {
            throw MapDownloadException("active-generation-reimport")
        }
        store.insertGeneration(
            MapGenerationEntity(
                generation = manifest.generation,
                manifestSha256 = verified.manifestSha256,
                active = false,
                activatedAtMillis = null,
            ),
            manifest.packageRows(manifest.generation, clock()),
        )
        return verified
    }

    suspend fun downloadMissing(
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

    suspend fun activate(generation: String): Boolean {
        val packages = store.packages(generation)
        if (packages.isEmpty() || packages.any { it.status != PACKAGE_READY }) return false
        store.activate(generation, clock())
        val active = store.activeGeneration() ?: return false
        cleanInactive(active.generation)
        return true
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
        val final = finalFile(generation, pkg)
        if (final.isFile && final.length() == pkg.bytes && sha256(final) == pkg.sha256) {
            store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_READY, pkg.bytes, pkg.etag, null, clock())
            return true
        }
        if (part.length() > pkg.bytes) part.delete()
        val needed = pkg.bytes - part.length()
        if (needed > 0 && freeBytes(dir) - needed < reserveBytes) {
            throw MapStorageException("map-storage-full")
        }
        val request = Request.Builder()
            .url("${baseUrl.trimEnd('/')}/v1/map/packages/${pkg.packageId}")
            .apply {
                if (part.length() > 0) {
                    header("Range", "bytes=${part.length()}-")
                    pkg.etag?.let { header("If-Range", it) }
                }
            }
            .build()
        store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_DOWNLOADING, part.length(), pkg.etag, null, clock())
        try {
            http.newCall(request).execute().use { response ->
                if (!response.isSuccessful && response.code != 206) {
                    throw MapDownloadException("http-${response.code}")
                }
                val append = response.code == 206
                if (!append) part.delete()
                val context = coroutineContext
                response.body?.byteStream()?.use { input ->
                    FileOutputStream(part, true).use { output ->
                        val buffer = ByteArray(256 * 1024)
                        var downloaded = part.length()
                        while (true) {
                            context.ensureActive()
                            val read = input.read(buffer)
                            if (read < 0) break
                            output.write(buffer, 0, read)
                            downloaded += read
                            onProgress(pkg.packageId, downloaded, pkg.bytes)
                        }
                    }
                }
                val etag = response.header("ETag") ?: pkg.etag
                if (part.length() != pkg.bytes || sha256(part) != pkg.sha256) {
                    part.delete()
                    store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, 0, etag, "digest-mismatch", clock())
                    throw MapDownloadException("package-digest-mismatch")
                }
                if (!part.renameTo(final)) {
                    part.copyTo(final, overwrite = true)
                    part.delete()
                }
                store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_READY, pkg.bytes, etag, null, clock())
                return true
            }
        } catch (error: MapStorageException) {
            store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, part.length(), pkg.etag, error.message, clock())
            throw error
        } catch (error: MapDownloadException) {
            store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, part.length(), pkg.etag, error.message, clock())
            throw error
        } catch (error: IOException) {
            store.updatePackage(pkg.generation, pkg.packageId, PACKAGE_FAILED, part.length(), pkg.etag, "network", clock())
            throw error
        }
    }

    private suspend fun cleanInactive(activeGeneration: String) {
        val root = filesRoot.resolve("map-packages")
        if (!root.isDirectory) return
        root.listFiles()?.forEach { dir ->
            if (dir.name != activeGeneration) dir.deleteRecursively()
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
