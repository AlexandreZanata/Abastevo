package com.anpfuel.data.remote

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.repository.CommentWriteReceipt
import com.anpfuel.domain.repository.FeedbackCommentView
import com.anpfuel.domain.repository.FeedbackException
import com.anpfuel.domain.repository.FeedbackGateway
import com.anpfuel.domain.repository.FeedbackPage
import com.anpfuel.domain.repository.FeedbackRejectKind
import com.anpfuel.domain.repository.RatingStatsSnapshot
import com.anpfuel.domain.repository.RatingWriteReceipt
import com.anpfuel.domain.repository.VoteTallySnapshot
import com.anpfuel.domain.repository.VoteWriteReceipt
import java.io.IOException
import java.net.URLEncoder
import java.time.Instant
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/**
 * P17-T02 bounded feedback HTTP client (B-BR-F01…F08), ratings
 * transport landed in P22-T02.
 *
 * Covers the thirteen P14/P22 routes: rating/stats/remove plus
 * comment/reply/edit/remove, comment/reply pages, vote/remove/tally
 * and report. Every write carries the live session (`family_id` +
 * `access_token`; the author derives server-side). A missing or
 * expired session never touches the network
 * ([FeedbackRejectKind.GATE_REQUIRED]). Backend codes map to stable
 * kinds (rating-out-of-range, stale-revision, self-vote,
 * quota-exceeded); IO failures, empty bodies and unmapped statuses
 * are TRANSPORT with fixed messages — raw bodies never enter error
 * text.
 */
