package com.anpfuel.data.remote

import android.content.Context
import com.anpfuel.domain.model.PriceTable
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.io.IOException
import java.security.MessageDigest
import javax.inject.Inject

/**
 * Streams ANP XLSX files into the app cache directory (UC-001).
 */
class AnpFileDownloader private constructor(
    private val downloadDirectory: File,
    private val okHttpClient: OkHttpClient,
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

        val request = Request.Builder()
            .url(sourceUrl)
            .get()
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            if (!response.isSuccessful) {
                throw IOException("ANP download failed with HTTP ${response.code} for $sourceUrl")
            }

            val body = requireNotNull(response.body) { "ANP download response body was empty for $sourceUrl" }
            val targetFile = File(downloadDirectory, resolveFileName(sourceUrl))

            body.byteStream().use { input ->
                targetFile.outputStream().use { output ->
                    input.copyTo(output)
                }
            }

            validateDownloadedFile(
                file = targetFile,
                contentType = response.header("Content-Type"),
                sourceUrl = sourceUrl,
            )

            AnpDownloadResult(
                file = targetFile,
                sourceUrl = sourceUrl,
                contentType = response.header("Content-Type"),
                sizeBytes = targetFile.length(),
                sha256 = sha256(targetFile),
            )
        }
    }

    internal fun validateDownloadedFile(
        file: File,
        contentType: String?,
        sourceUrl: String,
    ) {
        if (file.length() <= 0L) {
            throw IOException("Downloaded file is empty: ${file.name}")
        }

        val extension = file.extension.ifBlank { sourceUrl.substringAfterLast('.', "") }
        if (!extension.equals(XLSX_EXTENSION, ignoreCase = true)) {
            throw IOException("Downloaded file must have .$XLSX_EXTENSION extension: ${file.name}")
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

    private fun resolveFileName(sourceUrl: String): String =
        sourceUrl.substringAfterLast('/').ifBlank { "anp-download.$XLSX_EXTENSION" }

    companion object {
        const val DOWNLOAD_DIR = "anp"
        private const val XLSX_EXTENSION = "xlsx"

        val ACCEPTED_CONTENT_TYPES = setOf(
            "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
            "application/octet-stream",
            "application/vnd.ms-excel",
            "binary/octet-stream",
        )

        internal fun forDirectory(
            downloadDirectory: File,
            okHttpClient: OkHttpClient,
        ): AnpFileDownloader = AnpFileDownloader(
            downloadDirectory = downloadDirectory.apply { mkdirs() },
            okHttpClient = okHttpClient,
        )
    }
}
