package com.anpfuel.data.remote.ledger

import com.anpfuel.data.remote.PhotoOperationRefused
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.repository.ContributionRateLimited
import com.anpfuel.domain.repository.LedgerConsentStatus
import com.anpfuel.domain.repository.LedgerSyncConflict
import com.anpfuel.domain.repository.LedgerSyncDenied
import com.anpfuel.domain.repository.LedgerSyncReceipt
import java.io.File
import java.io.IOException
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * Ledger writes (`/v1/ledger/commands`, `/v1/ledger/consent`) go through the
 * signed transport only: the backend resolves their owner from the session
 * and the device key bound to it, and refuses unsigned writes.
 */
class LedgerSyncHttpClientTest {
    private val operationId = "11111111-1111-4111-8111-111111111111"
    private val recordId = "22222222-2222-4222-8222-222222222222"
    private val consentKey = "33333333-3333-4333-8333-333333333333"
    private val scope = PremiumOwnerScope("acct-1", "https://api.example.test")

    private data class Sent(val path: String, val key: String, val owner: String, val body: JSONObject)

    private fun okBody(revision: Long = 2, replayed: Boolean = false, id: String = operationId) = JSONObject(
        """{"results":[{"operation_id":"$id","dataset":"refuel","id":"$recordId","revision":$revision,"status":"accepted","replayed":$replayed}]}""",
    )

    private fun command(payloadJson: String = """{"a":1}""", delete: Boolean = false) = PremiumCommand(
        operationId = operationId, scope = scope, dataset = "refuel", recordId = recordId,
        expectedRevision = 0, payloadJson = payloadJson, delete = delete,
    )

    private fun client(answer: (Sent) -> JSONObject): Pair<LedgerSyncHttpClient, MutableList<Sent>> {
        val sent = mutableListOf<Sent>()
        val api = LedgerSyncHttpClient(
            http = OkHttpClient(),
            baseUrl = "https://api.example.test/",
            signed = LedgerSignedPost { path, key, owner, body ->
                Sent(path, key, owner, body).also { sent += it }.let(answer)
            },
            newKey = { consentKey },
        )
        return api to sent
    }

    private fun vector(name: String): JSONObject =
        JSONObject(File("../contracts/testdata/api/$name.json").readText()).getJSONObject("value")

    @Test fun `submit signs the exact one-command batch for the owner`() = runTest {
        val (api, sent) = client { okBody() }
        val receipt = api.submit(scope, command(), "attempt-1", "fam-1", "tok")

        assertEquals(LedgerSyncReceipt(operationId, 2, false), receipt)
        val call = sent.single()
        assertEquals("/v1/ledger/commands", call.path)
        assertEquals(operationId, call.key)
        assertEquals("acct-1", call.owner)
        assertEquals(vector("ledger-commands-valid").keySet(), call.body.keySet())
        assertEquals("fam-1", call.body.getString("family_id"))
        assertEquals("tok", call.body.getString("access_token"))
        val row = call.body.getJSONArray("commands").let { assertEquals(1, it.length()); it.getJSONObject(0) }
        assertEquals(
            setOf("operation_id", "dataset", "id", "expected_revision", "delete", "void", "payload"),
            row.keySet(),
        )
        assertEquals(operationId, row.getString("operation_id"))
        assertEquals("refuel", row.getString("dataset"))
        assertEquals(recordId, row.getString("id"))
        assertEquals(0L, row.getLong("expected_revision"))
        assertFalse(row.getBoolean("delete"))
        assertEquals(1, row.getJSONObject("payload").getInt("a"))
    }

    @Test fun `a delete sends a null payload`() = runTest {
        val (api, sent) = client { okBody() }
        api.submit(scope, command(payloadJson = "", delete = true), "attempt-1", "fam-1", "tok")
        val row = sent.single().body.getJSONArray("commands").getJSONObject(0)
        assertTrue(row.getBoolean("delete"))
        assertTrue(row.isNull("payload"))
    }

    @Test fun `a missing session never reaches the transport`() = runTest {
        val (api, sent) = client { okBody() }
        assertThrows(LedgerSyncDenied::class.java) { runBlocking { api.submit(scope, command(), "a", "", "tok") } }
        assertThrows(LedgerSyncDenied::class.java) { runBlocking { api.submit(scope, command(), "a", "fam", "") } }
        assertTrue(sent.isEmpty())
    }

    @Test fun `signed command refusals map to conflict, denial or retry`() = runTest {
        val cases: Map<Exception, Class<out Exception>> = mapOf(
            PhotoOperationRefused(409) to LedgerSyncConflict::class.java,
            PhotoOperationRefused(400) to LedgerSyncConflict::class.java,
            PhotoOperationRefused(401) to LedgerSyncDenied::class.java,
            PhotoOperationRefused(403, "ledger.capability-required") to LedgerSyncDenied::class.java,
            PhotoOperationRefused(404) to LedgerSyncDenied::class.java,
            PhotoOperationRefused(429) to IOException::class.java,
            PhotoOperationRefused(503, retryAfterSeconds = 30) to IOException::class.java,
            ContributionRateLimited(60) to IOException::class.java,
            IOException("timeout") to IOException::class.java,
            IOException("photo.identity-changed") to LedgerSyncDenied::class.java,
            LedgerOwnerChanged() to LedgerSyncDenied::class.java,
            LedgerSignInRequired() to LedgerSyncDenied::class.java,
        )
        cases.forEach { (failure, expected) ->
            val (api, _) = client { throw failure }
            val thrown = assertThrows(Exception::class.java) {
                runBlocking { api.submit(scope, command(), "attempt-1", "fam-1", "tok") }
            }
            assertTrue(expected.isInstance(thrown), "$failure -> $thrown")
            if (expected == IOException::class.java) {
                assertFalse(thrown is LedgerSyncDenied || thrown is LedgerSyncConflict, "$failure -> $thrown")
            }
        }
    }

