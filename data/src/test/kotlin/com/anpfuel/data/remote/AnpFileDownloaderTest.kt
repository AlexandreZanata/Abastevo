package com.anpfuel.data.remote

import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okio.Buffer
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.io.IOException
import java.nio.file.Path

class AnpFileDownloaderTest {

    @TempDir
    lateinit var tempDir: Path

    private lateinit var server: MockWebServer
    private lateinit var downloader: AnpFileDownloader

    private val summaryUrlPath =
        "/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
            "arquivos-lpc/2026/resumo_semanal_lpc_2026-06-07_2026-06-13.xlsx"

    @BeforeEach
    fun setUp() {
        server = MockWebServer()
        server.start()
        downloader = AnpFileDownloader.forDirectory(
            downloadDirectory = tempDir.toFile(),
            okHttpClient = OkHttpClient(),
            extraAllowedHosts = setOf("localhost"),
        )
    }

    @AfterEach
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun downloadsXlsxToCacheDirectoryWithSha256() = runBlocking {
        val payload = byteArrayOf(0x50, 0x4B, 0x03, 0x04, 0x01, 0x02)
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader(
                    "Content-Type",
                    "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
                )
                .setBody(Buffer().write(payload)),
        )

        val sourceUrl = server.url(summaryUrlPath).toString()
        val result = downloader.download(sourceUrl)

        assertTrue(result.file.exists())
        assertEquals(payload.size.toLong(), result.sizeBytes)
        assertEquals(
            "resumo_semanal_lpc_2026-06-07_2026-06-13.xlsx",
            result.file.name,
        )
        assertEquals(64, result.sha256.length)
        assertEquals(result.sha256, downloader.sha256(result.file))
    }

    @Test
    fun downloadsUsingPriceTableMetadata() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/octet-stream")
                .setBody(Buffer().write(byteArrayOf(1, 2, 3))),
        )

        val priceTable = PriceTable.create(
            surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
            tableType = PriceTableType.WEEKLY_SUMMARY,
            sourceUrl = server.url(summaryUrlPath).toString(),
        )

        val result = downloader.download(priceTable)

        assertEquals(priceTable.sourceUrl, result.sourceUrl)
        assertTrue(result.file.name.endsWith(".xlsx"))
    }

    @Test
    fun rejectsEmptyDownloadBody() {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/octet-stream"),
        )

        assertThrows(IOException::class.java) {
            runBlocking {
                downloader.download(server.url(summaryUrlPath).toString())
            }
        }
    }

    @Test
    fun rejectsUnsupportedContentType() {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "text/html")
                .setBody("not-xlsx"),
        )

        assertThrows(IOException::class.java) {
            runBlocking {
                downloader.download(server.url(summaryUrlPath).toString())
            }
        }
    }

    // SEC06-A: foreign, private, plaintext and userinfo destinations
    // refuse before any fetch (zero recorded requests).
    @Test
    fun refusesUnapprovedOriginsWithoutFetching() = runBlocking {
        val destinations = listOf(
            "https://example.com" + summaryUrlPath,
            "https://127.0.0.1:9" + summaryUrlPath,
            "https://user:pass@localhost:" + server.port + summaryUrlPath,
        )
        for (url in destinations) {
            try {
                downloader.download(url)
                throw AssertionError("fetched $url")
            } catch (_: IllegalArgumentException) {
            }
        }
        assertEquals(0, server.requestCount)
    }

    // SEC06-A: redirect chains re-check the origin — a bounce to a
    // dead/plaintext destination refuses with no file left behind.
    @Test
    fun refusesRedirectLeavingApprovedOrigins() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(301)
                .setHeader("Location", "http://127.0.0.1:9" + summaryUrlPath),
        )
        try {
            downloader.download(server.url(summaryUrlPath).toString())
            throw AssertionError("redirect fetched")
        } catch (_: IOException) {
        }
        val leftovers = tempDir.toFile().walkTopDown().filter { it.isFile }.toList()
        assertTrue(leftovers.isEmpty(), "redirect must leave no file: $leftovers")
    }

    // SEC06-A: transfers cap during the copy (a JSON maximum alone
    // would cap nothing) and partial files purge atomically.
    @Test
    fun refusesOversizedTransferAndPurgesPartial() = runBlocking {
        val over = ByteArray((AnpFileDownloader.MAX_DOWNLOAD_BYTES + 1).toInt()) { 0x7f }
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/octet-stream")
                .setBody(Buffer().write(over)),
        )
        try {
            downloader.download(server.url(summaryUrlPath).toString())
            throw AssertionError("oversize fetched")
        } catch (_: IOException) {
        }
        val leftovers = tempDir.toFile().walkTopDown().filter { it.isFile }.toList()
        assertTrue(leftovers.isEmpty(), "oversize must leave no file: $leftovers")
    }

    // SEC06-A: query/traversal-like names sanitize-or-refuse without
    // fetching, and failures never land outside the directory.
    @Test
    fun sanitizesUnsafeFileNames() = runBlocking {
        val base = server.url("/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/").toString()
            .removeSuffix("/")
        val traversal = base + "/arquivos-lpc/2026/x.xlsx/.."
        try {
            downloader.download(traversal)
            throw AssertionError("traversal fetched")
        } catch (_: IllegalArgumentException) {
        }
        assertEquals(0, server.requestCount)
        val withQuery = base + "/arquivos-lpc/2026/revendas_lpc_2026-06-07_2026-06-13.xlsx?token=abc&x=1"
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/octet-stream")
                .setBody(Buffer().write(byteArrayOf(1, 2, 3))),
        )
        val result = downloader.download(withQuery)
        assertEquals("revendas_lpc_2026-06-07_2026-06-13.xlsx", result.file.name)
        assertTrue(result.file.parentFile == tempDir.toFile())
    }

    // SEC06-A: same-origin redirects still complete (origin policy
    // follows the chain, it does not forbid it).
    @Test
    fun followsSameOriginRedirect() = runBlocking {
        val target = MockWebServer()
        target.start()
        try {
            target.enqueue(
                MockResponse()
                    .setResponseCode(200)
                    .setHeader("Content-Type", "application/octet-stream")
                    .setBody(Buffer().write(byteArrayOf(9, 9, 9))),
            )
            server.enqueue(
                MockResponse()
                    .setResponseCode(301)
                    .setHeader("Location", target.url(summaryUrlPath).toString()),
            )
            val result = downloader.download(server.url(summaryUrlPath).toString())
            assertEquals(3L, result.sizeBytes)
        } finally {
            target.shutdown()
        }
    }
}
