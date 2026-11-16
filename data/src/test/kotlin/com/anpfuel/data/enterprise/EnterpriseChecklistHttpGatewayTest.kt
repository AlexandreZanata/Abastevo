package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.enterprise.checklist.EnterpriseChecklistHttpGateway
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistContext
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistMatch
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistOutcome
import com.anpfuel.domain.enterprise.maintenance.checklist.PolicyMode
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.every
import io.mockk.mockk
import java.time.Instant
import kotlinx.coroutines.runBlocking
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import okio.Buffer
import org.json.JSONArray
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** EMP-CHECK-002 (EDR-283): the applicable read and the run read on the account cookie plane, parsed strictly and bound to the request. */
class EnterpriseChecklistHttpGatewayTest {
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val org = "7b0e2a54-0000-4000-8000-000000000002"
    private val member = "7b0e2a54-0000-4000-8000-000000000003"
    private val asset = "7b0e2a54-0000-4000-8000-000000000021"
    private val template = "7b0e2a54-0000-4000-8000-000000000031"
    private val run = "7b0e2a54-0000-4000-8000-000000000041"
    private val origin = "https://api.example.invalid"
    private val cookie = "__Host-empresas-session=abv_company_${"c".repeat(64)}; Path=/; Expires=Thu, 01 Jan 2099 08:00:00 GMT; Secure; HttpOnly; SameSite=Strict"
    private val context = EnterpriseContextSummary(EnterprisePartition.of(a, origin, org), member, "Synthetic company")
    private fun login() = JSONObject().put("account_id", a).put("csrf_token", "abv_company_csrf_${"d".repeat(64)}")
        .put("authenticated_at", "2098-12-31T23:55:00Z").put("expires_at", "2099-01-01T08:00:00Z")
    private fun item(position: Int, label: String, critical: Boolean = false, optional: Boolean = false, photo: Boolean = false) = JSONObject()
        .put("position", position).put("label", label).put("critical", critical).put("optional", optional).put("photo_on_not_ok", photo)
    private fun applicable(match: String = "ORGANIZATION") = JSONObject().put("asset_id", asset).put("context", "DEPARTURE").put("required", match != "NONE")
        .put("match", match).put("evaluated_at", "2026-11-15T12:30:00.25Z").put("data_mode", "SYNTHETIC").apply {
            if (match != "NONE") {
                put("template", JSONObject().put("template_id", template).put("name", "Checklist de saída").put("version", 2)
                    .put("published_at", "2026-11-15T12:00:00Z").put("items", JSONArray().put(item(1, "Luzes", optional = true))
                        .put(item(2, "Freios", critical = true)).put(item(3, "Pneus", photo = true))))
                put("policy", JSONObject().put("revision", 3).put("mode", "WARN").put("validity_seconds", 7200).put("effective_at", "2026-11-15T12:05:00Z"))
            }
        }
    private fun runRead(revision: Int = 1) = JSONObject().put("run_id", run).put("asset_id", asset).put("template_id", template).put("context", "DEPARTURE")
        .put("template_version", 1).put("policy_revision", 2).put("server_policy_revision", 3).put("revision", revision)
        .put("started_at", "2026-11-15T11:40:00Z").put("completed_at", "2026-11-15T12:00:00.123Z").put("received_at", "2026-11-15T12:30:00.5Z")
        .put("device_outcome", "COMPLETED_WITH_ISSUES").put("recheck_outcome", "BLOCKING").put("outcome", if (revision == 2) "COMPLETED_WITH_ISSUES" else "BLOCKING")
        .put("outcome_differs", true).put("review_required", true).put("template_superseded", false).put("acknowledged", true)
        .put("issues", JSONArray().put(2)).put("critical_issues", JSONArray().put(2)).put("data_mode", "SYNTHETIC")
        .apply { if (revision == 2) put("released_at", "2026-11-15T13:00:00Z") }

