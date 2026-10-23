package com.anpfuel.data.remote.occurrence

import com.anpfuel.data.local.quality.QualityPublicCache
import java.io.File
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
class OccurrenceHttpClientTest {
    private val revision = "eeeeeeee-6666-4666-8666-000000000001"
    private val recordId = "eeeeeeee-6666-4666-8666-000000000002"
    private val stationId = "eeeeeeee-5555-4555-8555-000000000001"
    private fun lease(rev: String = revision) = """{"source":"enforcement","stream":"enforcement-v1","edition":"enforcement-source-v1","revision_id":"$rev","available":true,"stale":false,"published_at":"2026-10-18T00:00:00Z","revision_status":"published","is_current_stream":true,"coverage_note":"checked source"}"""
    private fun record(id: String = recordId, kind: String = "company-shaped") = """{"id":"$id","native_key":"enforcement-source:abc","source_profile":"anp-enforcement-xlsx-2019-v1","subject_kind":"$kind","uf":"DF","municipality":"BRASILIA","segment":"Revenda de Combustíveis","document_day":"2026-10-04","date_basis":"document_day","document_number":"712508","procedure":"Inspection","result":"Resultado","evidence_ref":"synthetic.xlsx#row:2"}"""
    private fun page(next: String = "null") = """{"items":[${record()}],"lease":${lease()},"next_cursor":$next,"generated_at":"2026-10-18T00:00:00Z"}"""
    private fun stationPage() = """{"items":[{"id":"$stationId","display_name":"Posto DF-1","municipality":null,"state":"DF","status":"active","record_count":2}],"lease":${lease()},"next_cursor":null,"generated_at":"2026-10-18T00:00:00Z"}"""
    private fun coverage() = """{"observations":244247,"cases":141290,"first_day":"2019-01-02","last_day":"2026-10-04","date_basis":"document_day","lease":${lease()},"generated_at":"2026-10-18T00:00:00Z"}"""

