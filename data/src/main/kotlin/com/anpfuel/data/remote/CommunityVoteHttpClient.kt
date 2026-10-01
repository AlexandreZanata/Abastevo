package com.anpfuel.data.remote

import com.anpfuel.domain.repository.CommunityVoteException
import com.anpfuel.domain.repository.CommunityVoteGateway
import com.anpfuel.domain.repository.CommunityVoteReceipt
import com.anpfuel.domain.repository.CommunityVoteRejectKind
import java.io.IOException
import java.net.URLEncoder
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/**
 * P10-T07 bounded community-vote HTTP client (BUC-005, B-BR-006/011).
 *
 * Network-only: `POST /v1/observations/{id}/confirmations` and
 * `POST /v1/observations/{id}/disputes` with the stable
 * `client_submission_id` per intent. Transport failures, empty bodies
 * and unmapped statuses throw [CommunityVoteException] with
 * [CommunityVoteRejectKind.TRANSPORT]; mapped backend codes surface as
 * SELF_CONFIRMATION (403), ALREADY_RECORDED (409),
 * INELIGIBLE_TARGET (404/409) or CONFLICT (409, same key different
 * body) so the UI shows a clear status instead of counting a second
 * vote. Identical retries converge server-side to the same vote id;
 * this client never retries by itself. Private dispute detail travels
 * only in the dispute body and never appears in error messages
 * (B-BR-011). HTTPS only; the preview base URL never resolves until
 * deployment configuration lands.
 */
class CommunityVoteHttpClient(
    private val client: OkHttpClient,
    private val baseUrl: String,
) : CommunityVoteGateway {

    override suspend fun submitConfirmation(
        observationId: String,
        clientSubmissionId: String,
    ): CommunityVoteReceipt {
        require(observationId.isNotBlank()) { "observation_id is blank" }
        require(clientSubmissionId.isNotBlank()) { "client_submission_id is blank" }
        val payload = JSONObject()
            .put("client_submission_id", clientSubmissionId)
            .toString()
        val raw = post(
            path = "/v1/observations/" + encode(observationId) + "/confirmations",
            json = payload,
        )
        val voteId = parseId(raw, "confirmation_id")
        return CommunityVoteReceipt(voteId, observationId, replayed = false)
    }

    override suspend fun submitDispute(
        targetObservationId: String,
        clientSubmissionId: String,
        reasonWire: String,
        detail: String?,
        replacementObservationId: String?,
    ): CommunityVoteReceipt {
        require(targetObservationId.isNotBlank()) { "target_observation_id is blank" }
        require(clientSubmissionId.isNotBlank()) { "client_submission_id is blank" }
        require(reasonWire.isNotBlank()) { "reason is blank" }
        val payload = JSONObject()
            .put("client_submission_id", clientSubmissionId)
            .put("reason", reasonWire)
        if (!detail.isNullOrBlank()) {
            payload.put("detail", detail)
        }
        if (!replacementObservationId.isNullOrBlank()) {
            payload.put("replacement_observation_id", replacementObservationId)
        }
        val raw = post(
            path = "/v1/observations/" + encode(targetObservationId) + "/disputes",
            json = payload.toString(),
        )
        val voteId = parseId(raw, "dispute_id")
        return CommunityVoteReceipt(voteId, targetObservationId, replayed = false)
    }

    private fun post(path: String, json: String): String {
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + path)
            .post(json.toRequestBody(JSON_MEDIA))
            .header("Accept", "application/json")
            .header("Idempotency-Key", idempotencyOf(json))
            .build()
        try {
            client.newCall(request).execute().use { response ->
                val body = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    throw mapVoteError(response.code, body)
                }
                if (body.isBlank()) {
                    throw CommunityVoteException(
                        CommunityVoteRejectKind.TRANSPORT,
                        "vote request failed: empty body",
                    )
                }
                return body
            }
        } catch (voteError: CommunityVoteException) {
            throw voteError
        } catch (error: IOException) {
            throw CommunityVoteException(
                CommunityVoteRejectKind.TRANSPORT,
                "vote request failed: transport",
            )
        } catch (error: Exception) {
            throw CommunityVoteException(
                CommunityVoteRejectKind.TRANSPORT,
                "vote request failed: transport",
            )
        }
    }

    private fun mapVoteError(code: Int, body: String): CommunityVoteException {
        // Fixed messages only: the raw body (which may echo metadata)
        // and the private dispute detail never enter error text.
        return when {
            code == 403 && body.contains(CODE_SELF) ->
                CommunityVoteException(
                    CommunityVoteRejectKind.SELF_CONFIRMATION,
                    "contributors cannot confirm their own observations",
                )
            code == 409 && body.contains(CODE_ALREADY) ->
                CommunityVoteException(
                    CommunityVoteRejectKind.ALREADY_RECORDED,
                    "observation already recorded by contributor",
                )
            body.contains(CODE_INELIGIBLE) || body.contains(CODE_NOT_FOUND) ->
                CommunityVoteException(
                    CommunityVoteRejectKind.INELIGIBLE_TARGET,
                    "target not eligible for this command",
                )
            code == 409 && body.contains(CODE_CONFLICT) ->
                CommunityVoteException(
                    CommunityVoteRejectKind.CONFLICT,
                    "same key, different body",
                )
            else ->
                CommunityVoteException(
                    CommunityVoteRejectKind.TRANSPORT,
                    "vote request failed: HTTP $code",
                )
        }
    }

    private fun parseId(body: String, field: String): String {
        val id = try {
            JSONObject(body).optString(field, "")
        } catch (_: Exception) {
            ""
        }
        if (id.isBlank()) {
            throw CommunityVoteException(
                CommunityVoteRejectKind.TRANSPORT,
                "vote request failed: missing $field",
            )
        }
        return id
    }

    private fun idempotencyOf(json: String): String {
        return try {
            JSONObject(json).optString("client_submission_id", "unknown")
                .takeIf { it.isNotBlank() } ?: "unknown"
        } catch (_: Exception) {
            "unknown"
        }
    }

    private fun encode(value: String): String =
        URLEncoder.encode(value, Charsets.UTF_8.name())

    companion object {
        private val JSON_MEDIA = "application/json; charset=utf-8".toMediaType()
        private const val CODE_SELF = "community.self-confirmation"
        private const val CODE_ALREADY = "community.already-confirmed"
        private const val CODE_INELIGIBLE = "community.ineligible-target"
        private const val CODE_NOT_FOUND = "community.not-found"
        private const val CODE_CONFLICT = "community.conflict"
    }
}