class FeedbackHttpClient(
    private val client: OkHttpClient,
    private val baseUrl: String,
    private val sessions: AuthSessionStore,
    private val nowEpochSeconds: () -> Long = { System.currentTimeMillis() / 1000L },
    private val auth: AuthFlow? = null,
) : FeedbackGateway {

    private data class Session(val familyId: String, val accessToken: String)

    private fun liveSession(): Session {
        val stored = try {
            if (auth == null) sessions.load() else when (val result = auth.refreshSession()) {
                is AuthApiResult.Ok -> result.value
                is AuthApiResult.Err -> null
            }
        } catch (_: Exception) {
            null
        }
        if (stored == null || !PortableAuth.isAccessLive(stored, nowEpochSeconds())) {
            throw FeedbackException(FeedbackRejectKind.GATE_REQUIRED, "no live account session")
        }
        return Session(stored.familyId, stored.accessToken)
    }

    private fun sessionJson(): JSONObject {
        val live = liveSession()
        return JSONObject()
            .put("family_id", live.familyId)
            .put("access_token", live.accessToken)
    }

    override suspend fun rate(
        accountId: String,
        stationId: String,
        product: String,
        stars: Int,
    ): RatingWriteReceipt {
        val payload = sessionJson()
            .put("station_id", stationId)
            .put("product", product)
            .put("stars", stars)
            .toString()
        val raw = post("/v1/feedback/ratings", payload, expected = 201)
        val root = try {
            JSONObject(raw)
        } catch (_: Exception) {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad rating")
        }
        val rating = root.optJSONObject("rating")
        val created = rating?.optBoolean("created", false) ?: false
        return RatingWriteReceipt(
            ratingId = "local:$stationId|$product",
            created = created,
            stats = statsOf(root),
        )
    }

    override suspend fun deleteRating(accountId: String, stationId: String, product: String) {
        val payload = sessionJson()
            .put("station_id", stationId)
            .put("product", product)
            .toString()
        val raw = post("/v1/feedback/ratings/remove", payload, expected = 200)
        val status = try {
            JSONObject(raw).optString("status", "")
        } catch (_: Exception) {
            ""
        }
        if (status != "deleted") {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad rating delete")
        }
    }

    override suspend fun stats(stationId: String, product: String): RatingStatsSnapshot {
        val path = "/v1/feedback/ratings/stats?station_id=" + encode(stationId) +
            "&product=" + encode(product)
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + path)
            .get()
            .header("Accept", "application/json")
            .build()
        val raw = execute(request, expected = 200)
        val root = try {
            JSONObject(raw)
        } catch (_: Exception) {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad stats")
        }
        return statsOf(root)
    }

    override suspend fun submitComment(
        accountId: String,
        stationId: String,
        product: String,
        text: String,
    ): CommentWriteReceipt {
        val payload = sessionJson()
            .put("station_id", stationId)
            .put("product", product)
            .put("text", text)
            .toString()
        val raw = post("/v1/feedback/comments", payload, expected = 201)
        return commentReceipt(raw)
    }

    override suspend fun reply(
        accountId: String,
        stationId: String,
        product: String,
        parentId: String,
        text: String,
    ): CommentWriteReceipt {
        val payload = sessionJson()
            .put("text", text)
            .toString()
        val raw = post("/v1/feedback/comments/" + encode(parentId) + "/replies", payload, expected = 201)
        return commentReceipt(raw)
    }

    override suspend fun editComment(
        accountId: String,
        commentId: String,
        text: String,
        expectedRevision: Int,
    ): CommentWriteReceipt {
        val payload = sessionJson()
            .put("text", text)
            .put("expected_revision", expectedRevision)
            .toString()
        val raw = post("/v1/feedback/comments/" + encode(commentId) + "/edit", payload, expected = 200)
        return commentReceipt(raw)
    }

    override suspend fun deleteComment(accountId: String, commentId: String) {
        val raw = post(
            "/v1/feedback/comments/" + encode(commentId) + "/remove",
            sessionJson().toString(),
            expected = 200,
        )
        val status = try {
            JSONObject(raw).optString("status", "")
        } catch (_: Exception) {
            ""
        }
        if (status != "deleted") {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad delete")
        }
    }

    override suspend fun listComments(
        stationId: String,
        product: String,
        cursor: String,
        limit: Int,
    ): FeedbackPage {
        var path = "/v1/feedback/comments?station_id=" + encode(stationId) +
            "&product=" + encode(product) + "&limit=" + limit
        if (cursor.isNotBlank()) path += "&cursor=" + encode(cursor)
        return getPage(path)
    }

    override suspend fun listReplies(parentId: String, cursor: String, limit: Int): FeedbackPage {
        var path = "/v1/feedback/comments/" + encode(parentId) + "/replies?limit=" + limit
        if (cursor.isNotBlank()) path += "&cursor=" + encode(cursor)
        return getPage(path)
    }

    override suspend fun vote(accountId: String, commentId: String, choice: String): VoteWriteReceipt {
        val payload = sessionJson()
            .put("choice", choice)
            .toString()
        val raw = post("/v1/feedback/comments/" + encode(commentId) + "/votes", payload, expected = 200)
        val tally = tallyOf(raw)
        // The wire confirms votes with the exact revision tally, not a
        // vote id: the receipt id is a client correlation key only.
        return VoteWriteReceipt(voteId = "local:" + tally.commentId + "#" + tally.revision, created = true, tally = tally)
    }

    override suspend fun removeVote(accountId: String, commentId: String) {
        post(
            "/v1/feedback/comments/" + encode(commentId) + "/votes/remove",
            sessionJson().toString(),
            expected = 200,
        )
    }

    override suspend fun tally(commentId: String): VoteTallySnapshot {
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + "/v1/feedback/comments/" + encode(commentId) + "/tally")
            .get()
            .header("Accept", "application/json")
            .build()
        val raw = execute(request, expected = 200)
        return tallyOf(raw)
    }

    override suspend fun report(accountId: String, commentId: String, reason: String) {
        val payload = sessionJson()
            .put("reason", reason)
            .toString()
        val raw = post("/v1/feedback/comments/" + encode(commentId) + "/report", payload, expected = 202)
        val status = try {
            JSONObject(raw).optString("status", "")
        } catch (_: Exception) {
            ""
        }
        if (status != "reported") {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad report")
        }
    }

    private fun post(path: String, json: String, expected: Int): String {
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + path)
            .post(json.toRequestBody(JSON_MEDIA))
            .header("Accept", "application/json")
            .build()
        return execute(request, expected)
    }

    private fun execute(request: Request, expected: Int): String {
        try {
            client.newCall(request).execute().use { response ->
                val body = response.body?.string().orEmpty()
                if (response.code != expected) {
                    throw mapFeedbackError(response.code, body)
                }
                if (body.isBlank()) {
                    throw FeedbackException(
                        FeedbackRejectKind.TRANSPORT,
                        "feedback request failed: empty body",
                    )
                }
                return body
            }
        } catch (refused: FeedbackException) {
            throw refused
        } catch (_: IOException) {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: transport")
        } catch (error: Exception) {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: transport")
        }
    }

    private fun mapFeedbackError(code: Int, body: String): FeedbackException {
        // Fixed messages only: raw bodies never enter error text.
        return when {
            code == 401 || body.contains(CODE_SESSION) ->
                FeedbackException(FeedbackRejectKind.GATE_REQUIRED, "account session required")
            code == 403 && body.contains(CODE_SELF_VOTE) ->
                FeedbackException(FeedbackRejectKind.SELF_VOTE, "authors cannot vote on their comments")
            code == 403 ->
                FeedbackException(FeedbackRejectKind.NOT_AUTHOR, "feedback write refused")
            code == 404 && body.contains(CODE_RATING_NOT_FOUND) ->
                FeedbackException(FeedbackRejectKind.RATING_NOT_FOUND, "rating not found")
            code == 404 ->
                FeedbackException(FeedbackRejectKind.COMMENT_NOT_FOUND, "comment not found")
            code == 409 && body.contains(CODE_STALE) ->
                FeedbackException(FeedbackRejectKind.STALE_REVISION, "comment changed, reload and retry")
            code == 409 ->
                FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: HTTP 409")
            code == 429 || body.contains(CODE_QUOTA) ->
                FeedbackException(FeedbackRejectKind.QUOTA_EXCEEDED, "report quota exhausted, retry later")
            body.contains(CODE_TEXT_EMPTY) ->
                FeedbackException(FeedbackRejectKind.TEXT_EMPTY, "comment text is empty")
            body.contains(CODE_TEXT_LONG) ->
                FeedbackException(FeedbackRejectKind.TEXT_TOO_LONG, "comment text exceeds 280 scalars")
            body.contains(CODE_RATING) ->
                FeedbackException(FeedbackRejectKind.RATING_OUT_OF_RANGE, "rating outside 1-5")
            body.contains(CODE_CHOICE) ->
                FeedbackException(FeedbackRejectKind.VOTE_CHOICE_INVALID, "vote must be VALID or INVALID")
            body.contains(CODE_REPORT) ->
                FeedbackException(FeedbackRejectKind.REPORT_INVALID, "report reason required")
            else ->
                FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: HTTP $code")
        }
    }

    private fun commentReceipt(raw: String): CommentWriteReceipt {
        val comment = try {
            JSONObject(raw).getJSONObject("comment")
        } catch (_: Exception) {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad comment")
        }
        val id = comment.optString("id", "")
        val revision = comment.optInt("revision", -1)
        if (id.isBlank() || revision < 1) {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad comment")
        }
        return CommentWriteReceipt(id, revision)
    }

    private fun statsOf(root: JSONObject): RatingStatsSnapshot {
        val stats = root.optJSONObject("stats")
            ?: throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad stats")
        val stationId = stats.optString("station_id", "")
        val product = stats.optString("product", "")
        val count = stats.optLong("count", -1L)
        val sum = stats.optLong("sum", -1L)
        if (stationId.isBlank() || product.isBlank() || count < 0 || sum < 0) {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad stats")
        }
        return RatingStatsSnapshot(stationId, product, count, sum)
    }

    private fun tallyOf(raw: String): VoteTallySnapshot {        val tally = try {
            JSONObject(raw).getJSONObject("tally")
        } catch (_: Exception) {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad tally")
        }
        val commentId = tally.optString("comment_id", "")
        val revision = tally.optInt("revision", -1)
        val valid = tally.optLong("valid", -1L)
        val invalid = tally.optLong("invalid", -1L)
        if (commentId.isBlank() || revision < 1 || valid < 0 || invalid < 0) {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad tally")
        }
        return VoteTallySnapshot(commentId, revision, valid, invalid)
    }

    private fun getPage(path: String): FeedbackPage {
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + path)
            .get()
            .header("Accept", "application/json")
            .build()
        val raw = execute(request, expected = 200)
        val root = try {
            JSONObject(raw)
        } catch (_: Exception) {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad page")
        }
        val items = root.optJSONArray("comments")
            ?: throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad page")
        val out = ArrayList<FeedbackCommentView>(items.length())
        for (index in 0 until items.length()) {
            out += viewOf(items.getJSONObject(index))
        }
        val cursor = if (root.isNull("next_cursor")) null else root.optString("next_cursor", "").ifBlank { null }
        return FeedbackPage(out, cursor)
    }

    private fun viewOf(item: JSONObject): FeedbackCommentView {
        val id = item.optString("id", "")
        val revision = item.optInt("revision", -1)
        if (id.isBlank() || revision < 1) {
            throw FeedbackException(FeedbackRejectKind.TRANSPORT, "feedback request failed: bad row")
        }
        return FeedbackCommentView(
            id = id,
            alias = item.optString("public_alias", ""),
            stationId = item.optString("station_id", ""),
            product = item.optString("product", ""),
            parentId = item.optString("parent_id", ""),
            depth = item.optInt("depth", 0),
            text = item.optString("text", ""),
            revision = revision,
            createdAt = epochOf(item.optString("created_at", "")),
            updatedAt = epochOf(item.optString("updated_at", "")),
        )
    }

    private fun epochOf(raw: String): Long {
        return try {
            Instant.parse(raw).epochSecond
        } catch (_: Exception) {
            0L
        }
    }

    private fun encode(value: String): String =
        URLEncoder.encode(value, Charsets.UTF_8.name())

    companion object {
        private val JSON_MEDIA = "application/json; charset=utf-8".toMediaType()
        private const val CODE_SESSION = "feedback.session-invalid"
        private const val CODE_SELF_VOTE = "feedback.self-vote"
        private const val CODE_STALE = "feedback.stale-revision"
        private const val CODE_QUOTA = "feedback.quota-exceeded"
        private const val CODE_TEXT_EMPTY = "feedback.text-empty"
        private const val CODE_TEXT_LONG = "feedback.text-too-long"
        private const val CODE_RATING = "feedback.rating-out-of-range"
        private const val CODE_RATING_NOT_FOUND = "feedback.rating-not-found"
        private const val CODE_CHOICE = "feedback.vote-choice-invalid"
        private const val CODE_REPORT = "feedback.report-invalid"
    }
}
