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
}
