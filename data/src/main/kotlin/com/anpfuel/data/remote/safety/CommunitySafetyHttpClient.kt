package com.anpfuel.data.remote.safety

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.safety.BlockReceipt
import com.anpfuel.domain.safety.BlockedUser
import com.anpfuel.domain.safety.CommunitySafetyException
import com.anpfuel.domain.safety.CommunitySafetyGateway
import com.anpfuel.domain.safety.SafetyContentKind
import com.anpfuel.domain.safety.SafetyContentRef
import com.anpfuel.domain.safety.UserReportReason
import com.anpfuel.domain.safety.UserReportReceipt
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
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import org.json.JSONArray
import org.json.JSONObject

/** 500 blocks with labels fit well below this; anything larger is refused unread. */
private const val SAFETY_BODY_BUDGET = 256L * 1024L + 1
private const val MAX_HIDDEN_BATCH = 100
private const val MAX_BLOCKS = 500
private val SAFETY_JSON = "application/json".toMediaType()

/**
 * Transport for the community safety contract: report a user, block,
 * list and remove blocks, and ask which shown content is hidden
 * (B-BR-SAFE-02..07).
 *
 * Every route is a signed session command exactly like Forum social
 * writes: the session envelope travels in the JSON body and the request
 * proof covers those exact bytes (B-BR-SAFE-03). No redirects, no
 * automatic retries, a 15 s call timeout and a bounded response body.
 * Refusals map to [CommunitySafetyException] by status and platform error
 * code; server bodies never reach exception messages.
 */