    @Test fun `guest reads carry no credentials and unknown kinds are retained`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(page()))
            val api = OccurrenceHttpClient(OkHttpClient(), server.url("/").toString())
            val result = api.records(limit = 20, cursor = null)
            assertEquals(1, result.items.size)
            assertEquals("company-shaped", result.items.single().subjectKind)
            assertEquals("document_day", result.items.single().dateBasis)
            assertFalse(result.stale)
            val request = server.takeRequest(1, TimeUnit.SECONDS)
            assertNull(request?.getHeader("Authorization"))
            assertEquals("20", request?.requestUrl?.queryParameter("limit"))
            server.enqueue(MockResponse().setBody(page().replace("company-shaped", "FUTURE_KIND")))
            val future = api.records(limit = 20, cursor = null)
            assertEquals("FUTURE_KIND", future.items.single().subjectKind)
        }
    }

    @Test fun `foreign lease revision and repeated cursor fail closed`() = runTest {
        MockWebServer().use { server ->
            val api = OccurrenceHttpClient(OkHttpClient(), server.url("/").toString())
            server.enqueue(MockResponse().setBody(page().replace(revision, stationId)))
            assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.records(limit = 20, cursor = null, revisionId = revision) } }
            server.enqueue(MockResponse().setBody(page(next = "\"repeat\"")))
            assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.records(limit = 20, cursor = "repeat") } }
        }
    }

    @Test fun `oversize corrupt and private bodies never reach callers`() = runTest {
        MockWebServer().use { server ->
            val api = OccurrenceHttpClient(OkHttpClient(), server.url("/").toString())
            server.enqueue(MockResponse().setBody("x".repeat(1024 * 1024 + 1)))
            assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.records() } }
            server.enqueue(MockResponse().setBody("""{"items":[],"lease":${lease()},"next_cursor":null,"generated_at":"2026-10-18T00:00:00Z"}"""))
            assertEquals(0, runBlocking { api.records() }.items.size)
            server.enqueue(MockResponse().setBody("""{"items":[{"id":"not-a-uuid"}],"lease":${lease()}}"""))
            assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.records() } }
            // Private community report: refused, never cached or returned.
            server.enqueue(MockResponse().setBody("""{"items":[{"id":"$recordId","reporter":"user-123","price":5.99}],"lease":${lease()}}"""))
            assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.records() } }
        }
    }

    @Test fun `stations coverage and wrong origin fail closed`(@TempDir dir: File) = runTest {
        MockWebServer().use { server ->
            val cache = QualityPublicCache(File(dir, "cache"))
            val api = OccurrenceHttpClient(OkHttpClient(), server.url("/").toString(), cache)
            server.enqueue(MockResponse().setBody(stationPage()))
            val stations = api.stations(limit = 20, cursor = null)
            assertEquals("Posto DF-1", stations.items.single().displayName)
            assertEquals(2L, stations.items.single().recordCount)
            server.enqueue(MockResponse().setBody(coverage()))
            val (coverage, _) = api.recordCoverage()
            assertEquals(244247L, coverage.observations)
            assertEquals(141290L, coverage.cases)
            server.enqueue(MockResponse().setBody(stationPage().replace("enforcement", "pmqc")))
            assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.stations() } }
        }
    }

    @Test fun `cancellation stops transport without caching`(@TempDir dir: File) = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(page()).throttleBody(1, 5, TimeUnit.SECONDS))
            val cache = QualityPublicCache(File(dir, "cache"))
            val api = OccurrenceHttpClient(OkHttpClient(), server.url("/").toString(), cache)
            val job = launch { api.records() }
            job.cancel()
            job.join()
            assertTrue(job.isCancelled)
            assertNull(cache.load(server.url("/v1/quality/actions?limit=20").toString()))
        }
    }
    @Test fun `offline cache is explicitly stale`(@TempDir dir: File) = runTest {
        MockWebServer().use { server ->
            val api = OccurrenceHttpClient(OkHttpClient(), server.url("/").toString(), QualityPublicCache(File(dir, "cache")))
            server.enqueue(MockResponse().setBody(page()).setHeader("ETag", "v1"))
            api.records()
            server.enqueue(MockResponse().setResponseCode(503))
            val offline = api.records()
            assertTrue(offline.stale)
            assertTrue(offline.lease.stale)
        }
    }

    @Test fun `revocation and no-store evict previously cached evidence`(@TempDir dir: File) = runTest {
        MockWebServer().use { server ->
            for (status in listOf(200, 304, 400, 403, 404, 410)) {
                val api = OccurrenceHttpClient(OkHttpClient(), server.url("/prefix/").toString(), QualityPublicCache(File(dir, "cache$status")))
                server.enqueue(MockResponse().setBody(page()).setHeader("ETag", "v1"))
                api.records()
                server.enqueue(MockResponse().setResponseCode(status).setHeader("Cache-Control", "no-store").setBody(if (status == 200) page() else ""))
                if (status == 200 || status == 304) api.records()
                else assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.records() } }
                server.enqueue(MockResponse().setResponseCode(503))
                assertThrows(OccurrenceReadUnavailable::class.java, { runBlocking { api.records() } }, "status $status must revoke the old entry")
            }
        }
    }

    @Test fun `detail refuses a different record and mixed private payload`(@TempDir dir: File) = runTest {
        MockWebServer().use { server ->
            val cache = QualityPublicCache(File(dir, "cache"))
            val api = OccurrenceHttpClient(OkHttpClient(), server.url("/").toString(), cache)
            server.enqueue(MockResponse().setBody(record(stationId).dropLast(1) + ",\"lease\":" + lease() + "}"))
            assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.record(recordId) } }
            server.enqueue(MockResponse().setBody(page().replace("\"native_key\":", "\"reporter\":\"private-user\",\"native_key\":")))
            assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.records() } }
            assertNull(cache.load(server.url("/v1/quality/actions?limit=20").toString()))
        }
    }

    @Test fun `successful conditional validation and detail share the cache policy`(@TempDir dir: File) = runTest {
        MockWebServer().use { server ->
            val api = OccurrenceHttpClient(OkHttpClient(), server.url("/").toString(), QualityPublicCache(File(dir, "cache")))
            server.enqueue(MockResponse().setBody(record().dropLast(1) + ",\"lease\":" + lease() + "}").setHeader("ETag", "v1"))
            api.record(recordId)
            server.takeRequest(1, TimeUnit.SECONDS)
            server.enqueue(MockResponse().setResponseCode(304))
            assertEquals(recordId, api.record(recordId).first.id)
            assertEquals("v1", server.takeRequest(1, TimeUnit.SECONDS)?.getHeader("If-None-Match"))
            server.enqueue(MockResponse().setResponseCode(503))
            assertTrue(api.record(recordId).second.stale)
            server.enqueue(MockResponse().setResponseCode(410))
            assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.record(recordId) } }
            server.enqueue(MockResponse().setResponseCode(503))
            assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.record(recordId) } }
        }
    }

    @Test fun `withdrawn unavailable and invalid bodies revoke old snapshots`(@TempDir dir: File) = runTest {
        MockWebServer().use { server ->
            val api = OccurrenceHttpClient(OkHttpClient(), server.url("/").toString(), QualityPublicCache(File(dir, "cache")))
            for (bad in listOf(page().replace("published\"", "withdrawn\""), page().replace("\"available\":true", "\"available\":false"), page().replace("\"uf\":\"DF\"", "\"uf\":\"bad\""))) {
                server.enqueue(MockResponse().setBody(page()))
                api.records()
                server.enqueue(MockResponse().setBody(bad))
                assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.records() } }
                server.enqueue(MockResponse().setResponseCode(503))
                assertThrows(OccurrenceReadUnavailable::class.java) { runBlocking { api.records() } }
            }
        }
    }

    @Test fun `canonical Go source wire fields are accepted`() = runTest {
        val fixture = generateSequence(File(System.getProperty("user.dir"))) { it.parentFile }
            .map { File(it, "contracts/testdata/quality/enforcement-source-record-valid.json") }
            .first { it.isFile }.readText().trim()
        MockWebServer().use { server ->
            val api = OccurrenceHttpClient(OkHttpClient(), server.url("/").toString())
            server.enqueue(MockResponse().setBody(page().replace(record(), fixture)))
            assertEquals(recordId, api.records().items.single().id)
            server.enqueue(MockResponse().setBody(fixture.dropLast(1) + ",\"lease\":" + lease() + "}"))
            assertEquals(recordId, api.record(recordId).first.id)
        }
    }

}
