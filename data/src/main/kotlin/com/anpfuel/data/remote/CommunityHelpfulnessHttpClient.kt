package com.anpfuel.data.remote

import com.anpfuel.domain.repository.CommunityHelpfulnessException
import com.anpfuel.domain.repository.CommunityHelpfulnessGateway
import com.anpfuel.domain.repository.CommunityHelpfulnessReceipt
import com.anpfuel.domain.repository.CommunityHelpfulnessRejectKind
import com.anpfuel.domain.repository.CommunityHelpfulnessTally
import java.io.IOException
import java.net.URLEncoder
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/**
 * CS08A bounded helpfulness HTTP client (B-BR-CS01).
 *
 * Network-only: `POST /v1/observations/{id}/helpfulness`,
 * `POST /v1/observations/{id}/helpfulness/remove` and
 * `GET /v1/observations/{id}/helpfulness?decision_sequence=N` with the
 * stable `client_submission_id` per intent. Exact tallies parse;
 * `helpful_basis_points` and `my_choice` stay absent when the backend
 * omits them (zero votes, anonymous reads). Transport failures, empty
 * bodies and unmapped statuses throw [CommunityHelpfulnessException]
 * with [CommunityHelpfulnessRejectKind.TRANSPORT]; mapped backend codes
 * surface as SELF_VOTE (403), STALE_REVISION (409), INELIGIBLE_TARGET
 * (404/409), QUOTA_EXCEEDED (429) or INVALID_VOTE (400) so the UI shows
 * a clear status instead of counting a vote. Identical retries converge
 * server-side to the same vote id; this client never retries by itself.
 * HTTPS only; the preview base URL never resolves until deployment
 * configuration lands.
 */
