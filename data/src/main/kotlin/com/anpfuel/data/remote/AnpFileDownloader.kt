package com.anpfuel.data.remote

import android.content.Context
import com.anpfuel.domain.model.PriceTable
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.io.IOException
import java.security.MessageDigest
import javax.inject.Inject

/**
 * Streams ANP XLSX files into the app cache directory (UC-001).
 *
 * SEC06-A bounded fetch: approved origins only (re-checked after
 * redirects), capped transfer with atomic publish (partial files
 * purge on any failure), sanitized file names. A returned JSON
 * maximum alone would cap nothing — the cap enforces during the
 * copy, and the XLSX parser caps expansion again (SEC06-B).
 */
class AnpFileDownloader private constructor(
    private val downloadDirectory: File,
    private val okHttpClient: OkHttpClient,
    /**
     * Explicit test seam for offline suites: additional hosts
     * fetchable over any scheme (loopback MockWebServer). Production
     * wiring passes the default empty set (approved gov.br origins
     * over HTTPS only). Never populate from remote input.
     */
    private val extraAllowedHosts: Set<String> = emptySet(),
) {

    @Inject
    constructor(
        @ApplicationContext context: Context,
        okHttpClient: OkHttpClient,
    ) : this(File(context.cacheDir, DOWNLOAD_DIR).apply { mkdirs() }, okHttpClient)

    suspend fun download(priceTable: PriceTable): AnpDownloadResult =
        download(priceTable.sourceUrl)

    fun resolveDownloadedFile(sourceUrl: String): File {
        val fileName = resolveFileName(sourceUrl)
        return File(downloadDirectory, fileName)
    }

    suspend fun download(sourceUrl: String): AnpDownloadResult = withContext(Dispatchers.IO) {
        require(AnpPriceTableUrlParser.isPriceTableUrl(sourceUrl)) {
            "URL is not a supported ANP price table: $sourceUrl"
        }
        require(isFetchableUrl(sourceUrl)) {
            "URL is outside the approved ANP origins: $sourceUrl"
        }
        // File-name safety before any fetch: traversal-like names
        // refuse with zero requests.
        val targetFile = File(downloadDirectory, resolveFileName(sourceUrl))

        val request = Request.Builder()
            .url(sourceUrl)
            .get()
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            if (!response.isSuccessful) {
                throw IOException("ANP download failed with HTTP ${response.code} for $sourceUrl")
            }
            // Redirect-chain ownership: the final URL must satisfy the
            // same origin policy, so a listing compromise cannot bounce
            // the client to foreign/private/plaintext destinations.
            val finalUrl = response.request.url.toString()
            require(isFetchableUrl(finalUrl)) {
                "Redirected ANP download left the approved origins: $finalUrl"
            }

            val body = requireNotNull(response.body) { "ANP download response body was empty for $sourceUrl" }
            val stagingFile = File(downloadDirectory, targetFile.name + PART_SUFFIX)
            try {
                copyBounded(body.byteStream(), stagingFile)
                validateDownloadedFile(
                    file = stagingFile,
                    fileName = targetFile.name,
                    contentType = response.header("Content-Type"),
                    sourceUrl = sourceUrl,
                )
                if (!stagingFile.renameTo(targetFile)) {
                    throw IOException("ANP download publish failed: ${targetFile.name}")
                }
            } catch (e: Exception) {
                stagingFile.delete()
                // A failed validation must not leave a usable-looking
                // file behind either.
                if (e is IOException) throw e
                throw IOException("ANP download failed for $sourceUrl", e)
            }

            AnpDownloadResult(
                file = targetFile,
                sourceUrl = sourceUrl,
                contentType = response.header("Content-Type"),
                sizeBytes = targetFile.length(),
                sha256 = sha256(targetFile),
            )
        }
    }

    internal fun isFetchableUrl(rawUrl: String): Boolean {
        if (AnpEndpoints.isApprovedUrl(rawUrl)) return true
        if (extraAllowedHosts.isEmpty()) return false
        // Test seam: explicit loopback hosts only, and never with
        // embedded credentials (smuggled userinfo refuses everywhere).
        val url = rawUrl.trim().toHttpUrlOrNull() ?: return false
        if (url.username.isNotEmpty() || url.password.isNotEmpty()) return false
        return url.host in extraAllowedHosts
    }

    internal fun copyBounded(input: java.io.InputStream, stagingFile: File) {
        stagingFile.outputStream().use { output ->
            val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
            var total = 0L
            while (true) {
                val read = input.read(buffer)
                if (read < 0) break
                total += read
                if (total > MAX_DOWNLOAD_BYTES) {
                    throw IOException("ANP download exceeds $MAX_DOWNLOAD_BYTES bytes")
                }
                output.write(buffer, 0, read)
            }
            if (total <= 0L) {
                throw IOException("Downloaded file is empty: ${stagingFile.name}")
            }
        }
    }

    internal fun validateDownloadedFile(
        file: File,
        contentType: String?,
        sourceUrl: String,
        fileName: String = file.name,
    ) {
        if (file.length() <= 0L) {
            throw IOException("Downloaded file is empty: $fileName")
        }

        val extension = fileName.substringAfterLast('.', "")
            .ifBlank { sourceUrl.substringBefore('?').substringBefore('#').substringAfterLast('.', "") }
        if (!extension.equals(XLSX_EXTENSION, ignoreCase = true)) {
            throw IOException("Downloaded file must have .$XLSX_EXTENSION extension: $fileName")
        }

        if (!isAcceptedContentType(contentType)) {
            throw IOException("Unsupported ANP download content type: $contentType")
        }
    }

    internal fun isAcceptedContentType(contentType: String?): Boolean {
        if (contentType.isNullOrBlank()) {
            return true
        }
        val normalized = contentType.substringBefore(';').trim().lowercase()
        return normalized in ACCEPTED_CONTENT_TYPES
    }

    internal fun sha256(file: File): String {
        val digest = MessageDigest.getInstance("SHA-256")
        file.inputStream().use { input ->
            val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
            var read = input.read(buffer)
            while (read >= 0) {
                if (read > 0) {
                    digest.update(buffer, 0, read)
                }
                read = input.read(buffer)
            }
        }
        return digest.digest().joinToString(separator = "") { byte -> "%02x".format(byte) }
    }

    private fun resolveFileName(sourceUrl: String): String {
        // Strip query/fragment, then take the final path segment.
        // Traversal-like or blank names refuse before any fetch: partial
        // downloads must never land outside the download directory.
        val path = sourceUrl.trim().substringBefore('?').substringBefore('#')
        val segment = path.substringAfterLast('/').trim()
        require(segment.isNotBlank() && '/' !in segment && '\\' !in segment &&
            ".." !in segment && segment.all { it.code in 32..126 }
        ) {
            "ANP download file name is not safe: $sourceUrl"
        }
        return segment.ifBlank { "anp-download.$XLSX_EXTENSION" }
    }

    companion object {
        const val DOWNLOAD_DIR = "anp"
        private const val XLSX_EXTENSION = "xlsx"
        private const val PART_SUFFIX = ".part"
        /**
         * Transfer ceiling (SEC06-A): far above observed ANP weeklies
         * (single-digit MiB) while blocking unbounded amplification.
         * Expansion caps ride the XLSX parser (SEC06-B); the worker
         * verifies actual bytes again server-side.
         */
        const val MAX_DOWNLOAD_BYTES: Long = 64L * 1024L * 1024L

        val ACCEPTED_CONTENT_TYPES = setOf(
            "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
            "application/octet-stream",
            "application/vnd.ms-excel",
            "binary/octet-stream",
        )

        internal fun forDirectory(
            downloadDirectory: File,
            okHttpClient: OkHttpClient,
            extraAllowedHosts: Set<String> = emptySet(),
        ): AnpFileDownloader = AnpFileDownloader(
            downloadDirectory = downloadDirectory.apply { mkdirs() },
            okHttpClient = okHttpClient,
            extraAllowedHosts = extraAllowedHosts,
        )
    }
}
