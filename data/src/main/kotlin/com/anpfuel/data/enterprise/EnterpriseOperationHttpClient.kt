package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseAccessBinding
import com.anpfuel.application.enterprise.EnterpriseEnrollmentBinding
import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.queue.EnterpriseOperationDelivery
import com.anpfuel.application.enterprise.queue.EnterpriseOperationTransport
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueDeliveryFailure
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBatch
import com.anpfuel.domain.enterprise.sync.SyncBatchResponse
import com.anpfuel.domain.enterprise.sync.SyncDeliveryBudget
import com.anpfuel.domain.enterprise.sync.SyncLimits
import com.anpfuel.domain.portable.PortableAuth
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import java.util.UUID
import java.util.WeakHashMap
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

/** Actual current sessions are captured independently of the UI selection owner. */
class EnterpriseOperationSessionSource(
    origin: String,
    private val sessions: AuthSessionStore,
    private val auth: AuthFlow,
    private val fence: EnterpriseSessionFence,
    private val enrollment: suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> EnterpriseEnrollmentBinding,
    private val newId: () -> String = { UUID.randomUUID().toString() },
) : EnterpriseQueueSource {
    internal val apiOrigin = EnterprisePartition.canonicalOrigin(origin).also { require(it == origin) }

    private class Captured(val session: PortableAuth.Session, val epoch: Long, val background: Long) {
        override fun toString(): String = "EnterpriseOperationCapturedSession(redacted)"
    }

    // Only leases and attempts issued here can recover a delivery proof. Both
    // maps are accessed under the shared fence; they are never durable state.
    private val accounts = WeakHashMap<EnterpriseQueueAccountLease, Captured>()
    private val attempts = WeakHashMap<EnterpriseQueueAttempt, Captured>()

    private suspend fun <T> guarded(work: suspend () -> T): T = withContext(Dispatchers.IO) {
        try { work() }
        catch (cancelled: CancellationException) { throw cancelled }
        catch (denied: EnterpriseLocalDenied) { throw denied }
        catch (failure: EnterpriseQueueDeliveryFailure) { throw failure }
        catch (_: Exception) { throw EnterpriseQueueDeliveryFailure() }
    }

    private fun observed(): Captured = Captured(sessions.load() ?: throw EnterpriseLocalDenied(), fence.epoch, fence.backgroundEpoch)
    private fun retained(value: Captured): Boolean = fence.epoch == value.epoch && fence.backgroundEpoch == value.background &&
        sessions.load() == value.session

    private fun capture(value: Captured): EnterpriseQueueAccountLease {
        if (!retained(value)) throw EnterpriseLocalDenied()
        val identity = EnterpriseLocalIdentity(value.session.accountId, apiOrigin, value.session.familyId)
        val lease = EnterpriseQueueAccountLease.capture(identity, value.epoch, value.background, newId(),
            proofBudget = SyncDeliveryBudget.fromProof(value.session.familyId, value.session.accessToken),
            retained = { retained(value) })
        accounts[lease] = value
        return lease
    }

    private fun checked(account: EnterpriseQueueAccountLease): Captured {
        account.assertCurrent()
        val value = accounts[account] ?: throw EnterpriseLocalDenied()
        if (!retained(value) || account.identity.account != value.session.accountId || account.identity.origin != apiOrigin ||
            account.identity.family != value.session.familyId || account.epoch != value.epoch || account.background != value.background)
            throw EnterpriseLocalDenied()
        return value
    }

    /** An expired personal access token does not require networking for an offline local save. */
    override suspend fun account(): EnterpriseQueueAccountLease = guarded { fence.access { capture(observed()) } }

    /** Refresh can replace credentials only before this new exact attempt is captured. */
    override suspend fun deliveryAccount(): EnterpriseQueueAccountLease = guarded {
        val before = fence.access { observed() }
        val refreshed = when (val response = auth.refreshSession()) {
            is AuthApiResult.Ok -> response.value ?: throw EnterpriseLocalDenied()
            is AuthApiResult.Err -> throw EnterpriseQueueDeliveryFailure()
        }
        fence.access {
            val current = observed()
            if (current.epoch != before.epoch || current.background != before.background || current.session != refreshed ||
                current.session.accountId != before.session.accountId || current.session.familyId != before.session.familyId)
                throw EnterpriseLocalDenied()
            capture(current)
        }
    }

    override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt = guarded {
        val captured = fence.access { checked(account) }
        if (!account.identity.owns(partition)) throw EnterpriseLocalDenied()
        val actual = enrollment(account, partition)
        account.assertCurrent()
        if (actual.target.partition != partition) throw EnterpriseLocalDenied()
        fence.access {
            if (checked(account) !== captured) throw EnterpriseLocalDenied()
            account.forEnrollment(actual).also { attempts[it] = captured }
        }
    }

    /** Recheck actual custody metadata as well as current credentials, without retaining an I/O lock. */
    internal suspend fun proof(attempt: EnterpriseQueueAttempt): PortableAuth.Session = guarded {
        val captured = fence.access {
            attempt.assertCurrent()
            val value = attempts[attempt] ?: throw EnterpriseLocalDenied()
            if (checked(attempt.account) !== value) throw EnterpriseLocalDenied()
            value
        }
        if (enrollment(attempt.account, attempt.partition) != attempt.enrollment) throw EnterpriseLocalDenied()
        fence.access {
            attempt.assertCurrent()
            if (attempts[attempt] !== captured || checked(attempt.account) !== captured) throw EnterpriseLocalDenied()
            captured.session
        }
    }

    override fun toString(): String = "EnterpriseOperationSessionSource(redacted)"
}

