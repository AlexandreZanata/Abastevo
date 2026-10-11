package com.anpfuel.data.remote

import com.anpfuel.domain.repository.TrainingConsentException
import com.anpfuel.domain.repository.TrainingConsentGateway
import com.anpfuel.domain.repository.TrainingConsentReceipt
import com.anpfuel.domain.repository.TrainingConsentRejectKind
import com.anpfuel.domain.repository.TrainingConsentStatus
import java.io.IOException
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/**
 * CS11A bounded training-consent HTTP client (B-BR-CS06).
 *
 * Network-only: `POST /v1/privacy/training-consent/grant`,
 * `POST /v1/privacy/training-consent/revoke` and
 * `GET /v1/privacy/training-consent` with the per-device monotonic
 * revision the server orders. Receipts and the owner status parse
 * exactly; absent consent (`{"granted":false}`) parses with nulls
 * and revision zero. Transport failures, empty bodies and unmapped
 * statuses throw [TrainingConsentException] with
 * [TrainingConsentRejectKind.TRANSPORT] so the UI never claims
 * consent it does not hold. Identical retries converge server-side
 * (`replayed = true`); this client never retries by itself.
 * HTTPS only; the preview base URL never resolves until deployment
 * configuration lands.
 */
class TrainingConsentHttpClient(
    private val client: OkHttpClient,
    private val baseUrl: String,
) : TrainingConsentGateway {

    override suspend fun grantConsent(
        termsVersion: String,
        termsDigest: String,
        revision: Long,
    ): TrainingConsentReceipt {
        require(termsVersion.isNotBlank()) { "terms_version is blank" }
        require(termsDigest.isNotBlank()) { "terms_digest is blank" }
        require(revision >= 1) { "revision must be positive" }
        val payload = JSONObject()
            .put("terms_version", termsVersion)
            .put("terms_digest", termsDigest)
            .put("revision", revision)
            .toString()
        val raw = post("/v1/privacy/training-consent/grant", payload)
        val body = parseObject(raw)
        return TrainingConsentReceipt(
            state = body.getString("state"),
            revision = body.getLong("revision"),
            termsVersion = body.optString("terms_version", termsVersion),
            replayed = body.optBoolean("replayed", false),
        )
    }

    override suspend fun revokeConsent(
        revision: Long,
    ): TrainingConsentReceipt {
        require(revision >= 1) { "revision must be positive" }
        val payload = JSONObject()
            .put("revision", revision)
            .toString()
        val raw = post("/v1/privacy/training-consent/revoke", payload)
        val body = parseObject(raw)
        return TrainingConsentReceipt(
            state = body.getString("state"),
            revision = body.getLong("revision"),
            termsVersion = body.optString("terms_version", ""),
            replayed = body.optBoolean("replayed", false),
            deleteOwnedTrainingAssets = body.optBoolean("delete_owned_training_assets", false),
        )
    }

    override suspend fun consentStatus(): TrainingConsentStatus {
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + "/v1/privacy/training-consent")
            .get()
            .header("Accept", "application/json")
            .build()
        try {
            client.newCall(request).execute().use { response ->
                val body = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    throw mapConsentError(response.code, body)
                }
                if (body.isBlank()) {
                    throw TrainingConsentException(
                        TrainingConsentRejectKind.TRANSPORT,
                        "consent request failed: empty body",
                    )
                }
                return parseStatus(body)
            }
        } catch (consentError: TrainingConsentException) {
            throw consentError
        } catch (error: IOException) {
            throw TrainingConsentException(
                TrainingConsentRejectKind.TRANSPORT,
                "consent request failed: transport",
            )
        } catch (error: Exception) {
            throw TrainingConsentException(
                TrainingConsentRejectKind.TRANSPORT,
                "consent request failed: transport",
            )
        }
    }

    private fun parseStatus(raw: String): TrainingConsentStatus {
        val body = parseObject(raw)
        return TrainingConsentStatus(
            granted = body.optBoolean("granted", false),
            state = body.optString("state", "").takeIf { it.isNotBlank() },
            termsVersion = body.optString("terms_version", "").takeIf { it.isNotBlank() },
            termsDigest = body.optString("terms_digest", "").takeIf { it.isNotBlank() },
            revision = body.optLong("revision", 0),
            needsRegrant = body.optBoolean("needs_regrant", false),
        )
    }

    private fun parseObject(raw: String): JSONObject {
        return try {
            JSONObject(raw)
        } catch (_: Exception) {
            throw TrainingConsentException(
                TrainingConsentRejectKind.TRANSPORT,
                "consent request failed: malformed body",
            )
        }
    }

    private fun post(path: String, json: String): String {
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + path)
            .post(json.toRequestBody(JSON_MEDIA))
            .header("Accept", "application/json")
            .build()
        try {
            client.newCall(request).execute().use { response ->
                val body = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    throw mapConsentError(response.code, body)
                }
                if (body.isBlank()) {
                    throw TrainingConsentException(
                        TrainingConsentRejectKind.TRANSPORT,
                        "consent request failed: empty body",
                    )
                }
                return body
            }
        } catch (consentError: TrainingConsentException) {
            throw consentError
        } catch (error: IOException) {
            throw TrainingConsentException(
                TrainingConsentRejectKind.TRANSPORT,
                "consent request failed: transport",
            )
        } catch (error: Exception) {
            throw TrainingConsentException(
                TrainingConsentRejectKind.TRANSPORT,
                "consent request failed: transport",
            )
        }
    }

    private fun mapConsentError(code: Int, body: String): TrainingConsentException {
        // Fixed messages only: the raw body (which may echo metadata)
        // never enters error text.
        return when {
            code == 401 || body.contains(CODE_AUTH) ->
                TrainingConsentException(
                    TrainingConsentRejectKind.AUTH_REQUIRED,
                    "authentication required",
                )
            code == 400 && body.contains(CODE_TERMS) ->
                TrainingConsentException(
                    TrainingConsentRejectKind.TERMS_OUTDATED,
                    "terms superseded; review the current terms",
                )
            code == 409 && body.contains(CODE_STALE) ->
                TrainingConsentException(
                    TrainingConsentRejectKind.STALE_REVISION,
                    "decision superseded; reload the current state",
                )
            code == 409 ->
                TrainingConsentException(
                    TrainingConsentRejectKind.CONFLICT,
                    "same revision, different decision",
                )
            code == 400 ->
                TrainingConsentException(
                    TrainingConsentRejectKind.INVALID,
                    "invalid consent request",
                )
            else ->
                TrainingConsentException(
                    TrainingConsentRejectKind.TRANSPORT,
                    "consent request failed: HTTP $code",
                )
        }
    }

    companion object {
        private val JSON_MEDIA = "application/json; charset=utf-8".toMediaType()
        private const val CODE_AUTH = "privacy.auth-required"
        private const val CODE_TERMS = "privacy.terms-outdated"
        private const val CODE_STALE = "privacy.stale-revision"
    }
}