    private inner class Fixture(private val handle: (Request, Int) -> Response) {
        val fence = EnterpriseSessionFence()
        var stored: PortableAuth.Session? = PortableAuth.Session(a, a, "synthetic-access", "synthetic-refresh", 2000, 4000)
        val sessions = object : AuthSessionStore {
            override fun load() = stored
            override fun save(session: PortableAuth.Session) { stored = session }
            override fun clear() { stored = null }
        }
        val auth = mockk<AuthFlow>().also { every { it.refreshSession() } answers { AuthApiResult.Ok(stored) } }
        val requests = java.util.Collections.synchronizedList(mutableListOf<Request>())
        val client = CompanyAccessClient(OkHttpClient.Builder().addInterceptor(Interceptor { chain ->
            val index = synchronized(requests) { requests.add(chain.request()); requests.size - 1 }
            handle(chain.request(), index)
        }).build(), origin, sessions, auth, fence)
        val gateway = EnterpriseChecklistHttpGateway(client)
    }
    private fun response(request: Request, body: String, status: Int = 200, cookie: Boolean = false): Response =
        Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(status).message("Synthetic response")
            .header("Cache-Control", "no-store").body(body.toResponseBody("application/json".toMediaType()))
            .apply { if (cookie) addHeader("Set-Cookie", this@EnterpriseChecklistHttpGatewayTest.cookie) }.build()
    private fun body(request: Request): JSONObject = JSONObject(Buffer().also { request.body!!.writeTo(it) }.readUtf8())
    private fun answering(value: JSONObject, status: Int = 200) = Fixture { request, index ->
        if (index == 0) response(request, login().toString(), cookie = true) else response(request, value.toString(), status)
    }

    @Test fun `the applicable read opens the company channel once and is parsed into exactly what a run pins`() = runBlocking {
        val fixture = answering(applicable())
        val first = (fixture.gateway.applicable(context, asset, ChecklistContext.DEPARTURE) as EnterpriseAccessResult.Ok).value
        val second = fixture.gateway.applicable(context, asset, ChecklistContext.DEPARTURE)
        assertTrue(second is EnterpriseAccessResult.Ok)
        assertEquals(listOf("/v1/org-access/session/login", "/v1/orgs/$org/checklists/assets/$asset/applicable",
            "/v1/orgs/$org/checklists/assets/$asset/applicable"), fixture.requests.map { it.url.encodedPath })
        for (request in fixture.requests.drop(1)) {
            assertEquals(origin, request.header("Origin"))
            assertEquals(cookie.substringBefore(';'), request.header("Cookie"))
            assertEquals("abv_company_csrf_${"d".repeat(64)}", request.header("X-CSRF"))
            assertEquals(JSONObject().put("context", "DEPARTURE").toString(), body(request).toString())
        }
        assertEquals(asset, first.assetId)
        assertEquals(ChecklistContext.DEPARTURE, first.context)
        assertEquals(ChecklistMatch.ORGANIZATION, first.match)
        assertEquals(Instant.parse("2026-11-15T12:30:00.25Z"), first.evaluatedAt)
        val pinned = first.template!!
        assertEquals(listOf(template, "Checklist de saída", "2"), listOf(pinned.templateId, pinned.name, pinned.version.toString()))
        assertEquals(listOf("Luzes", "Freios", "Pneus"), pinned.items.map { it.label })
        assertEquals(listOf(false, true, false), pinned.items.map { it.critical })
        assertEquals(listOf(true, false, false), pinned.items.map { it.optional })
        assertEquals(listOf(false, false, true), pinned.items.map { it.photoOnNotOk })
        val policy = first.policy!!
        assertEquals(listOf(3L, 7200L), listOf(policy.revision, policy.validitySeconds))
        assertEquals(PolicyMode.WARN, policy.mode)
        assertFalse(first.toString().contains("Freios") || first.toString().contains(asset))
        val none = (answering(applicable("NONE")).gateway.applicable(context, asset, ChecklistContext.DEPARTURE) as EnterpriseAccessResult.Ok).value
        assertFalse(none.required)
        assertNull(none.template); assertNull(none.policy)
    }

