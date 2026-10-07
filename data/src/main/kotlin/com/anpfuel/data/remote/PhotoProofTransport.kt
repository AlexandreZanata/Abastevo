package com.anpfuel.data.remote

import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.local.auth.AndroidAnonymousDeviceKeys
import com.anpfuel.data.local.auth.AnonymousProofCrypto
import com.anpfuel.domain.portable.PortableAnonymousProof
import java.io.IOException
import java.security.MessageDigest
import java.util.Base64
import java.util.concurrent.TimeUnit
import javax.inject.Inject
import javax.inject.Named
import javax.inject.Singleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/** Signed photo operations; fresh server nonce, exact body digest and no mutation retries/redirects. */
@Singleton
class PhotoProofTransport internal constructor(
    private val keys: AndroidAnonymousDeviceKeys,
    private val identity: AnonymousProofHttpClient,
    private val auth: AuthFlow,
    val origin: String,
    httpClient: okhttp3.OkHttpClient,
) {
    @Inject constructor(
        keys: AndroidAnonymousDeviceKeys,
        identity: AnonymousProofHttpClient,
        auth: AuthFlow,
        @Named("apiOrigin") environment: ApiEnvironment,
    ): this(keys, identity, auth, environment.origin, OkHttpClientFactory.create(maxRetries=0))

    private val client=httpClient.newBuilder()
        .followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false).callTimeout(20,TimeUnit.SECONDS).build()
    private var registeredFingerprint: String?=null

    fun ownerScope(): String? = keys.fingerprint()?.let { fingerprint ->
        val account=auth.currentSession()?.accountId
        (account?.let { "account:$it:" } ?: "anonymous:")+fingerprint
    }

    suspend fun identityScope(): String = withContext(Dispatchers.IO) {
        synchronized(this@PhotoProofTransport) {
            ensureRegistered()
            ownerScope() ?: throw IOException("photo.identity-unavailable")
        }
    }

    suspend fun localScope(): String = withContext(Dispatchers.IO) {
        if (!keys.ensureKey()) throw IOException("photo.identity-unavailable")
        ownerScope() ?: throw IOException("photo.identity-unavailable")
    }

    suspend fun get(path: String, expectedScope: String? = null): JSONObject = withContext(Dispatchers.IO) {
        require(path.startsWith("/v1/") && !path.contains('?') && !path.contains('#'))
        synchronized(this@PhotoProofTransport) {
            if (expectedScope != null && ownerScope() != expectedScope) throw IOException("photo.identity-changed")
            ensureRegistered()
            val scope = ownerScope() ?: throw IOException("photo.identity-unavailable")
            val fingerprint = keys.fingerprint() ?: throw IOException("photo.identity-unavailable")
            val challenge = identity.requestChallenge(fingerprint, "SIGN")
            val now = System.currentTimeMillis() / 1000
            val lines = PortableAnonymousProof.buildBaseLines("GET", authority(), path, "", null, null, now, now + 120, fingerprint, challenge.nonce)
            val request = Request.Builder().url(origin.trimEnd('/') + path).get()
                .header("Accept", "application/json").header("Cache-Control", "no-store")
                .header("Signature", sign(lines)).header("Signature-Created", now.toString())
                .header("Signature-Expires", (now + 120).toString()).header("Signature-Keyid", fingerprint)
                .header("Signature-Nonce", challenge.nonce).build()
            if (ownerScope() != scope) throw IOException("photo.identity-changed")
            val result = execute(request)
            if (ownerScope() != scope) throw IOException("photo.identity-changed")
            result
        }
    }

    suspend fun post(path: String, key: String, doc: JSONObject, expectedScope: String? = null): JSONObject = withContext(Dispatchers.IO) {
        require(path.startsWith("/v1/") && !path.contains('?') && key.length in 1..128)
        synchronized(this@PhotoProofTransport) {
            if (expectedScope != null && ownerScope() != expectedScope) throw IOException("photo.identity-changed")
            ensureRegistered()
            val scope=ownerScope() ?: throw IOException("photo.identity-unavailable")
            val fingerprint=keys.fingerprint() ?: throw IOException("photo.identity-unavailable")
            val challenge=identity.requestChallenge(fingerprint,"SIGN")
            val body=doc.toString().toByteArray(Charsets.UTF_8)
            val now=System.currentTimeMillis()/1000
            val digest="\"sha-512=:${Base64.getEncoder().encodeToString(MessageDigest.getInstance("SHA-512").digest(body))}:\""
            val lines=PortableAnonymousProof.buildBaseLines("POST",authority(),path,"","application/json",digest,now,now+120,fingerprint,challenge.nonce)
            val signature=sign(lines)
            val request=Request.Builder().url(origin.trimEnd('/')+path)
                .post(body.toRequestBody(JSON)).header("Accept","application/json")
                .header("Cache-Control","no-store").header("Idempotency-Key",key)
                .header("Signature",signature).header("Signature-Created",now.toString())
                .header("Signature-Expires",(now+120).toString()).header("Signature-Keyid",fingerprint)
                .header("Signature-Nonce",challenge.nonce).build()
            if (ownerScope()!=scope) throw IOException("photo.identity-changed")
            val result=execute(request)
            if(ownerScope()!=scope) throw IOException("photo.identity-changed")
            result
        }
    }

    private fun ensureRegistered() {
        if(!keys.ensureKey()) throw IOException("photo.identity-unavailable")
        val fingerprint=keys.fingerprint() ?: throw IOException("photo.identity-unavailable")
        if(registeredFingerprint==fingerprint) return
        val jwk=keys.publicJwk() ?: throw IOException("photo.identity-unavailable")
        val challenge=identity.requestChallenge(fingerprint,"REGISTER")
        val now=System.currentTimeMillis()/1000
        val lines=PortableAnonymousProof.buildBaseLines("POST",authority(),"/v1/contributors","",null,null,now,now+120,fingerprint,challenge.nonce)
        val result=JSONObject(identity.register(jwk.first,jwk.second,challenge.challengeId,AnonymousProofHttpClient.Proof(lines,sign(lines))))
        if(result.optString("contributor_id").isBlank() || result.optString("key_id").isBlank()) throw IOException("photo.identity-unavailable")
        registeredFingerprint=fingerprint
    }

    private fun sign(lines: List<String>): String {
        val digest=MessageDigest.getInstance("SHA-512").digest(PortableAnonymousProof.buildBase(lines).toByteArray(Charsets.UTF_8))
        val signature=keys.signDigest(digest) ?: throw IOException("photo.signing-unavailable")
        return AnonymousProofCrypto.b64urlEncode(signature)
    }
    private fun authority(): String = java.net.URI(origin).rawAuthority ?: throw IOException("photo.origin-unavailable")
    private fun execute(request: Request): JSONObject {
        client.newCall(request).execute().use { response ->
            if(!response.isSuccessful) throw PhotoOperationRefused(response.code)
            val source=response.body?.source() ?: throw IOException("photo.empty-response")
            if(source.request(65537)) throw IOException("photo.response-size")
            return try { JSONObject(source.readUtf8()) } catch (_: Exception) { throw IOException("photo.invalid-response") }
        }
    }
    companion object { private val JSON="application/json".toMediaType() }
}

class PhotoOperationRefused(val status: Int): IOException("photo.operation-refused:$status")
