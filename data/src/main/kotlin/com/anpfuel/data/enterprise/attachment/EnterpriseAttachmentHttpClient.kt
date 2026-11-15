package com.anpfuel.data.enterprise.attachment

import com.anpfuel.application.enterprise.EnterpriseAccessBinding
import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.attachment.AttachmentSessionResult
import com.anpfuel.application.enterprise.attachment.AttachmentTransferResult
import com.anpfuel.application.enterprise.attachment.EnterpriseAttachmentTransport
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueDeliveryFailure
import com.anpfuel.data.enterprise.EnterpriseDeviceSigner
import com.anpfuel.data.enterprise.EnterpriseOperationSessionSource
import com.anpfuel.data.enterprise.EnterpriseRetryAfter
import com.anpfuel.data.enterprise.awaitCompanyResponse
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.attachment.AttachmentPolicy
import com.anpfuel.domain.enterprise.attachment.AttachmentRefusal
import com.anpfuel.domain.enterprise.attachment.AttachmentRequestSnapshot
import com.anpfuel.domain.enterprise.attachment.AttachmentSessionRequest
import com.anpfuel.domain.enterprise.attachment.AttachmentSessionResponse
import com.anpfuel.domain.enterprise.attachment.AttachmentStorage
import com.anpfuel.domain.enterprise.attachment.AttachmentUploadGrant
import com.anpfuel.domain.enterprise.attachment.EnterpriseAttachment
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.SyncJson
import java.util.UUID
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.Authenticator
import okhttp3.CookieJar
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response

/**
 * The signed attachment session (EDR-270) on the same device proof plane as the reference feed, and the
 * capability-bound EMP-DOC-002 ingress, which takes no cookie, signature or Origin. Never a company cookie path.
 * Only the closed final refusals of B-BR-EMP-417 become [AttachmentSessionResult.Refused]; every other answer,
 * including 401/404, is a retryable [EnterpriseQueueDeliveryFailure].
 */
