package com.anpfuel.data.remote

import com.anpfuel.application.portable.PhotoCache
import com.anpfuel.domain.portable.PortablePhoto
import com.anpfuel.domain.repository.ContributionReceipt
import com.anpfuel.domain.repository.ContributionRemoteStatus
import com.anpfuel.domain.repository.ContributionSubmissionGateway
import java.io.IOException
import javax.inject.Inject
import javax.inject.Singleton
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/**
 * P10-T05 bounded direct-media submission client (BUC-004, B-BR-010/011).
 *
 * Network-only: transport failures, non-2xx responses and empty bodies
 * throw [IOException]; retry/backoff lives in the worker, never here.
 * Direct media flow is reserve → presigned PUT → complete → observation
 * submit with the same stable `client_submission_id` and a fresh `nonce`
 * per send (headers `Idempotency-Key` / `X-Nonce`). Object success with
 * finalize failure throws so the command stays FAILED and retries the
 * same id with a new nonce (never a duplicate observation). An expired or
 * missing transient photo falls back to metadata-only with the payload's
 * own historical label intact — an old photo is never relabelled as fresh.
 * No contributor id, GPS, EXIF or signed URL is logged (B-BR-011); HTTPS
 * only and the preview base URL never resolves until deployment config.
 */
@Singleton
class ContributionUploadHttpClient @Inject constructor(
    private val client: OkHttpClient,
    private val photoCache: PhotoCache,
) : ContributionSubmissionGateway {

    var baseUrl: String = PREVIEW_BASE_URL
        internal set

    constructor(
        client: OkHttpClient,
        baseUrl: String,
        photoCache: PhotoCache,
    ) : this(client, photoCache) {
        this.baseUrl = baseUrl
    }

    override suspend fun submit(
        commandId: String,
        revision: Int,
        payloadJson: String,
        nonce: String,
    ): ContributionReceipt {
        require(commandId.isNotBlank()) { "command_id is blank" }
        require(nonce.isNotBlank()) { "nonce is blank" }
        val photoId = parsePhotoId(payloadJson)
        val evidenceId = if (photoId == null) {
            null
        } else {
            uploadPhoto(photoId, nonce)
        }
        val status = postObservation(commandId, payloadJson, nonce, evidenceId)
        return ContributionReceipt(commandId, revision, status)
    }

    private fun uploadPhoto(photoId: String, nonce: String): String? {
        val bytes = photoCache.get(photoId) ?: return null
        if (!PortablePhoto.fitsWireCap(bytes.size.toLong())) {
            throw IOException("photo over wire cap")
        }
        val uploadId = reserveUpload(nonce)
        putBytes(uploadId, bytes)
        completeUpload(uploadId, bytes.size.toLong(), nonce)
        return uploadId
    }

    private fun reserveUpload(nonce: String): String {
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + "/v1/uploads")
            .post("{}".toRequestBody(JSON_MEDIA))
            .header("Accept", "application/json")
            .header("X-Nonce", nonce)
            .build()
        val body = execute(request)
        val id = try {
            JSONObject(body).optString("upload_id", "")
        } catch (_: Exception) {
            ""
        }
        if (id.isBlank()) throw IOException("upload reserve failed: empty id")
        return id
    }

    private fun putBytes(uploadId: String, bytes: ByteArray) {
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + "/v1/uploads/" + encode(uploadId) + "/bytes")
            .put(bytes.toRequestBody(JPEG_MEDIA))
            .header("Accept", "application/json")
            .build()
        execute(request)
    }

    private fun completeUpload(uploadId: String, bytes: Long, nonce: String) {
        val payload = JSONObject()
            .put("bytes", bytes)
            .toString()
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + "/v1/uploads/" + encode(uploadId) + "/complete")
            .post(payload.toRequestBody(JSON_MEDIA))
            .header("Accept", "application/json")
            .header("X-Nonce", nonce)
            .build()
        execute(request)
    }

    private fun postObservation(
        commandId: String,
        payloadJson: String,
        nonce: String,
        evidenceId: String?,
    ): ContributionRemoteStatus {
        val doc = try {
            JSONObject(payloadJson)
        } catch (error: Exception) {
            throw IOException("malformed contribution payload", error)
        }
        if (evidenceId != null) {
            doc.put("evidence_id", evidenceId)
        }
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + "/v1/observations")
            .post(doc.toString().toRequestBody(JSON_MEDIA))
            .header("Accept", "application/json")
            .header("Idempotency-Key", commandId)
            .header("X-Nonce", nonce)
            .build()
        val body = execute(request)
        return if (body.contains("\"VALIDATED\"")) {
            ContributionRemoteStatus.VALIDATED
        } else {
            ContributionRemoteStatus.RECEIVED
        }
    }

    private fun execute(request: Request): String {
        try {
            client.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    throw IOException("contribution submit failed: HTTP ${response.code}")
                }
                val body = response.body?.string()
                if (body.isNullOrBlank()) {
                    throw IOException("contribution submit failed: empty body")
                }
                return body
            }
        } catch (error: IOException) {
            throw error
        } catch (error: Exception) {
            throw IOException("contribution submit failed", error)
        }
    }

    private fun parsePhotoId(payloadJson: String): String? {
        return try {
            val id = JSONObject(payloadJson).optString("photo_id", "")
            id.ifBlank { null }
        } catch (_: Exception) {
            null
        }
    }

    private fun encode(value: String): String =
        java.net.URLEncoder.encode(value, Charsets.UTF_8.name())

    companion object {
        const val PREVIEW_BASE_URL = "https://api.anpfuel.example.invalid"
        private val JSON_MEDIA = "application/json; charset=utf-8".toMediaType()
        private val JPEG_MEDIA = "image/jpeg".toMediaType()
    }
}
