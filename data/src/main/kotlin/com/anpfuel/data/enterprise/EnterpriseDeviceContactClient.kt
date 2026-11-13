package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.SyncJson
import com.anpfuel.domain.portable.PortableAuth
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

fun interface EnterpriseDeviceSigner {
    suspend fun headers(binding: EnterpriseAccessBinding, path: String, body: ByteArray): Map<String, String>
}

/** Reuses the existing identity-v1 key, contributor registration, binding and nonce implementation. */
class EnterpriseIdentityV1ContactSigner(private val proof: PhotoProofTransport) : EnterpriseDeviceSigner {
    override suspend fun headers(binding: EnterpriseAccessBinding, path: String, body: ByteArray): Map<String, String> {
        if (EnterprisePartition.canonicalOrigin(proof.origin) != binding.origin) throw EnterpriseLocalDenied()
        val scope = proof.identityScope()
        if (!scope.startsWith("account:${binding.accountId}:")) throw EnterpriseLocalDenied()
        val headers = proof.postProofHeaders(path, body)
        if (proof.ownerScope() != scope) throw EnterpriseLocalDenied()
        return headers
    }
    override fun toString(): String = "EnterpriseIdentityV1ContactSigner(redacted)"
}

/** Separate signed custody plane; company cookies never supply account or device proof. */
class EnterpriseDeviceContactClient(
    http: OkHttpClient,
    origin: String,
    private val sessions: AuthSessionStore,
    private val auth: AuthFlow,
    private val fence: EnterpriseSessionFence,
    private val signer: EnterpriseDeviceSigner,
    private val newId: () -> String = { UUID.randomUUID().toString() },
) : com.anpfuel.application.enterprise.EnterpriseContactAuthority {
    private val origin = EnterprisePartition.canonicalOrigin(origin)
    private val http = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).cookieJar(CookieJar.NO_COOKIES).cache(null)
        .authenticator(Authenticator.NONE).proxyAuthenticator(Authenticator.NONE)
        .callTimeout(20, TimeUnit.SECONDS).build()

    private class Source(val request: EnterpriseContactRequest, val session: PortableAuth.Session, val background: Long) {
        override fun toString(): String = "EnterpriseContactSource(redacted)"
    }
    private class Reply(val status: Int, val fields: Map<String, SyncJson>) {
        override fun toString(): String = "EnterpriseContactTransportReply(redacted)"
    }

    private fun refused(error: EnterpriseAccessError = EnterpriseAccessError.UNAVAILABLE): Nothing = throw EnterpriseProtectionRefused(error)

    private fun retained(source: Source) {
        val request = source.request
        if (request.binding.origin != origin || request.target.partition.origin != origin ||
            fence.epoch != request.binding.epoch || fence.backgroundEpoch != source.background || sessions.load() != source.session ||
            source.session.accountId != request.binding.accountId || source.session.familyId != request.binding.familyId
        ) refused(EnterpriseAccessError.STALE)
    }

    override suspend fun confirm(request: EnterpriseContactRequest): EnterpriseContactReply = withContext(Dispatchers.IO) {
        try {
            if (request.binding.origin != origin) refused(EnterpriseAccessError.ACCESS_DENIED)
            val background = fence.backgroundEpoch
            val old = sessions.load() ?: refused(EnterpriseAccessError.SIGN_IN_REQUIRED)
            retained(Source(request, old, background))
            val refreshed = auth.refreshSession()
            val session = when (refreshed) {
                is AuthApiResult.Ok -> refreshed.value ?: refused(EnterpriseAccessError.SIGN_IN_REQUIRED)
                is AuthApiResult.Err -> refused(if (refreshed.verdict == "unavailable" || refreshed.verdict == AuthFlow.SECURE_STORAGE_UNAVAILABLE)
                    EnterpriseAccessError.UNAVAILABLE else EnterpriseAccessError.SIGN_IN_REQUIRED)
            }
            val source = Source(request, session, background)
            retained(source)
            require(session.accessToken.length in 1..512 && session.accessToken.all { it.code in 33..126 })
            val enrollment = request.existingEnrollment ?: discover(source)
            retained(source)
            val correlation = newId().also { EnterprisePartition.requireId(it) }
            val reply = exchange(source, "/v1/orgs/${request.target.partition.organizationId}/devices/contact", mapOf(
                "schema_version" to SyncJson.Integer("1"), "request_id" to SyncJson.Text(correlation),
                "family_id" to SyncJson.Text(session.familyId), "access_token" to SyncJson.Text(session.accessToken),
                "membership_id" to SyncJson.Text(request.target.membershipId), "enrollment_id" to SyncJson.Text(enrollment.enrollmentId),
            ))
            if (reply.status != 200) refusal(reply)
            retained(source)
            contact(reply.fields, source, enrollment, correlation)
        } catch (cancelled: CancellationException) { throw cancelled }
        catch (failure: EnterpriseProtectionRefused) { throw failure }
        catch (_: Exception) { refused() }
    }

    private suspend fun discover(source: Source): EnterpriseEnrollmentBinding {
        val prefix = "/v1/orgs/${source.request.target.partition.organizationId}/devices/"
        val proof = mapOf("family_id" to SyncJson.Text(source.session.familyId), "access_token" to SyncJson.Text(source.session.accessToken))
        val checked = exchange(source, prefix + "check", proof)
        if (checked.status == 200) return device(checked.fields, source.request.target)
        // Absence authorizes only an attempt at the existing fresh source-owned
        // bootstrap. It never relabels existing encrypted work or a contact denial.
        if (checked.status != 404) refusal(checked)
        val operation = newId().also { EnterprisePartition.requireId(it) }
        val enrolled = exchange(source, prefix + "enroll", proof + mapOf("schema_version" to SyncJson.Integer("1"),
            "operation_id" to SyncJson.Text(operation), "label" to SyncJson.Text("Android")))
        if (enrolled.status != 200) refusal(enrolled)
        val fields = enrolled.fields
        require(fields.keys == setOf("device", "operation_id", "reason", "replayed", "data_mode"))
        require(text(fields, "operation_id") == operation && text(fields, "reason").isEmpty() && text(fields, "data_mode") == "SYNTHETIC")
        require(fields["replayed"] is SyncJson.Bool)
        return device((fields["device"] as? SyncJson.Object)?.fields ?: throw IllegalArgumentException(), source.request.target)
    }

    private fun device(fields: Map<String, SyncJson>, target: EnterpriseDeviceTarget): EnterpriseEnrollmentBinding {
        require(fields.keys == setOf("id", "membership_id", "label", "state", "revision", "revocation_class", "enrolled_at", "last_sync_at", "revoked_at", "data_mode"))
        require(text(fields, "membership_id") == target.membershipId && text(fields, "state") == "ENROLLED" &&
            text(fields, "revocation_class").isEmpty() && fields["revoked_at"] === SyncJson.Null && text(fields, "data_mode") == "SYNTHETIC")
        return EnterpriseEnrollmentBinding(target, text(fields, "id"), integer(fields, "revision").toLong().also { require(it > 0) })
    }

    private fun contact(fields: Map<String, SyncJson>, source: Source, enrollment: EnterpriseEnrollmentBinding,
        correlation: String): EnterpriseContactReply {
        val common = setOf("schema_version", "request_id", "account_id", "api_origin", "org_id", "membership_id", "enrollment_id",
            "family_id", "enrollment_revision", "server_time", "verdict")
        require(integer(fields, "schema_version") == "1" && text(fields, "request_id") == correlation &&
            text(fields, "account_id") == source.request.binding.accountId && text(fields, "api_origin") == origin &&
            text(fields, "org_id") == source.request.target.partition.organizationId &&
            text(fields, "membership_id") == source.request.target.membershipId && text(fields, "enrollment_id") == enrollment.enrollmentId &&
            text(fields, "family_id") == source.session.familyId)
        val rawRevision = text(fields, "enrollment_revision")
        require(Regex("[1-9][0-9]{0,18}").matches(rawRevision))
        val revision = rawRevision.toLong().also { require(it >= enrollment.revision) }
        val bound = enrollment.copy(revision = revision)
        val rawTime = text(fields, "server_time")
        val time = EnterpriseTime.parseInstant(rawTime)
        require(EnterpriseTime.formatInstant(time) == rawTime)
        val millis = time.toEpochMilli().also { require(it >= 0) }
        return when (text(fields, "verdict")) {
            "ACTIVE" -> {
                require(fields.keys == common + setOf("access_model", "offline_validity_seconds", "managed_unlock_seconds"))
                require(text(fields, "access_model") == "EXISTING_ACCOUNT" && integer(fields, "offline_validity_seconds") == "2592000" && fields["managed_unlock_seconds"] === SyncJson.Null)
                EnterpriseContactReply.Active(com.anpfuel.application.enterprise.EnterpriseContactGrant(bound, source.session.familyId, millis))
            }
            "ERASE" -> {
                require(fields.keys == common + "revocation_class" && text(fields, "revocation_class") == "LOST_OR_COMPROMISED")
                EnterpriseContactReply.Erase(bound, source.session.familyId, millis)
            }
            "LOCK" -> {
                require(fields.keys == common + "revocation_class" && text(fields, "revocation_class") == "ACCESS_ENDED")
                EnterpriseContactReply.Lock(bound, source.session.familyId, millis)
            }
            else -> throw IllegalArgumentException()
        }
    }

    private suspend fun exchange(source: Source, path: String, fields: Map<String, SyncJson>): Reply {
        retained(source)
        val raw = CanonicalJson.encode(SyncJson.Object(fields), 4)
        val headers = signer.headers(source.request.binding, path, raw.copyOf()).toMap()
        require(headers.keys == setOf("Signature", "Signature-Created", "Signature-Expires", "Signature-Keyid", "Signature-Nonce"))
        require(headers.values.all { it.length in 1..2048 && it.all { character -> character.code in 33..126 } })
        retained(source)
        val request = Request.Builder().url(origin + path).header("Origin", origin)
            .header("Accept", "application/json").header("Cache-Control", "no-store")
            .post(raw.toRequestBody(JSON)).apply { headers.forEach { (key, value) -> header(key, value) } }.build()
        awaitCompanyResponse(http.newCall(request)).use { response ->
            retained(source)
            require(response.headers.values("Set-Cookie").isEmpty() && response.headers.values("Cache-Control").singleOrNull()?.equals("no-store", true) == true)
            val body = response.body ?: throw IllegalArgumentException()
            require(response.headers.values("Content-Type").size == 1 && body.contentType()?.let { it.type == "application" && it.subtype == "json" } == true)
            val input = body.source()
            require(!input.request(65537))
            val parsed = CanonicalJson.parse(input.readByteArray(), 65536, 8) as? SyncJson.Object ?: throw IllegalArgumentException()
            retained(source)
            return Reply(response.code, parsed.fields)
        }
    }

    private fun refusal(reply: Reply): Nothing {
        val code = ((reply.fields["error"] as? SyncJson.Object)?.fields?.get("code") as? SyncJson.Text)?.value
        refused(when {
            reply.status == 401 && code == "FRESH_AUTH_REQUIRED" -> EnterpriseAccessError.FRESH_AUTH_REQUIRED
            reply.status == 401 -> EnterpriseAccessError.SIGN_IN_REQUIRED
            reply.status == 403 && code == "MANAGED_ACCOUNT_RESTRICTED" -> EnterpriseAccessError.MANAGED_RESTRICTED
            reply.status == 403 || reply.status == 404 -> EnterpriseAccessError.ACCESS_DENIED
            reply.status == 409 -> EnterpriseAccessError.CONFLICT
            reply.status == 429 -> EnterpriseAccessError.RATE_LIMITED
            else -> EnterpriseAccessError.UNAVAILABLE
        })
    }

    private fun text(fields: Map<String, SyncJson>, key: String): String = (fields[key] as? SyncJson.Text)?.value ?: throw IllegalArgumentException()
    private fun integer(fields: Map<String, SyncJson>, key: String): String = (fields[key] as? SyncJson.Integer)?.lexeme ?: throw IllegalArgumentException()
    override fun toString(): String = "EnterpriseDeviceContactClient(redacted)"
    companion object { private val JSON = "application/json".toMediaType() }
}