class EnterpriseAttachmentHttpClient(
    http: OkHttpClient,
    private val source: EnterpriseOperationSessionSource,
    private val signer: EnterpriseDeviceSigner,
    private val newId: () -> String = { UUID.randomUUID().toString() },
) : EnterpriseAttachmentTransport {
    private val http = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).cookieJar(CookieJar.NO_COOKIES).cache(null)
        .authenticator(Authenticator.NONE).proxyAuthenticator(Authenticator.NONE)
        .callTimeout(60, TimeUnit.SECONDS).build()

    private suspend fun <T> guarded(work: suspend () -> T): T = withContext(Dispatchers.IO) {
        try { work() }
        catch (cancelled: CancellationException) { throw cancelled }
        catch (denied: EnterpriseLocalDenied) { throw denied }
        catch (failure: EnterpriseQueueDeliveryFailure) { throw failure }
        catch (_: Exception) { throw EnterpriseQueueDeliveryFailure() }
    }

    private fun checkedHeaders(response: Response) {
        require(response.headers.values("Set-Cookie").isEmpty() && response.headers.values("ETag").isEmpty() &&
            response.headers.values("Cache-Control").singleOrNull()?.equals("no-store", true) == true)
        require(response.headers.values("Content-Type").size == 1 && response.body?.contentType()?.let {
            it.type == "application" && it.subtype == "json"
        } == true)
    }

    private fun body(response: Response, limit: Int): ByteArray {
        val input = (response.body ?: throw EnterpriseQueueDeliveryFailure()).source()
        require(!input.request(limit.toLong() + 1))
        return input.readByteArray()
    }

    private fun errorCode(raw: ByteArray): String? = try {
        val root = CanonicalJson.parse(raw, ERROR_BYTES) as SyncJson.Object
        ((root.fields["error"] as SyncJson.Object).fields["code"] as SyncJson.Text).value
    } catch (_: Exception) { null }

    override suspend fun open(attempt: EnterpriseQueueAttempt, attachment: EnterpriseAttachment): AttachmentSessionResult = guarded {
        attempt.assertCurrent()
        val partition = attempt.partition
        if (partition.origin != source.apiOrigin) throw EnterpriseLocalDenied()
        val session = source.proof(attempt)
        val request = AttachmentSessionRequest.create(AttachmentRequestSnapshot(newId(), session.familyId, session.accessToken,
            attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId, attachment.declarationId, attachment.documentId,
            AttachmentPolicy.KIND, attachment.sha256, attachment.bytes, AttachmentPolicy.MEDIA_TYPE, attachment.linkId,
            attachment.target.kind.name, attachment.target.id))
        val raw = request.canonicalBytes
        val path = "/v1/orgs/${partition.organizationId}/sync/attachments"
        val binding = EnterpriseAccessBinding(session.accountId, source.apiOrigin, session.familyId, attempt.account.epoch)
        if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
        val headers = signer.headers(binding, path, raw.copyOf()).toMap()
        require(headers.keys == PROOF_HEADERS && headers.values.all { value -> value.length in 1..2048 && value.all { it.code in 33..126 } })
        if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
        val call = Request.Builder().url(source.apiOrigin + path).header("Origin", source.apiOrigin)
            .header("Accept", "application/json").header("Cache-Control", "no-store")
            .post(raw.toRequestBody(JSON)).apply { headers.forEach { (name, value) -> header(name, value) } }.build()
        attempt.assertCurrent()
        awaitCompanyResponse(http.newCall(call)).use { response ->
            if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
            val retryAfter = EnterpriseRetryAfter.parse(response.headers.values("Retry-After").singleOrNull(), response.headers.values("Date").singleOrNull())
            checkedHeaders(response)
            if (response.code != 200) {
                val code = errorCode(body(response, ERROR_BYTES))
                val refusal = REFUSALS[response.code to code] ?: throw EnterpriseQueueDeliveryFailure(retryAfter)
                return@guarded AttachmentSessionResult.Refused(refusal)
            }
            val decoded = AttachmentSessionResponse.parse(body(response, AttachmentSessionResponse.MAX_BYTES), request)
            if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
            AttachmentSessionResult.Opened(decoded)
        }
    }

    override suspend fun upload(attempt: EnterpriseQueueAttempt, documentId: String, grant: AttachmentUploadGrant, bytes: ByteArray): AttachmentTransferResult = guarded {
        attempt.assertCurrent()
        EnterprisePartition.requireId(documentId)
        val partition = attempt.partition
        if (partition.origin != source.apiOrigin) throw EnterpriseLocalDenied()
        // The ingress needs no account proof; the custody and session still bound this transfer.
        val session = source.proof(attempt)
        val path = "/v1/orgs/${partition.organizationId}/documents/$documentId/content/upload"
        val call = Request.Builder().url(source.apiOrigin + path).header("Accept", "application/json")
            .header(CAPABILITY_HEADER, grant.capability).post(bytes.copyOf().toRequestBody(JPEG)).build()
        attempt.assertCurrent()
        awaitCompanyResponse(http.newCall(call)).use { response ->
            if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
            val retryAfter = EnterpriseRetryAfter.parse(response.headers.values("Retry-After").singleOrNull(), response.headers.values("Date").singleOrNull())
            checkedHeaders(response)
            if (response.code in REJECTED_TRANSFERS) return@guarded AttachmentTransferResult.Rejected(retryAfter)
            if (response.code != 200) throw EnterpriseQueueDeliveryFailure(retryAfter)
            val root = CanonicalJson.parse(body(response, INGRESS_BYTES), INGRESS_BYTES) as SyncJson.Object
            val document = root.fields["document"] as SyncJson.Object
            val storage = ((document.fields["statuses"] as SyncJson.Object).fields["storage"] as SyncJson.Text).value
            val revision = ((document.fields["revisions"] as SyncJson.Object).fields["storage"] as SyncJson.Integer).lexeme.toLong()
            require((document.fields["id"] as SyncJson.Text).value == documentId && revision >= 1)
            AttachmentTransferResult.Settled(AttachmentStorage.entries.first { it.name == storage }, revision)
        }
    }

    override fun toString(): String = "EnterpriseAttachmentHttpClient(redacted)"

    private companion object {
        val JSON = "application/json".toMediaType()
        val JPEG = "image/jpeg".toMediaType()
        const val CAPABILITY_HEADER = "X-Abastevo-Upload-Capability"
        const val ERROR_BYTES = 8192
        const val INGRESS_BYTES = 16384
        val PROOF_HEADERS = setOf("Signature", "Signature-Created", "Signature-Expires", "Signature-Keyid", "Signature-Nonce")
        /** Final, closed refusals of the session (B-BR-EMP-417); everything else is retried. */
        val REFUSALS = mapOf((403 to "FORBIDDEN") to AttachmentRefusal.FORBIDDEN, (409 to "CONFLICT") to AttachmentRefusal.CONFLICT,
            (409 to "OPERATION_ID_REUSED") to AttachmentRefusal.OPERATION_ID_REUSED, (422 to "VALIDATION_FAILED") to AttachmentRefusal.VALIDATION_FAILED)
        /** A refused transfer (capability, integrity, state) leaves the document pending; the next session decides. */
        val REJECTED_TRANSFERS = setOf(401, 404, 409, 422)
    }
}