    @Test fun `a malformed or foreign applicable read changes nothing`() = runBlocking {
        val malformed = listOf<JSONObject.() -> Unit>(
            { put("extra", 1) }, { remove("data_mode") }, { put("data_mode", "REAL") }, { put("required", false) },
            { put("evaluated_at", "2026-11-15 12:30:00") }, { getJSONObject("template").put("version", 0) },
            { getJSONObject("template").put("version", "2") }, { getJSONObject("template").put("version", 2.5) },
            { getJSONObject("template").getJSONArray("items").getJSONObject(0).put("position", 2) },
            { getJSONObject("template").getJSONArray("items").getJSONObject(1).put("optional", true) },
            { getJSONObject("template").put("items", JSONArray()) }, { getJSONObject("template").put("name", "") },
            { getJSONObject("policy").put("validity_seconds", 60) }, { getJSONObject("policy").put("mode", "ALLOW") },
            { remove("policy") }, { put("match", "SOMEWHERE") },
            { getJSONObject("template").getJSONArray("items").getJSONObject(0).put("label", "a\u0000b") },
        )
        for ((index, edit) in malformed.withIndex()) {
            val value = applicable().apply(edit)
            assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE),
                answering(value).gateway.applicable(context, asset, ChecklistContext.DEPARTURE), "case $index")
        }
        val withTemplate = applicable("NONE").put("template", applicable().getJSONObject("template")).put("policy", applicable().getJSONObject("policy"))
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE), answering(withTemplate).gateway.applicable(context, asset, ChecklistContext.DEPARTURE))
        for (foreign in listOf(applicable().put("asset_id", run), applicable().put("context", "RETURN"))) {
            assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED),
                answering(foreign).gateway.applicable(context, asset, ChecklistContext.DEPARTURE))
        }
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE),
            answering(JSONObject().put("error", JSONObject().put("code", "NOT_FOUND")), 404).gateway.applicable(context, asset, ChecklistContext.DEPARTURE))
    }

    @Test fun `the run read returns both outcomes for the asked run only`() = runBlocking {
        val fixture = answering(runRead())
        val read = (fixture.gateway.run(context, asset, run) as EnterpriseAccessResult.Ok).value
        assertEquals("/v1/orgs/$org/checklists/assets/$asset/runs/$run/read", fixture.requests[1].url.encodedPath)
        assertEquals("{}", body(fixture.requests[1]).toString())
        assertEquals(listOf(run, asset, template), listOf(read.runId, read.assetId, read.templateId))
        assertEquals(listOf(1L, 2L), listOf(read.templateVersion, read.policyRevision))
        assertEquals(Instant.parse("2026-11-15T12:00:00.123Z"), read.completedAt)
        assertTrue(read.acknowledged)
        val result = read.result
        assertEquals(listOf(ChecklistOutcome.COMPLETED_WITH_ISSUES, ChecklistOutcome.BLOCKING, ChecklistOutcome.BLOCKING),
            listOf(result.deviceOutcome, result.recheckOutcome, result.outcome))
        assertTrue(result.outcomeDiffers && result.reviewRequired)
        assertEquals(3, result.serverPolicyRevision)
        assertEquals(listOf(2), result.criticalIssues)
        assertNull(result.releasedAt)
        val released = (answering(runRead(2)).gateway.run(context, asset, run) as EnterpriseAccessResult.Ok).value.result
        assertEquals(ChecklistOutcome.COMPLETED_WITH_ISSUES, released.outcome)
        assertEquals(Instant.parse("2026-11-15T13:00:00Z"), released.releasedAt)
        for (bad in listOf(runRead().put("released_at", "2026-11-15T13:00:00Z"), runRead(2).apply { remove("released_at") },
            runRead().put("review_required", false), runRead().put("note", "x"), runRead().put("issues", JSONArray().put(0)),
            runRead().put("revision", 3), runRead().put("recheck_outcome", "MAYBE"))) {
            assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE), answering(bad).gateway.run(context, asset, run))
        }
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED), answering(runRead().put("run_id", template)).gateway.run(context, asset, run))
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED), answering(runRead().put("asset_id", run)).gateway.run(context, asset, run))
    }

    @Test fun `another account, a malformed selector or no session sends nothing`() = runBlocking {
        val fixture = answering(applicable())
        val other = EnterpriseContextSummary(EnterprisePartition.of(member, origin, org), member, "Synthetic company")
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED), fixture.gateway.applicable(other, asset, ChecklistContext.DEPARTURE))
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE), fixture.gateway.applicable(context, asset.uppercase(), ChecklistContext.DEPARTURE))
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE), fixture.gateway.run(context, asset, "../$run"))
        assertTrue(fixture.requests.isEmpty())
        fixture.stored = null
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.SIGN_IN_REQUIRED), fixture.gateway.applicable(context, asset, ChecklistContext.DEPARTURE))
        assertTrue(fixture.requests.isEmpty())
    }
}