    @Test fun `receipts for another operation or without a revision are refused`() = runTest {
        listOf(
            okBody(id = "99999999-9999-4999-8999-999999999999"),
            JSONObject("""{"results":[]}"""),
            JSONObject("""{"results":[{"operation_id":"$operationId"}]}"""),
            JSONObject(),
        ).forEach { body ->
            val (api, _) = client { body }
            assertThrows(LedgerSyncDenied::class.java, { runBlocking { api.submit(scope, command(), "a", "f", "t") } }, body.toString())
        }
    }

    @Test fun `consent signs the datasets and the explicit choice`() = runTest {
        val (api, sent) = client { JSONObject("""{"revision":4,"datasets":["refuel","expense"]}""") }
        val status = api.setConsent("acct-1", "fam-1", "tok", listOf(LedgerDataset.EXPENSE, LedgerDataset.REFUEL), enabled = true)

        assertEquals(LedgerConsentStatus(4, listOf(LedgerDataset.REFUEL, LedgerDataset.EXPENSE)), status)
        val call = sent.single()
        assertEquals("/v1/ledger/consent", call.path)
        assertEquals(consentKey, call.key)
        assertEquals("acct-1", call.owner)
        assertEquals(vector("ledger-consent-valid").keySet(), call.body.keySet())
        assertEquals("fam-1", call.body.getString("family_id"))
        assertEquals("tok", call.body.getString("access_token"))
        assertEquals(listOf("refuel", "expense"), call.body.getJSONArray("datasets").let { a -> (0 until a.length()).map(a::getString) })
        assertTrue(call.body.getBoolean("enabled"))
    }

    @Test fun `consent revocation sends enabled false`() = runTest {
        val (api, sent) = client { JSONObject("""{"revision":5,"datasets":[]}""") }
        val status = api.setConsent("acct-1", "fam-1", "tok", LedgerDataset.entries, enabled = false)
        assertEquals(LedgerConsentStatus(5, emptyList()), status)
        assertFalse(sent.single().body.getBoolean("enabled"))
    }

    @Test fun `invalid consent requests never reach the transport`() = runTest {
        val (api, sent) = client { JSONObject("""{"revision":1,"datasets":[]}""") }
        listOf<suspend () -> Unit>(
            { api.setConsent("acct-1", "fam-1", "tok", emptyList(), true) },
            { api.setConsent("acct-1", "fam-1", "tok", listOf(LedgerDataset.REFUEL, LedgerDataset.REFUEL), true) },
            { api.setConsent("", "fam-1", "tok", listOf(LedgerDataset.REFUEL), true) },
            { api.setConsent("acct-1", "", "tok", listOf(LedgerDataset.REFUEL), true) },
            { api.setConsent("acct-1", "fam-1", "", listOf(LedgerDataset.REFUEL), true) },
        ).forEach { call -> assertThrows(LedgerSyncDenied::class.java) { runBlocking { call() } } }
        assertTrue(sent.isEmpty())
    }

    @Test fun `malformed consent answers and refusals fail closed`() = runTest {
        listOf(
            JSONObject("""{"datasets":["refuel"]}"""),
            JSONObject("""{"revision":-1,"datasets":["refuel"]}"""),
            JSONObject("""{"revision":"1","datasets":["refuel"]}"""),
            JSONObject("""{"revision":1.5,"datasets":["refuel"]}"""),
            JSONObject("""{"revision":1,"datasets":["professional"]}"""),
            JSONObject("""{"revision":1,"datasets":["refuel","refuel"]}"""),
            JSONObject("""{"revision":1}"""),
        ).forEach { body ->
            val (api, _) = client { body }
            assertThrows(LedgerSyncDenied::class.java, {
                runBlocking { api.setConsent("acct-1", "f", "t", listOf(LedgerDataset.REFUEL), true) }
            }, body.toString())
        }
        mapOf(
            PhotoOperationRefused(400) to LedgerSyncDenied::class.java,
            PhotoOperationRefused(401) to LedgerSyncDenied::class.java,
            PhotoOperationRefused(403) to LedgerSyncDenied::class.java,
            PhotoOperationRefused(503) to IOException::class.java,
            IOException("timeout") to IOException::class.java,
            LedgerOwnerChanged() to LedgerSyncDenied::class.java,
        ).forEach { (failure, expected) ->
            val (api, _) = client { throw failure }
            val thrown = assertThrows(Exception::class.java) {
                runBlocking { api.setConsent("acct-1", "f", "t", listOf(LedgerDataset.REFUEL), true) }
            }
            assertTrue(expected.isInstance(thrown), "$failure -> $thrown")
        }
    }
}
