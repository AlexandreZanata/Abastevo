package com.anpfuel.data.remote

import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.repository.ContributionRateLimited
import com.anpfuel.domain.repository.TrainingConsentException
import com.anpfuel.domain.repository.TrainingConsentGateway
import com.anpfuel.domain.repository.TrainingConsentReceipt
import com.anpfuel.domain.repository.TrainingConsentRejectKind
import com.anpfuel.domain.repository.TrainingConsentStatus
import kotlinx.coroutines.CancellationException
import org.json.JSONObject

/** One shared signed attempt: stable decision/body, fresh nonce, exact owner/origin. */
class TrainingConsentHttpClient(private val proof: PhotoProofTransport) : TrainingConsentGateway {
    override suspend fun grantConsent(scope: ContributionScope, termsVersion: String, termsDigest: String, revision: Long, commandId: String): TrainingConsentReceipt {
        require(termsVersion.isNotBlank()) { "terms_version is blank" }
        require(termsDigest.isNotBlank() && termsVersion.length <= 128 && termsDigest.length <= 128)
        require(revision > 0)
        return call(scope) {
            receipt(proof.post("/v1/privacy/training-consent/grant", commandId,
                JSONObject().put("terms_version", termsVersion).put("terms_digest", termsDigest).put("revision", revision),
                scope.ownerScope), "GRANTED", revision)
        }
    }

    override suspend fun revokeConsent(scope: ContributionScope, revision: Long, commandId: String): TrainingConsentReceipt {
        require(revision > 0)
        return call(scope) {
            receipt(proof.post("/v1/privacy/training-consent/revoke", commandId,
                JSONObject().put("revision", revision), scope.ownerScope), "REVOKED", revision)
        }
    }

    override suspend fun consentStatus(scope: ContributionScope): TrainingConsentStatus = call(scope) {
        val body = proof.get("/v1/privacy/training-consent", scope.ownerScope)
        val granted = body.get("granted") as? Boolean ?: invalid()
        val state = string(body,"state")
        val revision = if (body.has("revision")) number(body, "revision") else 0
        val version = string(body,"terms_version")
        val digest = string(body,"terms_digest")
        if (revision < 0 || (state != null && state !in setOf("GRANTED", "REVOKED")) ||
            (granted && (state != "GRANTED" || revision < 1 || version == null || digest == null))) invalid()
        TrainingConsentStatus(granted, state, version, digest, revision, boolean(body,"needs_regrant"))
    }

    private fun receipt(body: JSONObject, expected: String, revision: Long): TrainingConsentReceipt {
        if (body.getString("state") != expected || number(body, "revision") != revision) invalid()
        return TrainingConsentReceipt(expected, revision, body.optString("terms_version", ""),
            body.get("replayed") as? Boolean ?: invalid(), boolean(body,"delete_owned_training_assets"))
    }

    private fun string(body:JSONObject,name:String):String? {
        if(!body.has(name)) return null
        val value=body.get(name) as? String ?: invalid()
        if(value.length>128) invalid()
        return value.takeIf {it.isNotBlank()}
    }

    private fun boolean(body:JSONObject,name:String):Boolean =
        if(body.has(name)) body.get(name) as? Boolean ?: invalid() else false

    private fun number(body: JSONObject, name: String): Long {
        val value = body.get(name)
        if (value !is Int && value !is Long) invalid()
        return (value as Number).toLong()
    }

    private suspend fun <T> call(scope: ContributionScope, run: suspend () -> T): T {
        if (scope.origin != proof.origin || !scope.ownerScope.startsWith("account:")) {
            throw TrainingConsentException(TrainingConsentRejectKind.AUTH_REQUIRED, "privacy owner unavailable")
        }
        return try {
            run()
        } catch (error: CancellationException) {
            throw error
        } catch (error: TrainingConsentException) {
            throw error
        } catch (error: ContributionRateLimited) {
            throw TrainingConsentException(TrainingConsentRejectKind.TRANSPORT, "privacy request limited", error.retryAfterSeconds)
        } catch (error: PhotoOperationRefused) {
            val kind = when {
                error.status == 401 || error.status == 403 -> TrainingConsentRejectKind.AUTH_REQUIRED
                error.code == "privacy.terms-outdated" -> TrainingConsentRejectKind.TERMS_OUTDATED
                error.code == "privacy.stale-revision" -> TrainingConsentRejectKind.STALE_REVISION
                error.status == 409 -> TrainingConsentRejectKind.CONFLICT
                error.status == 400 -> TrainingConsentRejectKind.INVALID
                else -> TrainingConsentRejectKind.TRANSPORT
            }
            throw TrainingConsentException(kind, "privacy request refused",error.retryAfterSeconds)
        } catch (_: Exception) {
            invalid()
        }
    }

    private fun invalid(): Nothing = throw TrainingConsentException(TrainingConsentRejectKind.TRANSPORT, "invalid privacy response")
}
