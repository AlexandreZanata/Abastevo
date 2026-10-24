package com.anpfuel.data.remote.charging

import com.anpfuel.domain.charging.ChargingCondition
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingObservationDraft
import com.anpfuel.domain.charging.ChargingObservationException
import com.anpfuel.domain.charging.ChargingObservationGateway
import com.anpfuel.domain.charging.ChargingReportReason
import com.anpfuel.domain.charging.ChargingTally
import com.anpfuel.domain.charging.ChargingVote
import com.anpfuel.domain.charging.CommunityChargingObservation
import com.anpfuel.domain.charging.OwnedChargingObservation
import java.io.IOException
import java.time.LocalDate
import java.util.UUID
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
import org.json.JSONObject

private const val BODY_BUDGET = 256L * 1024L + 1
private val JSON = "application/json; charset=utf-8".toMediaType()

/**
 * EE-22 bounded community EV client over EE-12/13 routes.
 *
 * Session proof travels only inside the exact request body; the client
 * never sends an account id, GPS or private context. Observation writes
 * are idempotent by caller ID, corrections/withdrawals require the exact
 * revision, and votes/reports converge by submission/target. All calls
 * are cancellable, no-store, redirect-free and body-bounded.
 */
class ChargingObservationHttpClient(
    http: OkHttpClient,
    baseUrl: String,
) : ChargingObservationGateway {

    private val origin = baseUrl.toHttpUrl()
    private val client = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).callTimeout(20, TimeUnit.SECONDS).build()

    override suspend fun history(placeId: String, limit: Int): List<CommunityChargingObservation> {
        require(UUID.fromString(placeId).toString() == placeId && limit in 1..100)
        val url = origin.newBuilder().addPathSegments("v1/charging/observations/history")
            .addQueryParameter("place_id", placeId).addQueryParameter("limit", limit.toString()).build()
        val root = call("GET", url.toString(), null)
        val items = root.optJSONArray("items") ?: throw sdkInvalid()
        return (0 until items.length()).map { index ->
            val row = items.getJSONObject(index)
            CommunityChargingObservation(
                id = id(row.getString("id")),
                connector = connector(row.getString("connector_code")),
                priceMilliBrlPerKwh = price(row),
                condition = condition(row.getString("condition")),
                observedDay = day(row.getString("observed_day")),
                note = note(row.optString("note", "")),
                helpful = count(row, "helpful"),
                notHelpful = count(row, "not_helpful"),
            )
        }
    }

    override suspend fun mine(familyId: String, accessToken: String, limit: Int): List<OwnedChargingObservation> {
        val body = JSONObject().put("session", session(familyId, accessToken)).put("limit", limit)
        val root = call("POST", route("v1/charging/observations/mine"), body.toString())
        val items = root.optJSONArray("items") ?: throw sdkInvalid()
        return (0 until items.length()).map { index ->
            val row = items.getJSONObject(index)
            OwnedChargingObservation(
                id = id(row.getString("id")),
                placeId = if (row.isNull("place_id")) null else id(row.getString("place_id")),
                connector = connector(row.getString("connector_code")),
                priceMilliBrlPerKwh = price(row),
                condition = condition(row.getString("condition")),
                observedDay = day(row.getString("observed_day")),
                note = note(row.optString("note", "")),
                revision = positive(row, "revision"),
            )
        }
    }

    override suspend fun submit(familyId: String, accessToken: String, id: String, draft: ChargingObservationDraft): String {
        id(id)
        val body = base(familyId, accessToken, id, draft)
        val root = call("POST", route("v1/charging/observations"), body.toString())
        return id(root.getString("id"))
    }

    override suspend fun correct(
        familyId: String,
        accessToken: String,
        id: String,
        expectedRevision: Long,
        draft: ChargingObservationDraft,
    ): Long {
        id(id)
        require(expectedRevision > 0)
        val body = base(familyId, accessToken, id, draft).put("expected_revision", expectedRevision)
        val root = call("PUT", route("v1/charging/observations/$id"), body.toString())
        return positive(root, "revision")
    }

    override suspend fun withdraw(familyId: String, accessToken: String, id: String, expectedRevision: Long): Long {
        id(id)
        require(expectedRevision > 0)
        val body = JSONObject().put("session", session(familyId, accessToken)).put("expected_revision", expectedRevision)
        val root = call("DELETE", route("v1/charging/observations/$id"), body.toString())
        return positive(root, "revision")
    }

    override suspend fun vote(
        familyId: String,
        accessToken: String,
        observationId: String,
        choice: ChargingVote,
        submissionId: String,
    ): ChargingTally {
        id(observationId)
        require(submissionId.isNotBlank() && submissionId.length <= 128)
        val body = JSONObject().put("session", session(familyId, accessToken))
            .put("choice", choice.name).put("client_submission_id", submissionId)
        val root = call("POST", route("v1/charging/observations/$observationId/helpfulness"), body.toString())
        if (root.getString("observation_id") != observationId) throw sdkInvalid()
        return ChargingTally(count(root, "helpful"), count(root, "not_helpful"))
    }

    override suspend fun report(
        familyId: String,
        accessToken: String,
        observationId: String,
        reason: ChargingReportReason,
        detail: String,
    ) {
        id(observationId)
        require(detail.length <= 1024 && detail.none { it.isISOControl() })
        val body = JSONObject().put("session", session(familyId, accessToken))
            .put("reason", reason.name).put("detail", detail)
        val root = call("POST", route("v1/charging/observations/$observationId/report"), body.toString())
        if (root.getString("observation_id") != observationId || root.getString("status") != "received") {
            throw sdkInvalid()
        }
    }

    private fun base(familyId: String, accessToken: String, id: String, draft: ChargingObservationDraft) =
        JSONObject().put("session", session(familyId, accessToken)).put("id", id)
            .put("connector_code", draft.connector.name)
            .put("condition", draft.condition.name)
            .put("observed_day", draft.observedDay)
            .put("note", draft.note).apply {
                if (draft.placeId != null) put("place_id", draft.placeId)
                if (draft.priceMilliBrlPerKwh != null) put("price_milli_brl_per_kwh", draft.priceMilliBrlPerKwh)
            }

    private fun session(familyId: String, accessToken: String): JSONObject {
        if (familyId.isBlank() || accessToken.isBlank()) throw ChargingObservationException.SignedOut()
        return JSONObject().put("family_id", familyId).put("access_token", accessToken)
    }

    private fun route(path: String): String = origin.newBuilder().addPathSegments(path).build().toString()

    private fun id(value: String): String = try {
        if (UUID.fromString(value).toString() == value) value else throw sdkInvalid()
    } catch (_: IllegalArgumentException) {
        throw sdkInvalid()
    }

    private fun connector(value: String): ChargingConnector =
        ChargingConnector.entries.firstOrNull { it.name == value } ?: ChargingConnector.UNKNOWN

    private fun condition(value: String): ChargingCondition =
        ChargingCondition.entries.firstOrNull { it.name == value } ?: throw sdkInvalid()

    private fun day(value: String): String = try {
        LocalDate.parse(value).toString()
    } catch (_: Exception) {
        throw sdkInvalid()
    }

    private fun note(value: String): String =
        if (value.length <= 1024) value else throw sdkInvalid()

    private fun price(row: JSONObject): Long? {
        if (!row.has("price_milli_brl_per_kwh") || row.isNull("price_milli_brl_per_kwh")) return null
        val value = row.getLong("price_milli_brl_per_kwh")
        if (value !in 1..10_000_000) throw sdkInvalid()
        return value
    }

    private fun count(row: JSONObject, key: String): Long {
        val value = row.getLong(key)
        if (value < 0) throw sdkInvalid()
        return value
    }

    private fun positive(row: JSONObject, key: String): Long {
        val value = row.getLong(key)
        if (value < 1) throw sdkInvalid()
        return value
    }

    private fun sdkInvalid() = ChargingObservationException.Invalid("charging.invalid-body")

    private suspend fun call(method: String, url: String, body: String?): JSONObject {
        return try {
            withContext(Dispatchers.IO) {
                suspendCancellableCoroutine { continuation ->
                    val builder = Request.Builder().url(url)
                        .header("Accept", "application/json")
                        .header("Cache-Control", "no-store")
                    if (body == null) {
                        builder.get()
                    } else {
                        builder.method(method, body.toByteArray(Charsets.UTF_8).toRequestBody(JSON))
                    }
                    val request = builder.build()
                    val call = client.newCall(request)
                    continuation.invokeOnCancellation { call.cancel() }
                    call.enqueue(object : Callback {
                        override fun onFailure(call: Call, e: IOException) {
                            if (continuation.isActive) {
                                continuation.resumeWithException(ChargingObservationException.Unavailable("charging.offline", e))
                            }
                        }

                        override fun onResponse(call: Call, response: Response) {
                            response.use {
                                if (!continuation.isActive) return
                                try {
                                    when (response.code) {
                                        in 200..299 -> Unit
                                        400, 404 -> throw ChargingObservationException.Invalid("charging.invalid")
                                        401 -> throw ChargingObservationException.SignedOut()
                                        403 -> throw ChargingObservationException.Denied("charging.denied")
                                        409 -> throw ChargingObservationException.Conflict("charging.conflict")
                                        429 -> throw ChargingObservationException.Denied("charging.quota")
                                        else -> throw ChargingObservationException.Unavailable("charging.unavailable")
                                    }
                                    val source = response.body?.source()
                                        ?: throw ChargingObservationException.Invalid("charging.empty")
                                    if (source.request(BODY_BUDGET)) throw ChargingObservationException.Invalid("charging.over-budget")
                                    continuation.resume(JSONObject(source.readUtf8()))
                                } catch (e: Exception) {
                                    if (continuation.isActive) continuation.resumeWithException(
                                        if (e is ChargingObservationException) e
                                        else ChargingObservationException.Invalid("charging.invalid-body", e),
                                    )
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
