package com.anpfuel.data.remote.ledger

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.remote.PhotoOperationRefused
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.repository.LedgerConsentGateway
import com.anpfuel.domain.repository.LedgerConsentStatus
import com.anpfuel.domain.repository.LedgerSyncConflict
import com.anpfuel.domain.repository.LedgerSyncDenied
import com.anpfuel.domain.repository.LedgerSyncGateway
import com.anpfuel.domain.repository.LedgerSyncReceipt
import java.io.IOException
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
import org.json.JSONArray
import org.json.JSONObject

private val BODY_BUDGET = 1024L * 1024L + 1
private val JSON = "application/json; charset=utf-8".toMediaType()

/** The live session or the bound device key belongs to another account. */
internal class LedgerOwnerChanged : IOException("ledger.owner-changed")

/** No live session for a signed ledger write. */
internal class LedgerSignInRequired : IOException("ledger.sign-in-required")

/**
 * One signed ledger write: the exact [body] signed by the device key bound to
 * [owner], sent once with [key] as its idempotency key, answered as JSON.
 * Refusals surface as [PhotoOperationRefused]; nothing is retried here.
 */
fun interface LedgerSignedPost {
    suspend fun post(path: String, key: String, owner: String, body: JSONObject): JSONObject
}

/**
 * MEX-14 private ledger transport (backend `/v1/ledger/*`).
 *
 * Writes (`/v1/ledger/commands`, `/v1/ledger/consent`) go through [signed]:
 * the backend resolves their owner from the session and the device key bound
 * to it, and refuses unsigned writes. Record pages (`/v1/ledger/records`) are
 * plain session reads, free after a lapse.
 *
 * One command per call (bounded payloads, bounded clock), with its operation
 * id as idempotency key. 409/400 refuse without retry (the held command is
 * parked, never deleted); 401/403/404 and an owner change are denials with a
 * bounded retry budget (proof, consent or grant may arrive); outages and lost
 * replies retry idempotently with the same operation id. No-store everywhere.
 */
class LedgerSyncHttpClient(
    http: OkHttpClient,
    baseUrl: String,
    private val signed: LedgerSignedPost,
    private val newKey: () -> String = { UUID.randomUUID().toString() },
) : LedgerSyncGateway, com.anpfuel.domain.repository.LedgerRecordsGateway, LedgerConsentGateway {

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
            )
        val root = signedWrite(COMMANDS_PATH, command.operationId, scope.accountId, body) { status ->
            when (status) {
                409 -> LedgerSyncConflict("ledger.conflict")
                400 -> LedgerSyncConflict("ledger.invalid")
                401, 403, 404 -> LedgerSyncDenied("ledger.denied")
                else -> IOException("ledger.unavailable")
            }
        }
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

    override suspend fun setConsent(
        accountId: String,
        familyId: String,
        accessToken: String,
        datasets: List<LedgerDataset>,
        enabled: Boolean,
    ): LedgerConsentStatus {
        if (accountId.isBlank() || familyId.isBlank() || accessToken.isBlank()) throw LedgerSyncDenied("ledger.no-proof")
        val requested = LedgerDataset.entries.filter { it in datasets }
        if (requested.isEmpty() || requested.size != datasets.size) throw LedgerSyncDenied("ledger.invalid-consent")
        val body = JSONObject()
            .put("family_id", familyId)
            .put("access_token", accessToken)
            .put("datasets", JSONArray(requested.map { it.wire }))
            .put("enabled", enabled)
        val root = signedWrite(CONSENT_PATH, newKey(), accountId, body) { status ->
            if (status in 400..499 && status != 409 && status != 429) LedgerSyncDenied("ledger.consent-refused")
            else IOException("ledger.unavailable")
        }
        return try {
            val revision = when (val raw = root.opt("revision")) {
                is Int -> raw.toLong()
                is Long -> raw
                else -> throw LedgerSyncDenied("ledger.invalid-consent")
            }
            if (revision < 0) throw LedgerSyncDenied("ledger.invalid-consent")
            val wires = root.optJSONArray("datasets") ?: throw LedgerSyncDenied("ledger.invalid-consent")
            val confirmed = (0 until wires.length()).map { index ->
                val wire = wires.opt(index) as? String ?: throw LedgerSyncDenied("ledger.invalid-consent")
                LedgerDataset.entries.firstOrNull { it.wire == wire } ?: throw LedgerSyncDenied("ledger.invalid-consent")
            }
            if (confirmed.toSet().size != confirmed.size) throw LedgerSyncDenied("ledger.invalid-consent")
            LedgerConsentStatus(revision, confirmed)
        } catch (e: LedgerSyncDenied) {
            throw e
        } catch (e: Exception) {
            throw LedgerSyncDenied("ledger.invalid-consent")
        }
    }

    /**
     * Sends one signed write and maps its failures: an HTTP refusal through
     * [refusal], an owner change or missing session to a denial, and any other
     * I/O failure (offline, rate limit, identity not ready) to a retryable
     * [IOException].
     */
    private suspend fun signedWrite(
        path: String,
        key: String,
        owner: String,
        body: JSONObject,
        refusal: (Int) -> Exception,
    ): JSONObject = try {
        signed.post(path, key, owner, body)
    } catch (cancelled: CancellationException) {
        throw cancelled
    } catch (refused: PhotoOperationRefused) {
        throw refusal(refused.status)
    } catch (_: LedgerOwnerChanged) {
        throw LedgerSyncDenied("ledger.owner-changed")
    } catch (_: LedgerSignInRequired) {
        throw LedgerSyncDenied("ledger.no-proof")
    } catch (failure: IOException) {
        if (failure.message == IDENTITY_CHANGED) throw LedgerSyncDenied("ledger.owner-changed")
        throw failure
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

    companion object {
        const val COMMANDS_PATH = "/v1/ledger/commands"
        const val CONSENT_PATH = "/v1/ledger/consent"
        private const val IDENTITY_CHANGED = "photo.identity-changed"

        /**
         * Production signer: refreshes the session for every attempt (as other
         * signed account writes do), refuses another account, and signs with the
         * device key bound to the owner. The refreshed session replaces the one
         * in the body, so a background retry never sends an expired token.
         */
        fun signed(proof: PhotoProofTransport, auth: AuthFlow): LedgerSignedPost =
            LedgerSignedPost { path, key, owner, body ->
                val live = liveSession(auth, owner)
                val scope = proof.localScope()
                if (!scope.startsWith("account:$owner:")) throw LedgerOwnerChanged()
                val doc = JSONObject(body.toString())
                    .put("family_id", live.familyId)
                    .put("access_token", live.accessToken)
                proof.post(path, key, doc, scope)
            }

        private suspend fun liveSession(auth: AuthFlow, owner: String): PortableAuth.Session =
            withContext(Dispatchers.IO) {
                val live = when (val result = auth.refreshSession()) {
                    is AuthApiResult.Err -> {
                        if (auth.rehydrate() == AuthFlow.AuthState.LoggedOut) throw LedgerSignInRequired()
                        throw IOException("ledger.session-unavailable")
                    }
                    is AuthApiResult.Ok -> result.value ?: throw LedgerSignInRequired()
                }
                if (!PortableAuth.isAccessLive(live, System.currentTimeMillis() / 1000)) throw LedgerSignInRequired()
                if (live.accountId != owner) throw LedgerOwnerChanged()
                live
            }
    }
}
