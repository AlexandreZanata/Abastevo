package com.anpfuel.data.remote.occurrence

import com.anpfuel.data.local.quality.QualityPublicCache
import com.anpfuel.domain.occurrence.*
import com.anpfuel.domain.repository.OccurrenceGateway
import java.io.IOException
import java.time.LocalDate
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import okhttp3.*
import okhttp3.HttpUrl.Companion.toHttpUrl
import org.json.JSONArray
import org.json.JSONObject

/** Bounded, anonymous official occurrence reads over the accepted
 * enforcement APIs. Strict typed parsing with unknown-enum retention;
 * corrupt, oversize or foreign bodies fail closed without disclosure.
 * Only validated official bodies reach the shared public cache; private
 * reports are refused before any write. */
class OccurrenceHttpClient(http: OkHttpClient, baseUrl: String, private val cache: QualityPublicCache? = null) : OccurrenceGateway {
    private val origin = baseUrl.toHttpUrl()
    private val client = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).callTimeout(20, TimeUnit.SECONDS).build()

    override suspend fun records(uf: String?, municipality: String?, segment: String?, subjectKind: String?, fromDay: String?, toDay: String?, limit: Int, cursor: String?, revisionId: String?): OccurrencePage<OccurrenceRecord> {
        val params = mutableMapOf("limit" to limit.toString(), "cursor" to cursor, "revision_id" to revisionId,
            "uf" to uf, "municipality" to municipality, "segment" to segment, "subject_kind" to subjectKind,
            "from_day" to fromDay, "to_day" to toDay)
        val response = read("actions", params)
        return response.validated {
            val root = response.body
            val lease = root.lease(revisionId)
            val items = root.array("items", 100).map(::record)
            if (items.map { it.id }.distinct().size != items.size || (!lease.available && items.isNotEmpty())) invalid()
            OccurrencePage(items, root.cursor(cursor), lease, response.stale || lease.stale)
        }
    }

    override suspend fun recordCoverage(revisionId: String?): Pair<RecordCoverage, OccurrenceLease> {
        val response = read("actions/coverage", mapOf("revision_id" to revisionId))
        return response.validated {
            val root = response.body
            val lease = root.lease(revisionId)
            val result = RecordCoverage(root.count("observations"), root.count("cases"),
                root.optionalText("first_day"), root.optionalText("last_day"), root.text("date_basis"))
            if (result.dateBasis != "document_day" || result.cases > result.observations) invalid()
            result to lease
        }
    }

    override suspend fun stations(state: String?, segment: String?, fromDay: String?, toDay: String?, limit: Int, cursor: String?, revisionId: String?): OccurrencePage<OccurrenceStation> {
        val response = read("enforcement/stations", mapOf("limit" to limit.toString(), "cursor" to cursor,
            "revision_id" to revisionId, "state" to state, "segment" to segment,
            "from_day" to fromDay, "to_day" to toDay))
        return response.validated {
            val root = response.body
            val lease = root.lease(revisionId)
            val items = root.array("items", 100).map(::station)
            if (items.map { it.id }.distinct().size != items.size || (!lease.available && items.isNotEmpty())) invalid()
            OccurrencePage(items, root.cursor(cursor), lease, response.stale || lease.stale)
        }
    }

    override suspend fun stationCoverage(revisionId: String?): Pair<StationCoverage, OccurrenceLease> {
        val response = read("enforcement/stations/coverage", mapOf("revision_id" to revisionId))
        return response.validated {
            val root = response.body
            val lease = root.lease(revisionId)
            val result = StationCoverage(root.count("stations"), root.count("records"))
            if (result.records < result.stations) invalid()
            result to lease
        }
    }

    override suspend fun record(id: String, revisionId: String?): Pair<OccurrenceRecord, OccurrenceLease> {
        if (!Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}").matches(id)) throw OccurrenceReadUnavailable()
        val response = readDetail("actions", id, mapOf("revision_id" to revisionId))
        return response.validated {
            val root = response.body
            val lease = root.lease(revisionId)
            if (!lease.available) invalid()
            val result = record(root)
            if (result.id != id) invalid()
            result to lease
        }
    }

    private suspend fun <T> PublicRead.validated(parse: () -> T): T {
        val result = try { parse() } catch (e: Exception) { reject(); throw OccurrenceReadUnavailable() }
        accept()
        return result
    }

    private suspend fun readDetail(route: String, id: String, params: Map<String, String?>): PublicRead =
        read("$route/$id", params)

    private fun record(row: JSONObject): OccurrenceRecord {
        row.onlyKeys(setOf("id", "native_key", "scope", "status", "occurred_on", "as_of", "source_profile", "subject_kind", "uf", "municipality", "segment", "document_day", "date_basis", "document_number", "procedure", "result", "evidence_ref", "lease", "generated_at"))
        if (row.optString("source_profile", "") != "anp-enforcement-xlsx-2019-v1") invalid()
        return OccurrenceRecord(row.uuid("id"), row.text("native_key"), row.text("subject_kind"),
            row.text("uf"), row.text("municipality"), row.text("segment"), row.day("document_day"),
            row.text("date_basis"), row.text("document_number"), row.text("procedure"),
            row.text("result"), row.text("evidence_ref"))
    }

    private fun station(row: JSONObject): OccurrenceStation {
        return OccurrenceStation(row.uuid("id"), row.text("display_name"), row.optionalText("municipality"),
            row.optionalText("state"), row.text("status"), row.count("record_count"))
    }

    private data class PublicRead(val body: JSONObject, val stale: Boolean, val accept: suspend () -> Unit, val reject: suspend () -> Unit)
    private class TransportFailure : IOException()
    private suspend fun read(route: String, params: Map<String, String?>): PublicRead {
        val url = origin.newBuilder().addPathSegments("v1/quality/$route").apply {
            params.forEach { (key, value) -> value?.let { addQueryParameter(key, it) } }
        }.build()
        val cached = if (cache == null) null else withContext(Dispatchers.IO) { runCatching { cache.load(url.toString()) }.getOrNull() }
        var etag: String? = null
        var cacheable = true
        val body = try {
            suspendCancellableCoroutine<JSONObject> { continuation ->
                val request = Request.Builder().url(url).header("Accept", "application/json").header("Cache-Control", "no-cache")
                cached?.etag?.let { request.header("If-None-Match", it) }
                val call = client.newCall(request.get().build())
                continuation.invokeOnCancellation { call.cancel() }
                call.enqueue(object : Callback {
                    override fun onFailure(call: Call, e: IOException) {
                        if (continuation.isActive) continuation.resumeWithException(TransportFailure())
                    }
                    override fun onResponse(call: Call, response: Response) {
                        response.use {
                            if (!continuation.isActive) return
                            val parsed = try {
                                cacheable = !response.cacheControl.noStore
                                if (!cacheable) runCatching { cache?.remove(url.toString()) }
                                etag = if (response.code == 304) response.header("ETag") ?: cached?.etag else response.header("ETag")
                                if (response.code == 304 && cached != null) {
                                    continuation.resume(JSONObject(cached.body))
                                    return
                                }
                                if (response.code >= 500) throw TransportFailure()
                                if (!response.isSuccessful) throw OccurrenceReadUnavailable(response.code == 404)
                                val source = response.body?.source() ?: invalid()
                                if (source.request(1024L * 1024L + 1)) invalid()
                                JSONObject(source.readUtf8())
                            } catch (e: Exception) {
                                if (continuation.isActive) continuation.resumeWithException(when (e) { is OccurrenceReadUnavailable -> e; is IOException -> TransportFailure(); else -> OccurrenceReadUnavailable() })
                                return
                            }
                            continuation.resume(parsed)
                        }
                    }
                })
            }
        } catch (e: CancellationException) { throw e }
        catch (e: OccurrenceReadUnavailable) {
            withContext(Dispatchers.IO) { runCatching { cache?.clearOrigin(origin.resolve("/")!!.toString().trimEnd('/')) } }
            throw e
        } catch (_: TransportFailure) {
            val fallback = withContext(Dispatchers.IO) { runCatching { cache?.load(url.toString()) }.getOrNull() } ?: throw OccurrenceReadUnavailable()
            val stale = try { JSONObject(fallback.body) } catch (_: Exception) { throw OccurrenceReadUnavailable() }
            stale.optJSONObject("lease")?.put("stale", true)
            return PublicRead(stale, true, {}, { withContext(Dispatchers.IO) { cache?.remove(url.toString()); Unit } })
        }
        return PublicRead(body, false,
            { withContext(Dispatchers.IO) { runCatching { if (cacheable) cache?.save(url.toString(), body.toString(), etag) else cache?.remove(url.toString()) } }; Unit },
            { withContext(Dispatchers.IO) { cache?.remove(url.toString()); Unit } })
    }

    private fun JSONObject.lease(expected: String?): OccurrenceLease {
        val row = opt("lease") as? JSONObject ?: invalid()
        if (row.text("source") != "enforcement" || row.text("stream") != "enforcement-v1") invalid()
        val revision = row.optionalText("revision_id")
        if (expected != null && expected != revision) invalid()
        if (revision != null) row.uuid("revision_id")
        val available = row.bool("available")
        if (available && (revision == null || row.text("revision_status") !in setOf("published", "superseded"))) invalid()
        if (row.optionalText("provenance_state")?.let { it != "VERIFIED" } == true) invalid()
        return OccurrenceLease(
            revision = if (revision == null) null else SourceRevision("enforcement", "enforcement-v1", row.text("edition"), revision),
            available = available, stale = row.bool("stale"), coverageNote = row.optionalText("coverage_note").orEmpty())
    }

    private fun JSONObject.onlyKeys(allowed: Set<String>) {
        if (keys().asSequence().any { it !in allowed }) invalid()
    }
    private fun JSONObject.text(key: String): String = (opt(key) as? String)?.takeIf { it.length <= 65536 } ?: invalid()
    private fun JSONObject.optionalText(key: String): String? = if (!has(key) || isNull(key)) null else text(key)
    private fun JSONObject.bool(key: String): Boolean = opt(key) as? Boolean ?: invalid()
    private fun JSONObject.uuid(key: String): String = text(key).also { if (!Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}").matches(it)) invalid() }
    private fun JSONObject.day(key: String): String = text(key).also { try { if (LocalDate.parse(it).toString() != it) invalid() } catch (_: Exception) { invalid() } }
    private fun JSONObject.count(key: String): Long {
        val raw = opt(key)
        if (raw !is Int && raw !is Long) invalid()
        return (raw as Number).toLong().also { if (it < 0) invalid() }
    }
    private fun JSONObject.cursor(previous: String?): String? = optionalText("next_cursor")?.also { if (it.isBlank() || it.length > 8192 || it == previous) invalid() }
    private fun JSONObject.array(key: String, limit: Int): List<JSONObject> {
        val rows = opt(key) as? JSONArray ?: invalid()
        if (rows.length() > limit) invalid()
        return (0 until rows.length()).map { rows.opt(it) as? JSONObject ?: invalid() }
    }
    private fun invalid(): Nothing = throw OccurrenceReadUnavailable()
}

class OccurrenceReadUnavailable(val disabled: Boolean = false) : IOException()