class CommunityHelpfulnessHttpClient(
    private val client: OkHttpClient,
    private val baseUrl: String,
) : CommunityHelpfulnessGateway {

    override suspend fun castHelpfulness(
        observationId: String,
        clientSubmissionId: String,
        choiceWire: String,
        decisionSequence: Long,
    ): CommunityHelpfulnessReceipt {
        require(observationId.isNotBlank()) { "observation_id is blank" }
        require(clientSubmissionId.isNotBlank()) { "client_submission_id is blank" }
        require(choiceWire.isNotBlank()) { "choice is blank" }
        val payload = JSONObject()
            .put("choice", choiceWire)
            .put("decision_sequence", decisionSequence)
            .put("client_submission_id", clientSubmissionId)
            .toString()
        val raw = post(
            path = "/v1/observations/" + encode(observationId) + "/helpfulness",
            json = payload,
        )
        val body = JSONObject(raw)
        return CommunityHelpfulnessReceipt(
            voteId = body.getString("vote_id"),
            observationId = observationId,
            helpful = body.getLong("helpful"),
            notHelpful = body.getLong("not_helpful"),
            total = body.optLong("total", body.getLong("helpful") + body.getLong("not_helpful")),
            helpfulBasisPoints = if (body.has("helpful_basis_points")) body.getLong("helpful_basis_points") else null,
            myChoice = body.optString("my_choice", "").takeIf { it.isNotBlank() },
            replayed = false,
        )
    }

    override suspend fun removeHelpfulness(
        observationId: String,
        decisionSequence: Long,
    ): CommunityHelpfulnessTally {
        require(observationId.isNotBlank()) { "observation_id is blank" }
        val payload = JSONObject()
            .put("decision_sequence", decisionSequence)
            .toString()
        val raw = post(
            path = "/v1/observations/" + encode(observationId) + "/helpfulness/remove",
            json = payload,
        )
        return parseTally(observationId, raw)
    }

    override suspend fun tallyFor(
        observationId: String,
        decisionSequence: Long,
    ): CommunityHelpfulnessTally {
        require(observationId.isNotBlank()) { "observation_id is blank" }
        val request = Request.Builder()
            .url(
                baseUrl.trimEnd('/') + "/v1/observations/" + encode(observationId) +
                    "/helpfulness?decision_sequence=" + decisionSequence,
            )
            .get()
            .header("Accept", "application/json")
            .build()
        try {
            client.newCall(request).execute().use { response ->
                val body = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    throw mapHelpfulnessError(response.code, body)
                }
                if (body.isBlank()) {
                    throw CommunityHelpfulnessException(
                        CommunityHelpfulnessRejectKind.TRANSPORT,
                        "helpfulness request failed: empty body",
                    )
                }
                return parseTally(observationId, body)
            }
        } catch (voteError: CommunityHelpfulnessException) {
            throw voteError
        } catch (error: IOException) {
            throw CommunityHelpfulnessException(
                CommunityHelpfulnessRejectKind.TRANSPORT,
                "helpfulness request failed: transport",
            )
        } catch (error: Exception) {
            throw CommunityHelpfulnessException(
                CommunityHelpfulnessRejectKind.TRANSPORT,
                "helpfulness request failed: transport",
            )
        }
    }

    private fun parseTally(observationId: String, raw: String): CommunityHelpfulnessTally {
        val body = try {
            JSONObject(raw)
        } catch (_: Exception) {
            throw CommunityHelpfulnessException(
                CommunityHelpfulnessRejectKind.TRANSPORT,
                "helpfulness request failed: malformed tally",
            )
        }
        return CommunityHelpfulnessTally(
            observationId = observationId,
            decisionSequence = body.optLong("decision_sequence", 0),
            helpful = body.optLong("helpful", 0),
            notHelpful = body.optLong("not_helpful", 0),
            total = body.optLong("total", body.optLong("helpful", 0) + body.optLong("not_helpful", 0)),
            helpfulBasisPoints = if (body.has("helpful_basis_points")) body.getLong("helpful_basis_points") else null,
            myChoice = body.optString("my_choice", "").takeIf { it.isNotBlank() },
            underReview = body.optBoolean("under_review", false),
        )
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
                    throw mapHelpfulnessError(response.code, body)
                }
                if (body.isBlank()) {
                    throw CommunityHelpfulnessException(
                        CommunityHelpfulnessRejectKind.TRANSPORT,
                        "helpfulness request failed: empty body",
                    )
                }
                return body
            }
        } catch (voteError: CommunityHelpfulnessException) {
            throw voteError
        } catch (error: IOException) {
            throw CommunityHelpfulnessException(
                CommunityHelpfulnessRejectKind.TRANSPORT,
                "helpfulness request failed: transport",
            )
        } catch (error: Exception) {
            throw CommunityHelpfulnessException(
                CommunityHelpfulnessRejectKind.TRANSPORT,
                "helpfulness request failed: transport",
            )
        }
    }

    private fun mapHelpfulnessError(code: Int, body: String): CommunityHelpfulnessException {
        // Fixed messages only: the raw body (which may echo metadata)
        // never enters error text.
        return when {
            code == 403 && body.contains(CODE_SELF) ->
                CommunityHelpfulnessException(
                    CommunityHelpfulnessRejectKind.SELF_VOTE,
                    "contributors cannot vote on their own contributions",
                )
            code == 409 && body.contains(CODE_STALE) ->
                CommunityHelpfulnessException(
                    CommunityHelpfulnessRejectKind.STALE_REVISION,
                    "contribution moved on; bind the current revision",
                )
            body.contains(CODE_INELIGIBLE) || body.contains(CODE_NOT_FOUND) ->
                CommunityHelpfulnessException(
                    CommunityHelpfulnessRejectKind.INELIGIBLE_TARGET,
                    "target not eligible for this command",
                )
            code == 429 ->
                CommunityHelpfulnessException(
                    CommunityHelpfulnessRejectKind.QUOTA_EXCEEDED,
                    "write budget exhausted, try again later",
                )
            code == 400 && body.contains(CODE_INVALID) ->
                CommunityHelpfulnessException(
                    CommunityHelpfulnessRejectKind.INVALID_VOTE,
                    "invalid helpfulness vote",
                )
            else ->
                CommunityHelpfulnessException(
                    CommunityHelpfulnessRejectKind.TRANSPORT,
                    "helpfulness request failed: HTTP $code",
                )
        }
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
        private const val CODE_SELF = "community.self-vote"
        private const val CODE_STALE = "community.stale-revision"
        private const val CODE_INELIGIBLE = "community.ineligible-target"
        private const val CODE_NOT_FOUND = "community.not-found"
        private const val CODE_INVALID = "community.invalid-vote"
    }
}
