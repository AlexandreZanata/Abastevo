package com.anpfuel.data.remote.premium

import com.anpfuel.data.remote.PhotoOperationRefused
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.StoreProvider
import com.anpfuel.domain.premium.StorePurchaseKind
import com.anpfuel.domain.premium.StoreVerification
import com.anpfuel.domain.repository.ContributionRateLimited
import java.io.IOException
import kotlinx.coroutines.test.runTest
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class StorePurchaseHttpClientTest {
    private val operationId = "11111111-1111-4111-8111-111111111111"
    private val status = """
        {"policy_version":1,"state":"ACTIVE","reason":"verified",
         "capabilities":["cloud.sync"],"valid_until":"2026-12-31T00:00:00Z",
         "retained_data_access":true,"payments_configured":true,
         "tiers":[{"tier":"PRO","source":"play","valid_until":"2026-12-31T00:00:00Z"}]}
    """.trimIndent()

    private data class Sent(val path: String, val key: String, val owner: String, val body: JSONObject)

    private fun client(answer: (Sent) -> JSONObject): Pair<StorePurchaseHttpClient, MutableList<Sent>> {
        val sent = mutableListOf<Sent>()
        val client = StorePurchaseHttpClient { path, key, owner, body ->
            val call = Sent(path, key, owner, body)
            sent += call
            answer(call)
        }
        return client to sent
    }

    private fun receipt(replayed: Boolean = false, id: String = operationId) =
        JSONObject().put("operation_id", id).put("status", JSONObject(status)).put("replayed", replayed)

    @Test fun `verify sends only the purchase key and operation id on the signed route`() = runTest {
        val (api, sent) = client { receipt() }
        val result = api.submit(StorePurchaseKind.VERIFY, "acct", operationId, "purchase-token", StoreProvider.PLAY)

        val verified = result as StoreVerification.Verified
        assertEquals(PremiumState.ACTIVE, verified.status.state)
        assertFalse(verified.replayed)
        val call = sent.single()
        assertEquals("/v1/premium/purchases/verify", call.path)
        assertEquals(operationId, call.key)
        assertEquals("acct", call.owner)
        assertEquals(setOf("operation_id", "purchase_token", "provider"), call.body.keySet())
        assertEquals("purchase-token", call.body.getString("purchase_token"))
        assertEquals("play", call.body.getString("provider"))
    }

    @Test fun `restore uses its own route and reports a replay`() = runTest {
        val (api, sent) = client { receipt(replayed = true) }
        val result = api.submit(StorePurchaseKind.RESTORE, "acct", operationId, "purchase-token", StoreProvider.PLAY)
        assertTrue((result as StoreVerification.Verified).replayed)
        assertEquals("/v1/premium/purchases/restore", sent.single().path)
    }

    @Test fun `invalid inputs are refused before any request`() = runTest {
        val (api, sent) = client { receipt() }
        assertEquals(StoreVerification.Invalid, api.submit(StorePurchaseKind.VERIFY, "acct", "NOT-AN-ID", "t", StoreProvider.PLAY))
        assertEquals(StoreVerification.Invalid, api.submit(StorePurchaseKind.VERIFY, "acct", operationId, "has space", StoreProvider.PLAY))
        assertEquals(StoreVerification.Invalid, api.submit(StorePurchaseKind.VERIFY, "", operationId, "t", StoreProvider.PLAY))
        assertTrue(sent.isEmpty())
    }

    @Test fun `http refusals map to typed verdicts`() = runTest {
        val cases = mapOf(
            PhotoOperationRefused(400) to StoreVerification.Invalid,
            PhotoOperationRefused(401) to StoreVerification.SignInRequired,
            PhotoOperationRefused(403, "subscription.proof-denied") to StoreVerification.Denied,
            PhotoOperationRefused(409, "subscription.retry-verification") to StoreVerification.RetryLater(null),
            PhotoOperationRefused(429) to StoreVerification.RetryLater(null),
            PhotoOperationRefused(503, retryAfterSeconds = 30) to StoreVerification.Unavailable,
            ContributionRateLimited(60) to StoreVerification.RetryLater(60),
            IOException("timeout") to StoreVerification.Unavailable,
            IOException("photo.identity-changed") to StoreVerification.OwnerChanged,
            StoreOwnerChanged() to StoreVerification.OwnerChanged,
            StoreSignInRequired() to StoreVerification.SignInRequired,
            IllegalStateException("boom") to StoreVerification.Unavailable,
        )
        cases.forEach { (failure, expected) ->
            val (api, _) = client { throw failure }
            assertEquals(expected, api.submit(StorePurchaseKind.VERIFY, "acct", operationId, "t", StoreProvider.PLAY), failure.toString())
        }
    }

    @Test fun `malformed receipts never become a verified status`() = runTest {
        listOf(
            receipt(id = "22222222-2222-4222-8222-222222222222"),
            receipt().apply { remove("replayed") },
            receipt().put("replayed", "false"),
            receipt().put("status", "ACTIVE"),
            receipt().put("status", JSONObject(status).put("retained_data_access", false)),
            JSONObject(),
        ).forEach { body ->
            val (api, _) = client { body }
            assertEquals(StoreVerification.Invalid, api.submit(StorePurchaseKind.VERIFY, "acct", operationId, "t", StoreProvider.PLAY), body.toString())
        }
    }
}