/** The signed delivery plane never uses company cookies or changes custody on a generic refusal. */
class EnterpriseOperationHttpClient(
    http: OkHttpClient,
    private val source: EnterpriseOperationSessionSource,
    private val signer: EnterpriseDeviceSigner,
    private val limits: SyncLimits = SyncLimits.DEFAULT,
) : EnterpriseOperationTransport {
    private val http = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).cookieJar(CookieJar.NO_COOKIES).cache(null)
        .authenticator(Authenticator.NONE).proxyAuthenticator(Authenticator.NONE)
        .callTimeout(20, TimeUnit.SECONDS).build()

    override suspend fun deliver(attempt: EnterpriseQueueAttempt, operations: List<EnterpriseOperation>): EnterpriseOperationDelivery = withContext(Dispatchers.IO) {
        try {
            attempt.assertCurrent()
            val expected = operations.toList()
            val partition = attempt.partition
            if (partition.origin != source.apiOrigin || expected.any {
                    it.binding.ownerAccountId != partition.ownerAccount || it.binding.apiOrigin != partition.origin ||
                        it.binding.organizationId != partition.organizationId || it.membershipId != attempt.enrollment.target.membershipId ||
                        it.enrollmentId != attempt.enrollment.enrollmentId
                }) throw EnterpriseLocalDenied()
            val session = source.proof(attempt)
            val raw = SyncBatch.encode(session.familyId, session.accessToken, expected, limits)
            val path = "/v1/orgs/${partition.organizationId}/sync/operations"
            val binding = EnterpriseAccessBinding(session.accountId, source.apiOrigin, session.familyId, attempt.account.epoch)
            if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
            val headers = signer.headers(binding, path, raw.copyOf()).toMap()
            require(headers.keys == PROOF_HEADERS && headers.values.all { value ->
                value.length in 1..2048 && value.all { it.code in 33..126 }
            })
            if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
            val request = Request.Builder().url(source.apiOrigin + path).header("Origin", source.apiOrigin)
                .header("Accept", "application/json").header("Cache-Control", "no-store")
                .post(raw.toRequestBody(JSON)).apply { headers.forEach { (name, value) -> header(name, value) } }.build()
            attempt.assertCurrent()
            awaitCompanyResponse(http.newCall(request)).use { response ->
                if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
                val retryAfter = retryAfter(response.headers.values("Retry-After").singleOrNull(),
                    response.headers.values("Date").singleOrNull())
                require(response.headers.values("Set-Cookie").isEmpty() && response.headers.values("ETag").isEmpty() &&
                    response.headers.values("Cache-Control").singleOrNull()?.equals("no-store", true) == true)
                val body = response.body ?: throw EnterpriseQueueDeliveryFailure(retryAfter)
                require(response.headers.values("Content-Type").size == 1 && body.contentType()?.let {
                    it.type == "application" && it.subtype == "json"
                } == true)
                if (response.code != 200) throw EnterpriseQueueDeliveryFailure(retryAfter)
                val input = body.source()
                require(!input.request(limits.maxBatchBytes.toLong() + 1))
                val decoded = SyncBatchResponse.parse(input.readByteArray(), expected, limits)
                if (source.proof(attempt) != session) throw EnterpriseLocalDenied()
                EnterpriseOperationDelivery(decoded.acknowledgements, retryAfter)
            }
        } catch (cancelled: CancellationException) { throw cancelled }
        catch (denied: EnterpriseLocalDenied) { throw denied }
        catch (failure: EnterpriseQueueDeliveryFailure) { throw failure }
        catch (_: Exception) { throw EnterpriseQueueDeliveryFailure() }
    }

    private fun retryAfter(header: String?, responseDate: String?): Long? {
        val raw = header?.trim()?.takeIf { it.isNotEmpty() } ?: return null
        if (raw.all { it in '0'..'9' }) {
            val seconds = raw.toLongOrNull() ?: return Long.MAX_VALUE
            return if (seconds > Long.MAX_VALUE / 1000) Long.MAX_VALUE else seconds * 1000
        }
        val target = httpDate(raw) ?: return null
        val received = httpDate(responseDate?.trim() ?: return null) ?: return null
        // Both instants come from the same authenticated response. Ignoring
        // transit is conservative; a fast device clock never shortens this wait.
        if (target <= received) return 0
        return if (received < 0 && target > Long.MAX_VALUE + received) Long.MAX_VALUE else target - received
    }

    private fun httpDate(raw: String): Long? = try {
        ZonedDateTime.parse(raw, DateTimeFormatter.RFC_1123_DATE_TIME).toInstant().toEpochMilli()
    } catch (_: Exception) { null }

    override fun toString(): String = "EnterpriseOperationHttpClient(redacted)"
    companion object {
        private val JSON = "application/json".toMediaType()
        private val PROOF_HEADERS = setOf("Signature", "Signature-Created", "Signature-Expires", "Signature-Keyid", "Signature-Nonce")
    }
}
