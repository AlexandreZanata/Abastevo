package com.anpfuel.data.remote

import com.anpfuel.domain.portable.PortableAnonymousProof
import java.io.IOException
import java.util.concurrent.TimeUnit
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/**
 * P10-T03 bounded anonymous-proof HTTP client.
 *
 * Network-only: transport failures and non-2xx responses throw
 * [IOException]; empty or oversized (>64 KiB) bodies throw as well. Proof
 * lines and signatures are sent verbatim; private key material never
 * crosses this client (only public JWK coordinates). The preview base URL
 * never resolves until deployment configuration lands.
 */
class AnonymousProofHttpClient(
    private val client: OkHttpClient,
    private val baseUrl: String,
    private val nowMillis: () -> Long = { System.currentTimeMillis() },
) {
    data class ChallengeResponse(
        val challengeId: String,
        val nonce: String,
        val fingerprint: String,
        val purpose: String,
        val rawBody: String,
    )

    data class Proof(val lines: List<String>, val signatureB64Url: String)

    fun requestChallenge(fingerprint: String, purpose: String): ChallengeResponse {
        require(PortableAnonymousProof.isFingerprintShape(fingerprint)) { "fingerprint malformed" }
        require(PortableAnonymousProof.parsePurpose(purpose) != null) { "purpose unknown" }
        val payload = JSONObject()
            .put("fingerprint", fingerprint)
            .put("purpose", purpose)
            .toString()
        val raw = post("/v1/identity/challenges", payload)
        val doc = try {
            JSONObject(raw)
        } catch (error: Exception) {
            throw IOException("anonymous challenge: unparsable reply", error)
        }
        val challengeId = doc.optString("challenge_id", "")
        val nonce = doc.optString("nonce", "")
        val echoFp = doc.optString("fingerprint", "")
        val echoPurpose = doc.optString("purpose", "")
        if (challengeId.isBlank() || nonce.isBlank() || echoFp != fingerprint) {
            throw IOException("anonymous challenge: invalid reply")
        }
        if (PortableAnonymousProof.splitNonce(nonce) == null) {
            throw IOException("anonymous challenge: malformed nonce")
        }
        return ChallengeResponse(challengeId, nonce, echoFp, echoPurpose, raw)
    }

    fun register(
        jwkX: String,
        jwkY: String,
        challengeId: String,
        proof: Proof,
    ): String {
        require(challengeId.isNotBlank()) { "challenge_id is blank" }
        require(proof.lines.size == 8 || proof.lines.size == 10) { "proof lines malformed" }
        require(proof.signatureB64Url.isNotBlank()) { "signature is blank" }
        val payload = JSONObject()
            .put(
                "public_jwk",
                JSONObject()
                    .put("kty", "EC")
                    .put("crv", "P-256")
                    .put("x", jwkX)
                    .put("y", jwkY),
            )
            .put("challenge_id", challengeId)
            .put(
                "proof",
                JSONObject()
                    .put("lines", org.json.JSONArray(proof.lines))
                    .put("signature", proof.signatureB64Url),
            )
            .toString()
        return post("/v1/contributors", payload)
    }

    fun rotate(
        oldChallengeId: String,
        oldProof: Proof,
        newJwkX: String,
        newJwkY: String,
        newChallengeId: String,
        newProof: Proof,
    ): String {
        require(oldChallengeId.isNotBlank() && newChallengeId.isNotBlank()) { "challenge_id is blank" }
        val payload = JSONObject()
            .put(
                "old",
                JSONObject()
                    .put("challenge_id", oldChallengeId)
                    .put("lines", org.json.JSONArray(oldProof.lines))
                    .put("signature", oldProof.signatureB64Url),
            )
            .put(
                "new_jwk",
                JSONObject()
                    .put("kty", "EC")
                    .put("crv", "P-256")
                    .put("x", newJwkX)
                    .put("y", newJwkY),
            )
            .put(
                "new",
                JSONObject()
                    .put("challenge_id", newChallengeId)
                    .put("lines", org.json.JSONArray(newProof.lines))
                    .put("signature", newProof.signatureB64Url),
            )
            .toString()
        return post("/v1/contributors/me/keys/rotate", payload)
    }

    private fun post(path: String, payload: String): String {
        val bytes = payload.toByteArray(Charsets.UTF_8)
        require(bytes.size <= PortableAnonymousProof.MAX_BODY_BYTES) { "body exceeds 64 KiB" }
        val url = baseUrl.trimEnd('/') + path
        val request = Request.Builder()
            .url(url)
            .post(bytes.toRequestBody("application/json".toMediaType()))
            .header("Accept", "application/json")
            .header("Content-Type", "application/json")
            .build()
        try {
            client.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    throw IOException("anonymous proof call failed: HTTP ${response.code}")
                }
                val body = response.body?.string()
                if (body.isNullOrBlank()) {
                    throw IOException("anonymous proof call failed: empty body")
                }
                if (body.toByteArray(Charsets.UTF_8).size > PortableAnonymousProof.MAX_BODY_BYTES) {
                    throw IOException("anonymous proof call failed: reply exceeds 64 KiB")
                }
                return body
            }
        } catch (error: IOException) {
            throw error
        } catch (error: Exception) {
            throw IOException("anonymous proof call failed", error)
        }
    }

    companion object {
        fun defaultClient(): OkHttpClient = OkHttpClient.Builder()
            .connectTimeout(10L, TimeUnit.SECONDS)
            .readTimeout(10L, TimeUnit.SECONDS)
            .build()
    }
}