class CommunitySafetyHttpClient(
    http: OkHttpClient,
    baseUrl: String,
    private val sessions: AuthSessionStore? = null,
    private val auth: AuthFlow? = null,
    private val nowEpochSeconds: () -> Long = { System.currentTimeMillis() / 1000L },
    private val proofHeaders: (suspend (path: String, body: ByteArray) -> Map<String, String>)? = null,
) : CommunitySafetyGateway {

    private val origin = baseUrl.trimEnd('/').toHttpUrl()
    private val client = http.newBuilder()
        .followRedirects(false)
        .followSslRedirects(false)
        .retryOnConnectionFailure(false)
        .callTimeout(15, TimeUnit.SECONDS)
        .build()

    private data class SessionCredentials(val familyId: String, val accessToken: String)

    private fun liveSession(): SessionCredentials {
        val store = sessions ?: throw CommunitySafetyException.AuthRequired()
        val stored = try {
            if (auth == null) store.load() else when (val result = auth.refreshSession()) {
                is AuthApiResult.Ok -> result.value
                is AuthApiResult.Err -> null
            }
        } catch (_: Exception) {
            null
        }
        if (stored == null || !PortableAuth.isAccessLive(stored, nowEpochSeconds())) {
            throw CommunitySafetyException.AuthRequired()
        }
        return SessionCredentials(stored.familyId, stored.accessToken)
    }

    private fun envelope(): JSONObject {
        val session = liveSession()
        return JSONObject().apply {
            put("family_id", session.familyId)
            put("access_token", session.accessToken)
        }
    }

    override suspend fun reportUser(
        ref: SafetyContentRef,
        reason: UserReportReason,
        detail: String?,
    ): UserReportReceipt {
        val payload = envelope().apply {
            put("content_kind", ref.kind.wire)
            put("content_id", ref.id)
            put("reason", reason.wire)
            if (!detail.isNullOrBlank()) put("detail", detail)
        }
        val raw = signedPost(path("v1/community/user-reports"), payload)
        return parse(raw) { body ->
            val receipt = body.getJSONObject("receipt")
            UserReportReceipt(
                reportId = receipt.requiredString("report_id"),
                reason = UserReportReason.fromWire(receipt.optionalString("reason")),
                createdAt = receipt.requiredString("created_at"),
                replayed = receipt.optBoolean("replayed", false),
            )
        }
    }

    override suspend fun blockUser(ref: SafetyContentRef): BlockReceipt {
        val payload = envelope().apply {
            put("content_kind", ref.kind.wire)
            put("content_id", ref.id)
        }
        val raw = signedPost(path("v1/community/blocks"), payload)
        return parse(raw) { body ->
            BlockReceipt(
                blockedUser = blockedUser(body.getJSONObject("block")),
                replayed = body.optBoolean("replayed", false),
            )
        }
    }

    override suspend fun listBlocks(): List<BlockedUser> {
        val raw = signedPost(path("v1/community/blocks/list"), envelope())
        return parse(raw) { body ->
            val items = body.getJSONArray("items")
            if (items.length() > MAX_BLOCKS) throw IllegalStateException("too many blocks")
            List(items.length()) { index -> blockedUser(items.getJSONObject(index)) }
        }
    }

    override suspend fun removeBlock(blockId: String) {
        val url = origin.newBuilder()
            .addPathSegments("v1/community/blocks")
            .addPathSegment(blockId.trim())
            .addPathSegment("remove")
            .build()
        val raw = signedPost(url, envelope())
        val removed = parse(raw) { body -> body.getBoolean("removed") }
        if (!removed) throw CommunitySafetyException.Unavailable("safety.bad-response")
    }

    override suspend fun hiddenContent(refs: List<SafetyContentRef>): Set<SafetyContentRef> {
        val asked = refs.toSet()
        if (asked.isEmpty()) return emptySet()
        if (asked.size > MAX_HIDDEN_BATCH) throw CommunitySafetyException.BadRequest()
        val items = JSONArray()
        asked.forEach { ref ->
            items.put(JSONObject().apply {
                put("content_kind", ref.kind.wire)
                put("content_id", ref.id)
            })
        }
        val payload = envelope().apply { put("items", items) }
        val raw = signedPost(path("v1/community/blocks/hidden"), payload)
        return parse(raw) { body ->
            val hidden = body.getJSONArray("hidden")
            buildSet {
                for (index in 0 until hidden.length()) {
                    val item = hidden.getJSONObject(index)
                    val kind = SafetyContentKind.fromWire(item.optionalString("content_kind")) ?: continue
                    val id = item.optionalString("content_id") ?: continue
                    // The answer is a subset of the question; anything else is ignored.
                    SafetyContentRef(kind, id).takeIf { it in asked }?.let(::add)
                }
            }
        }
    }

    private fun path(segments: String): HttpUrl = origin.newBuilder().addPathSegments(segments).build()

    private fun blockedUser(json: JSONObject): BlockedUser = BlockedUser(
        blockId = json.requiredString("block_id"),
        label = json.optionalString("label").orEmpty(),
        sourceKind = SafetyContentKind.fromWire(json.optionalString("source_kind")),
        createdAt = json.requiredString("created_at"),
    )

    private inline fun <T> parse(raw: String, read: (JSONObject) -> T): T = try {
        read(JSONObject(raw))
    } catch (e: CommunitySafetyException) {
        throw e
    } catch (_: Exception) {
        throw CommunitySafetyException.Unavailable("safety.bad-response")
    }

    /** Signs the exact body bytes (B-BR-SAFE-03) and posts them once. */
    private suspend fun signedPost(url: HttpUrl, payload: JSONObject): String {
        val body = payload.toString().toByteArray(Charsets.UTF_8)
        val proof = proofHeaders?.let { sign ->
            try {
                sign(url.encodedPath, body)
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                throw CommunitySafetyException.Unavailable("safety.signing-unavailable")
            }
        }.orEmpty()
        val builder = Request.Builder()
            .url(url)
            .post(body.toRequestBody(SAFETY_JSON))
            .header("Accept", "application/json")
            .header("Cache-Control", "no-store")
        proof.forEach { (name, value) -> builder.header(name, value) }
        return execute(builder.build())
    }

    private suspend fun execute(request: Request): String = withContext(Dispatchers.IO) {
        suspendCancellableCoroutine { continuation ->
            val call = client.newCall(request)
            continuation.invokeOnCancellation { call.cancel() }
            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    if (continuation.isActive) continuation.resumeWithException(CommunitySafetyException.Offline())
                }

                override fun onResponse(call: Call, response: Response) {
                    response.use {
                        if (!continuation.isActive) return
                        try {
                            continuation.resume(readOrRefuse(response))
                        } catch (e: IOException) {
                            if (continuation.isActive) continuation.resumeWithException(CommunitySafetyException.Offline())
                        } catch (e: Exception) {
                            if (continuation.isActive) continuation.resumeWithException(e)
                        }
                    }
                }
            })
        }
    }

    private fun readOrRefuse(response: Response): String {
        val source = response.body?.source() ?: throw CommunitySafetyException.Unavailable()
        if (source.request(SAFETY_BODY_BUDGET)) throw CommunitySafetyException.Unavailable("safety.over-budget")
        val body = source.readUtf8()
        return when (response.code) {
            200 -> body
            400 -> when (val code = errorCode(body)) {
                "safety.self-target" -> throw CommunitySafetyException.SelfTarget()
                else -> throw CommunitySafetyException.BadRequest(code?.takeIf { it.startsWith("safety.") } ?: "safety.bad-request")
            }
            401 -> throw CommunitySafetyException.AuthRequired()
            404 -> throw CommunitySafetyException.NotFound()
            409 -> throw CommunitySafetyException.BlockLimit()
            429 -> throw CommunitySafetyException.RateLimited(
                response.header("Retry-After")?.trim()?.toLongOrNull()?.takeIf { it >= 0 },
            )
            else -> throw CommunitySafetyException.Unavailable()
        }
    }

    private fun errorCode(body: String): String? = try {
        JSONObject(body).optJSONObject("error")?.optionalString("code")
    } catch (_: Exception) {
        null
    }
}

private fun JSONObject.optionalString(name: String): String? =
    if (isNull(name)) null else opt(name) as? String

private fun JSONObject.requiredString(name: String): String =
    optionalString(name)?.takeIf { it.isNotBlank() } ?: throw IllegalStateException("missing $name")
