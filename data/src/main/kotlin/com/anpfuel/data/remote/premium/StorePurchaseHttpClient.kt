package com.anpfuel.data.remote.premium

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.remote.PhotoOperationRefused
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.StoreOperationId
import com.anpfuel.domain.premium.StoreProvider
import com.anpfuel.domain.premium.StorePurchase
import com.anpfuel.domain.premium.StorePurchaseKind
import com.anpfuel.domain.premium.StoreVerification
import com.anpfuel.domain.repository.ContributionRateLimited
import com.anpfuel.domain.repository.StorePurchaseGateway
import java.io.IOException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject

/** The session or device binding no longer belongs to the expected account. */
internal class StoreOwnerChanged : IOException("store.owner-changed")

/** No live session for a signed store request. */
internal class StoreSignInRequired : IOException("store.sign-in-required")

/**
 * Signed store purchase verification/restore
 * (`POST /v1/premium/purchases/{verify,restore}`).
 *
 * Each attempt carries a fresh session proof and an exact-body signature by
 * the device key bound to the account ([PhotoProofTransport]); the operation
 * ID is also the idempotency key, so a retry of the same attempt replays.
 * The purchase token is sent only in the signed body and never logged. The
 * client never acknowledges the purchase (B-BR-STORE-08).
 */
class StorePurchaseHttpClient internal constructor(
    private val request: suspend (path: String, operationId: String, owner: String, body: JSONObject) -> JSONObject,
) : StorePurchaseGateway {

    override suspend fun submit(
        kind: StorePurchaseKind,
        owner: String,
        operationId: String,
        purchaseToken: String,
        provider: StoreProvider,
    ): StoreVerification {
        if (owner.isBlank() || !StoreOperationId.isValid(operationId) || !StorePurchase.isValidToken(purchaseToken)) {
            return StoreVerification.Invalid
        }
        val body = JSONObject()
            .put("operation_id", operationId)
            .put("purchase_token", purchaseToken)
            .put("provider", provider.wire)
        val root = try {
            request("/v1/premium/purchases/${kind.route}", operationId, owner, body)
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: StoreOwnerChanged) {
            return StoreVerification.OwnerChanged
        } catch (_: StoreSignInRequired) {
            return StoreVerification.SignInRequired
        } catch (limited: ContributionRateLimited) {
            return StoreVerification.RetryLater(limited.retryAfterSeconds)
        } catch (refused: PhotoOperationRefused) {
            return refusal(refused)
        } catch (failure: IOException) {
            return if (failure.message == IDENTITY_CHANGED) StoreVerification.OwnerChanged else StoreVerification.Unavailable
        } catch (_: Exception) {
            return StoreVerification.Unavailable
        }
        return receipt(root, operationId)
    }

    private fun refusal(refused: PhotoOperationRefused): StoreVerification = when (refused.status) {
        400, 404 -> StoreVerification.Invalid
        401 -> StoreVerification.SignInRequired
        403 -> StoreVerification.Denied
        409, 429 -> StoreVerification.RetryLater(refused.retryAfterSeconds)
        else -> StoreVerification.Unavailable
    }

    private fun receipt(root: JSONObject, operationId: String): StoreVerification = try {
        val echoed = root.opt("operation_id") as? String
        val replayed = root.opt("replayed") as? Boolean
        val status = root.opt("status") as? JSONObject
        if (echoed != operationId || replayed == null || status == null) {
            StoreVerification.Invalid
        } else {
            StoreVerification.Verified(PremiumStatusJson.decode(status), replayed)
        }
    } catch (_: Exception) {
        StoreVerification.Invalid
    }

    companion object {
        private const val IDENTITY_CHANGED = "photo.identity-changed"

        /**
         * Production client: refreshes the session for every attempt, refuses a
         * different account and signs with the device key bound to [owner].
         */
        fun signed(proof: PhotoProofTransport, auth: AuthFlow): StorePurchaseHttpClient =
            StorePurchaseHttpClient { path, operationId, owner, body ->
                val live = liveSession(auth, owner)
                val scope = proof.localScope()
                if (!scope.startsWith("account:$owner:")) throw StoreOwnerChanged()
                val doc = JSONObject(body.toString())
                    .put("family_id", live.familyId)
                    .put("access_token", live.accessToken)
                proof.post(path, operationId, doc, scope)
            }

        private suspend fun liveSession(auth: AuthFlow, owner: String): PortableAuth.Session =
            withContext(Dispatchers.IO) {
                val live = when (val result = auth.refreshSession()) {
                    is AuthApiResult.Err -> {
                        if (auth.rehydrate() == AuthFlow.AuthState.LoggedOut) throw StoreSignInRequired()
                        throw IOException("store.session-unavailable")
                    }
                    is AuthApiResult.Ok -> result.value ?: throw StoreSignInRequired()
                }
                if (!PortableAuth.isAccessLive(live, System.currentTimeMillis() / 1000)) throw StoreSignInRequired()
                if (live.accountId != owner) throw StoreOwnerChanged()
                live
            }
    }
}
