package com.anpfuel.data.remote.ledger

import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.repository.LedgerSyncConflict
import com.anpfuel.domain.repository.LedgerSyncDenied
import com.anpfuel.domain.repository.LedgerSyncGateway
import com.anpfuel.domain.repository.LedgerSyncReceipt
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import okhttp3.Call
import okhttp3.Callback
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import org.json.JSONArray
import org.json.JSONObject

private val BODY_BUDGET = 1024L * 1024L + 1
private val JSON = "application/json; charset=utf-8".toMediaType()

/**
 * MEX-14 private ledger transport (backend /v1/ledger/commands).
 *
 * One command per call with the live owner proof attached; no batching
 * (bounded payloads, bounded clock). 409/400 refuse without retry
 * (the held command is parked, never deleted); 401/403 keep a bounded
 * retry budget (proof or grant may arrive); outages and lost replies
 * retry idempotently with the same operation id. No-store everywhere.
 */
class LedgerSyncHttpClient(
    http: OkHttpClient,
    baseUrl: String,
) : LedgerSyncGateway, com.anpfuel.domain.repository.LedgerRecordsGateway {

    private val origin = baseUrl.toHttpUrl()
    private val client = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).callTimeout(20, TimeUnit.SECONDS).build()

    override suspend fun submit(
        scope: PremiumOwnerScope,
        command: PremiumCommand,
        attemptNonce: String,
        familyId: String,
        accessToken: String,
    ): LedgerSyncReceipt {
        if (familyId.isBlank() || accessToken.isBlank()) throw LedgerSyncDenied("ledger.no-proof")
        val body = JSONObject()
            .put("family_id", familyId)
            .put("access_token", accessToken)
            .put(
                "commands",
                JSONArray().put(
                    JSONObject()
                        .put("operation_id", command.operationId)
                        .put("dataset", command.dataset)
                        .put("id", command.recordId)
                        .put("expected_revision", command.expectedRevision)
                        .put("delete", command.delete)
                        .put("void", command.void)
                        .put("payload", if (command.payloadJson.isEmpty()) JSONObject.NULL else JSONObject(command.payloadJson)),
                ),
            ).toString()
        val root = post(body)
        return try {
            val results = root.optJSONArray("results") ?: throw LedgerSyncDenied("ledger.invalid-receipt")
            if (results.length() != 1) throw LedgerSyncDenied("ledger.invalid-receipt")
            val row = results.getJSONObject(0)
            if (row.optString("operation_id", "") != command.operationId) {
                throw LedgerSyncDenied("ledger.invalid-receipt")
            }
            LedgerSyncReceipt(
                operationId = row.getString("operation_id"),
                revision = row.optLong("revision", -1).also {
                    if (it < 0) throw LedgerSyncDenied("ledger.invalid-receipt")
                },
                replayed = row.optBoolean("replayed", false),
            )
        } catch (e: LedgerSyncConflict) {
            throw e
        } catch (e: LedgerSyncDenied) {
            throw e
        } catch (e: Exception) {
            throw LedgerSyncDenied("ledger.invalid-receipt")
        }
    }

    override suspend fun list(
        familyId: String,
        accessToken: String,
        dataset: String,
        vehicleId: String?,
        limit: Int,
        cursor: String?,
    ): com.anpfuel.domain.repository.LedgerRecordsPage {
        if (familyId.isBlank() || accessToken.isBlank()) throw LedgerSyncDenied("ledger.no-proof")
        if (dataset !in setOf("refuel", "expense", "charge")) throw LedgerSyncDenied("ledger.unknown-dataset")
        if (limit !in 1..100) throw LedgerSyncDenied("ledger.bad-limit")
        val body = JSONObject()
            .put("family_id", familyId)
            .put("access_token", accessToken)
            .put("dataset", dataset)
            .put("limit", limit)
        if (!vehicleId.isNullOrBlank()) body.put("vehicle_id", vehicleId)
        if (!cursor.isNullOrBlank()) body.put("cursor", cursor)
        val root = postRecords(body.toString())
        return try {
            val items = root.optJSONArray("items") ?: throw LedgerSyncDenied("ledger.invalid-records")
            if (items.length() > limit) throw LedgerSyncDenied("ledger.over-budget")
            val seen = mutableSetOf<String>()
            val rows = (0 until items.length()).map { index ->
                val row = items.getJSONObject(index)
                val rowDataset = row.optString("dataset", "")
                if (rowDataset != dataset) throw LedgerSyncDenied("ledger.cross-dataset")
                val recordId = row.optString("id", "")
                val owner = row.optString("vehicle_id", "")
                if (!validId(recordId) || !validId(owner) || !seen.add(recordId) ||
                    (vehicleId != null && vehicleId != owner)) throw LedgerSyncDenied("ledger.invalid-record")
                val rawRevision = row.opt("revision")
                if (rawRevision !is Long && rawRevision !is Int) throw LedgerSyncDenied("ledger.invalid-revision")
                val payload = row.optJSONObject("payload") ?: throw LedgerSyncDenied("ledger.invalid-payload")
                val decoded = com.anpfuel.data.mapper.LedgerDraftCodec.decode(
                    com.anpfuel.domain.premium.LedgerDataset.parse(dataset), payload.toString())
                if (decoded.recordId != recordId || payload.optString("vehicle_id") != owner) {
                    throw LedgerSyncDenied("ledger.invalid-payload")
                }
                java.time.Instant.parse(row.getString("occurred_at"))
                if (row.opt("voided") !is Boolean) throw LedgerSyncDenied("ledger.invalid-record")
                com.anpfuel.domain.repository.LedgerServerRecord(
                    dataset = rowDataset,
                    id = recordId,
                    vehicleId = owner,
                    occurredAt = row.optString("occurred_at", ""),
                    revision = row.optLong("revision", -1).also {
                        if (it < 0) throw LedgerSyncDenied("ledger.invalid-record")
                    },
                    voided = row.optBoolean("voided", false),
                    payloadJson = decoded.bodyJson(),
                )
            }
            if (!root.isNull("next_cursor") && root.opt("next_cursor") !is String) throw LedgerSyncDenied("ledger.invalid-cursor")
            val next = if (root.isNull("next_cursor")) null else root.getString("next_cursor").takeIf { it.isNotEmpty() }
            com.anpfuel.domain.repository.LedgerRecordsPage(rows, next)
        } catch (e: LedgerSyncDenied) {
            throw e
        } catch (e: Exception) {
            throw LedgerSyncDenied("ledger.invalid-records")
        }
    }

    private fun validId(value: String): Boolean = try {
        java.util.UUID.fromString(value).toString() == value
    } catch (_: IllegalArgumentException) { false }

    private suspend fun postRecords(body: String): JSONObject {
        val url = origin.newBuilder().addPathSegments("v1/ledger/records").build()
        return try {
            withContext(Dispatchers.IO) {
                suspendCancellableCoroutine { continuation ->
                    val request = Request.Builder().url(url)
                        .header("Accept", "application/json")
                        .header("Cache-Control", "no-store")
                        .post(body.toByteArray(Charsets.UTF_8).toRequestBody(JSON)).build()
                    val call = client.newCall(request)
                    continuation.invokeOnCancellation { call.cancel() }
                    call.enqueue(object : Callback {
                        override fun onFailure(call: Call, e: IOException) {
                            if (continuation.isActive) continuation.resumeWithException(e)
                        }

                        override fun onResponse(call: Call, response: Response) {
                            response.use {
                                if (!continuation.isActive) return
                                try {
                                    when (response.code) {
                                        200 -> Unit
                                        in 400..499 -> throw LedgerSyncDenied("ledger.denied")
                                        else -> throw IOException("ledger.unavailable")
                                    }
                                    val source = response.body?.source() ?: throw IOException("ledger.empty")
                                    if (source.request(BODY_BUDGET)) throw IOException("ledger.over-budget")
                                    continuation.resume(JSONObject(source.readUtf8()))
                                } catch (e: Exception) {
                                    if (continuation.isActive) continuation.resumeWithException(e)
                                }
                            }
                        }
                    })
                }
            }
        } catch (e: CancellationException) {
            throw e
        }
    }
    private suspend fun post(body: String): JSONObject {
        val url = origin.newBuilder().addPathSegments("v1/ledger/commands").build()
        return try {
            withContext(Dispatchers.IO) {
                suspendCancellableCoroutine { continuation ->
                    val request = Request.Builder().url(url)
                        .header("Accept", "application/json")
                        .header("Cache-Control", "no-store")
                        .post(body.toByteArray(Charsets.UTF_8).toRequestBody(JSON)).build()
                    val call = client.newCall(request)
                    continuation.invokeOnCancellation { call.cancel() }
                    call.enqueue(object : Callback {
                        override fun onFailure(call: Call, e: IOException) {
                            if (continuation.isActive) continuation.resumeWithException(e)
                        }

                        override fun onResponse(call: Call, response: Response) {
                            response.use {
                                if (!continuation.isActive) return
                                try {
                                    when (response.code) {
                                        200 -> Unit
                                        409 -> throw LedgerSyncConflict("ledger.conflict")
                                        400 -> throw LedgerSyncConflict("ledger.invalid")
                                        401, 403, 404 -> throw LedgerSyncDenied("ledger.denied")
                                        else -> throw IOException("ledger.unavailable")
                                    }
                                    val source = response.body?.source() ?: throw IOException("ledger.empty")
                                    if (source.request(BODY_BUDGET)) throw IOException("ledger.over-budget")
                                    continuation.resume(JSONObject(source.readUtf8()))
                                } catch (e: Exception) {
                                    if (continuation.isActive) continuation.resumeWithException(e)
                                }
                            }
                        }
                    })
                }
            }
        } catch (e: CancellationException) {
            throw e
        }
    }
}
