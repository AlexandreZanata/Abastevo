package com.anpfuel.data.remote

import com.anpfuel.application.portable.PhotoCache
import com.anpfuel.data.local.dao.PhotoUploadSessionDao
import com.anpfuel.data.local.entity.PhotoUploadSessionEntity
import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.exception.ContributionPhotoExpired
import com.anpfuel.domain.exception.ContributionPhotoRejected
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.portable.PortablePhoto
import com.anpfuel.domain.repository.ContributionReceipt
import com.anpfuel.domain.repository.ContributionRemoteStatus
import com.anpfuel.domain.repository.ContributionSubmissionGateway
import com.anpfuel.domain.valueobject.FuelProduct
import java.io.IOException
import java.security.MessageDigest
import java.time.Instant
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/** Signed immutable row commands share one durable media negotiation. No expired-photo fallback. */
class ContributionUploadHttpClient(
    private val proof: PhotoProofTransport,
    private val photoCache: PhotoCache,
    private val sessions: PhotoUploadSessionDao,
    client: OkHttpClient,
    private val nowMillis: () -> Long = System::currentTimeMillis,
) : ContributionSubmissionGateway {
    private val mediaMutex = Mutex()
    private val privateClient = client.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).callTimeout(20, java.util.concurrent.TimeUnit.SECONDS).cookieJar(okhttp3.CookieJar.NO_COOKIES)
        .authenticator(okhttp3.Authenticator.NONE).proxyAuthenticator(okhttp3.Authenticator.NONE).build()

    private data class Intent(val doc: JSONObject, val scope: ContributionScope, val product: FuelProduct,
        val captureId: String?, val expiresAt: Long?, val photoId: String?, val capturedAt: Long) {
        fun wire(evidenceId: String?): JSONObject = JSONObject()
            .put("client_submission_id", doc.getString("client_submission_id"))
            .put("station_id", doc.getString("station_id")).put("fuel_product", WireFuelMapper.toWire(product))
            .put("price", JSONObject().put("amount_milli_brl", doc.getLong("amount_milli_brl"))
                .put("currency", "BRL").put("unit", unit(product)))
            .put("condition", JSONObject().put("kind", doc.getString("condition_kind")))
            .put("claimed_captured_at", Instant.ofEpochMilli(capturedAt).toString()).also {
                if (evidenceId != null) it.put("evidence_id", evidenceId).put("photo_capture_id", captureId)
                if (!doc.isNull("supersedes_observation_id")) it.put("supersedes_observation_id", doc.getString("supersedes_observation_id"))
            }
    }

    private suspend fun intent(payload: String, commandId: String): Intent {
        val value = try {
            val doc = JSONObject(payload)
            val scoped = doc.getJSONObject("scope")
            val scope = ContributionScope(scoped.getString("owner_scope"), scoped.getString("origin"))
            val product = FuelProduct.valueOf(doc.getString("fuel_product"))
            val amount = integer(doc, "amount_milli_brl")
            val capturedAt = integer(doc, "captured_at_millis")
            val photo = if (doc.isNull("photo_id")) null else doc.getString("photo_id").also { require(it.length in 1..128) }
            val context = if (doc.isNull("photo_capture")) null else doc.getJSONObject("photo_capture")
            require((photo == null) == (context == null))
            val captureId = context?.getString("capture_id")?.also { requireUuid(it) }
            val deadline = context?.let { integer(it, "expires_at_millis") }
            require(doc.getString("client_submission_id") == commandId && commandId.length in 1..128)
            require(doc.getString("currency") == "BRL" && doc.getString("unit") in setOf(unit(product), "BRL/${unit(product)}"))
            require(doc.getString("condition_kind") in setOf("STANDARD", "CASH", "DEBIT", "CREDIT", "APP", "LOYALTY", "OTHER"))
            if (photo != null) require(doc.getString("condition_kind") == "STANDARD")
            ContributionDraft.create(commandId, doc.getString("station_id"), product, amount, "BRL", unit(product),
                doc.getString("condition_kind"), capturedAt, photo,
                if (doc.isNull("supersedes_observation_id")) null else doc.getString("supersedes_observation_id"))
            Intent(doc, scope, product, captureId, deadline, photo, capturedAt)
        } catch (_: Exception) { throw DomainException("contribution.invalid-or-unscoped") }
        checkScope(value)
        return value
    }

    private suspend fun checkScope(intent: Intent) {
        if (intent.scope.origin != proof.origin || intent.scope.ownerScope != proof.localScope()) {
            throw DomainException("contribution.owner-or-origin-changed")
        }
    }

    override suspend fun submit(commandId: String, revision: Int, payloadJson: String, nonce: String): ContributionReceipt {
        require(revision > 0 && nonce.isNotBlank())
        val intent = intent(payloadJson, commandId)
        val evidence = if (intent.photoId == null) null else mediaMutex.withLock { upload(intent) }
        checkScope(intent)
        val received = proof.post("/v1/observations", commandId, intent.wire(evidence), intent.scope.ownerScope)
        val id = received.getString("id").also(::requireUuid)
        // Intake acknowledges only RECEIVED. A substring in an error/decision is never validation.
        if (received.getString("validation_state") != "RECEIVED") throw IOException("contribution.invalid-receipt")
        return ContributionReceipt(commandId, revision, ContributionRemoteStatus.RECEIVED, id)
    }

    override suspend fun refresh(receipt: ContributionReceipt, payloadJson: String): ContributionReceipt {
        val intent = intent(payloadJson, receipt.commandId)
        val id = receipt.observationId?.also(::requireUuid) ?: throw DomainException("contribution.missing-receipt")
        val response = proof.get("/v1/observations/$id", intent.scope.ownerScope)
        val observation = response.getJSONObject("observation")
        if (observation.getString("id") != id || observation.getString("station_id") != intent.doc.getString("station_id") ||
            observation.getString("fuel_product") != WireFuelMapper.toWire(intent.product) ||
            observation.getString("unit") != unit(intent.product) || integer(observation, "amount_milli_brl") != integer(intent.doc,"amount_milli_brl") ||
            observation.getJSONObject("condition").getString("kind") != intent.doc.getString("condition_kind")) {
            throw IOException("contribution.receipt-mismatch")
        }
        val status = when (response.getString("validation_state")) {
            "RECEIVED" -> ContributionRemoteStatus.RECEIVED
            "VALIDATING" -> ContributionRemoteStatus.VALIDATING
            "VALIDATED" -> ContributionRemoteStatus.VALIDATED
            "REJECTED" -> ContributionRemoteStatus.REJECTED
            else -> throw IOException("contribution.invalid-status")
        }
        if (observation.getString("validation_state") != status.name) throw IOException("contribution.status-mismatch")
        return receipt.copy(status = status, reason = if (status == ContributionRemoteStatus.REJECTED) "contribution.rejected" else null)
    }

    private suspend fun upload(intent: Intent): String {
        checkScope(intent)
        val capture = requireNotNull(intent.captureId)
        var saved = sessions.find(capture)
        saved?.let {
            if (it.photoId != intent.photoId || it.ownerScope != intent.scope.ownerScope || it.origin != intent.scope.origin ||
                it.capturedAtMillis != intent.capturedAt || it.expiresAtMillis > requireNotNull(intent.expiresAt)) {
                throw DomainException("contribution.media-intent-changed")
            }
            // Replay a previously READY observation after a lost acknowledgement, without another upload.
            it.evidenceId?.let { evidence -> requireUuid(evidence); return evidence }
        }
        if (nowMillis() >= requireNotNull(intent.expiresAt) || nowMillis() - intent.capturedAt >= 86_400_000L) throw ContributionPhotoExpired()
        if (saved != null) {
            readyOrPending(intent, saved, proof.get("/v1/uploads/${saved.sessionId}", intent.scope.ownerScope))?.let { return it }
        }
        val bytes = withContext(Dispatchers.IO) { photoCache.get(requireNotNull(intent.photoId)) } ?: throw ContributionPhotoExpired()
        if (!PortablePhoto.fitsWireCap(bytes.size.toLong())) throw DomainException("contribution.photo-size")
        val hash = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }
        if (saved != null && saved.sha256 != hash) throw DomainException("contribution.photo-changed")
        val doc = JSONObject().put("client_submission_id", "photo:$capture").put("content_type", "image/jpeg")
            .put("size_bytes", bytes.size).put("sha256", hash).put("photo_capture_id", capture)
            .put("station_id", intent.doc.getString("station_id")).put("captured_at", Instant.ofEpochMilli(intent.capturedAt).toString())
        val reserve = proof.post("/v1/uploads", "photo:$capture", doc, intent.scope.ownerScope)
        val sessionId = reserve.getString("upload_id").also(::requireUuid)
        val deadline = Instant.parse(reserve.getString("session_expires_at")).toEpochMilli()
        if (deadline <= nowMillis() || deadline > intent.expiresAt) throw IOException("contribution.invalid-media-deadline")
        saved = sessions.remember(PhotoUploadSessionEntity(capture, sessionId, requireNotNull(intent.photoId),
            intent.scope.ownerScope, intent.scope.origin, intent.capturedAt, deadline, hash))
        checkScope(intent)
        put(reserve, bytes, deadline)
        checkScope(intent)
        val status = proof.post("/v1/uploads/$sessionId/complete", "complete:$capture", JSONObject(), intent.scope.ownerScope)
        return readyOrPending(intent, saved, status) ?: throw IOException("contribution.media-not-ready")
    }

    private suspend fun readyOrPending(intent: Intent, saved: PhotoUploadSessionEntity, result: JSONObject): String? {
        if (result.getString("upload_id") != saved.sessionId) throw IOException("contribution.media-receipt-mismatch")
        return when (result.getString("state")) {
            "READY" -> {
                val evidence = result.getString("evidence_id").also(::requireUuid)
                checkScope(intent)
                if (sessions.ready(saved.captureId, saved.sessionId, evidence) != 1) throw DomainException("contribution.media-receipt-changed")
                evidence
            }
            "ISSUED" -> null
            "VERIFYING" -> throw IOException("contribution.media-verifying")
            "EXPIRED" -> throw ContributionPhotoExpired()
            "REJECTED" -> throw ContributionPhotoRejected()
            else -> throw IOException("contribution.invalid-media-status")
        }
    }

    private suspend fun put(reserve: JSONObject, bytes: ByteArray, deadline: Long) = withContext(Dispatchers.IO) {
        val url = reserve.getString("url")
        val uri = java.net.URI(url)
        val expiry = Instant.parse(reserve.getString("expires_at")).toEpochMilli()
        val headers = reserve.getJSONObject("required_headers")
        if (reserve.getString("method") != "PUT" || url.length > 8192 || uri.scheme != "https" || uri.host.isNullOrBlank() ||
            uri.rawUserInfo != null || uri.rawFragment != null || uri.rawQuery.isNullOrBlank() || expiry <= nowMillis() ||
            expiry > nowMillis() + 300_000 || expiry > deadline || integer(reserve, "max_bytes") < bytes.size ||
            headers.length() != 1 || headers.optString("content-type") != "image/jpeg") {
            throw IOException("contribution.invalid-upload-authorization")
        }
        val request = Request.Builder().url(url).put(bytes.toRequestBody("image/jpeg".toMediaType())).build()
        privateClient.newCall(request).execute().use { if (!it.isSuccessful) throw IOException("contribution.media-put-failed:${it.code}") }
    }

    companion object {
        private fun requireUuid(value: String) { require(value.matches(Regex("[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}"))) { "invalid identifier" } }
        private fun integer(doc: JSONObject, key: String): Long = when (val value = doc.get(key)) {
            is Int -> value.toLong()
            is Long -> value
            else -> throw DomainException("contribution.non-integer")
        }
        private fun unit(product: FuelProduct) = when(product) { FuelProduct.CNG -> "M3"; FuelProduct.LPG_P13 -> "KG_13"; else -> "L" }
    }
}
