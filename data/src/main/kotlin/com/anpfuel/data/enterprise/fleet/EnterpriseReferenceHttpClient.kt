package com.anpfuel.data.enterprise.fleet

import com.anpfuel.application.enterprise.EnterpriseAccessBinding
import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.trip.TripFieldReader
import com.anpfuel.application.enterprise.trip.TripFieldReference
import com.anpfuel.data.enterprise.trip.TripFieldWire
import com.anpfuel.domain.enterprise.EnterprisePartition
import org.json.JSONObject
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceNotFound
import com.anpfuel.application.enterprise.fleet.EnterpriseReferencePull
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceTransport
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueDeliveryFailure
import com.anpfuel.data.enterprise.EnterpriseDeviceSigner
import com.anpfuel.data.enterprise.EnterpriseOperationSessionSource
import com.anpfuel.data.enterprise.EnterpriseRetryAfter
import com.anpfuel.data.enterprise.awaitCompanyResponse
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedRequest
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedRequestSnapshot
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedResponse
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

/**
 * Signed pull of the member's reference feed (EMP-OFFL-006 route, EDR-225) on the same device
 * proof plane as the review results; never a company cookie read. The whole answer is bound to
 * the request ID, the exact member/enrollment and the held revision before anything is returned.
 */
class EnterpriseReferenceHttpClient(
    http: OkHttpClient,
    private val source: EnterpriseOperationSessionSource,
    private val signer: EnterpriseDeviceSigner,
    private val newId: () -> String = { UUID.randomUUID().toString() },
) : EnterpriseReferenceTransport, TripFieldReader {
    private val http = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).cookieJar(CookieJar.NO_COOKIES).cache(null)
        .authenticator(Authenticator.NONE).proxyAuthenticator(Authenticator.NONE)
        .callTimeout(30, TimeUnit.SECONDS).build()

    override suspend fun pull(attempt: EnterpriseQueueAttempt, cursor: String?, heldRevision: Long?): EnterpriseReferencePull =
        withContext(Dispatchers.IO) {
            try {
                attempt.assertCurrent()
                val partition = attempt.partition
                if (partition.origin != source.apiOrigin) throw EnterpriseLocalDenied()
                if ((cursor == null) != (heldRevision == null)) throw EnterpriseQueueDeliveryFailure()
                val session = source.proof(attempt)
                val request = ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(newId(), session.familyId, session.accessToken,
                    attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId, cursor))
                val raw = request.canonicalBytes
                val path = "/v1/orgs/${partition.organizationId}/sync/reference"
                val binding = EnterpriseAccessBinding(session.accountId, source.apiOrigin, session.familyId, attempt.account.epoch)
                if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
                val headers = signer.headers(binding, path, raw.copyOf()).toMap()
                require(headers.keys == PROOF_HEADERS && headers.values.all { value ->
                    value.length in 1..2048 && value.all { it.code in 33..126 }
                })
                if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
                val call = Request.Builder().url(source.apiOrigin + path).header("Origin", source.apiOrigin)
                    .header("Accept", "application/json").header("Cache-Control", "no-store")
                    .post(raw.toRequestBody(JSON)).apply { headers.forEach { (name, value) -> header(name, value) } }.build()
                attempt.assertCurrent()
                awaitCompanyResponse(http.newCall(call)).use { response ->
                    if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
                    val retryAfter = EnterpriseRetryAfter.parse(response.headers.values("Retry-After").singleOrNull(),
                        response.headers.values("Date").singleOrNull())
                    if (response.code == 404) throw EnterpriseReferenceNotFound()
                    require(response.headers.values("Set-Cookie").isEmpty() && response.headers.values("ETag").isEmpty() &&
                        response.headers.values("Cache-Control").singleOrNull()?.equals("no-store", true) == true)
                    val body = response.body ?: throw EnterpriseQueueDeliveryFailure(retryAfter)
                    require(response.headers.values("Content-Type").size == 1 && body.contentType()?.let {
                        it.type == "application" && it.subtype == "json"
                    } == true)
                    if (response.code != 200) throw EnterpriseQueueDeliveryFailure(retryAfter)
                    val input = body.source()
                    require(!input.request(ReferenceFeedResponse.MAX_BYTES.toLong() + 1))
                    val decoded = ReferenceFeedResponse.parse(input.readByteArray(), request, heldRevision)
                    if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
                    EnterpriseReferencePull(request, decoded)
                }
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (denied: EnterpriseLocalDenied) { throw denied }
            catch (notFound: EnterpriseReferenceNotFound) { throw notFound }
            catch (failure: EnterpriseQueueDeliveryFailure) { throw failure }
            catch (_: Exception) { throw EnterpriseQueueDeliveryFailure() }
        }

    override suspend fun read(context: EnterpriseContextSummary, tripId: String): TripFieldReference? =
        withContext(Dispatchers.IO) {
            EnterprisePartition.requireId(tripId)
            val attempt = source.attempt(source.deliveryAccount(), context.partition)
            if (attempt.enrollment.target.membershipId != context.membershipId || context.partition.origin != source.apiOrigin) {
                throw EnterpriseLocalDenied()
            }
            val session = source.proof(attempt)
            val id = newId().also(EnterprisePartition::requireId)
            val raw = JSONObject().put("schema_version", 1).put("request_id", id)
                .put("family_id", session.familyId).put("access_token", session.accessToken)
                .put("membership_id", context.membershipId).put("enrollment_id", attempt.enrollment.enrollmentId)
                .put("cursor", JSONObject.NULL).put("trip_id", tripId).toString().encodeToByteArray()
            val path = "/v1/orgs/${context.partition.organizationId}/sync/reference"
            val binding = EnterpriseAccessBinding(session.accountId, source.apiOrigin, session.familyId, attempt.account.epoch)
            val headers = signer.headers(binding, path, raw.copyOf()).toMap()
            require(headers.keys == PROOF_HEADERS && headers.values.all { value ->
                value.length in 1..2048 && value.all { it.code in 33..126 }
            })
            if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
            val call = Request.Builder().url(source.apiOrigin + path).header("Origin", source.apiOrigin)
                .header("Accept", "application/json").header("Cache-Control", "no-store")
                .post(raw.toRequestBody(JSON)).apply { headers.forEach { (name, value) -> header(name, value) } }.build()
            awaitCompanyResponse(http.newCall(call)).use { response ->
                if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
                if (response.code == 404) return@use null
                require(response.code == 200 && response.headers.values("Set-Cookie").isEmpty() &&
                    response.headers.values("ETag").isEmpty() &&
                    response.headers.values("Cache-Control").singleOrNull()?.equals("no-store", true) == true)
                val body = requireNotNull(response.body)
                require(body.contentType()?.let { it.type == "application" && it.subtype == "json" } == true)
                val input = body.source()
                require(!input.request(TripFieldWire.MAX_BYTES.toLong() + 1))
                TripFieldWire.read(input.readUtf8(), id, context.membershipId, attempt.enrollment.enrollmentId, tripId).also {
                    if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
                }
            }
        }

    override fun toString(): String = "EnterpriseReferenceHttpClient(redacted)"

    private companion object {
        val JSON = "application/json".toMediaType()
        val PROOF_HEADERS = setOf("Signature", "Signature-Created", "Signature-Expires", "Signature-Keyid", "Signature-Nonce")
    }
}
