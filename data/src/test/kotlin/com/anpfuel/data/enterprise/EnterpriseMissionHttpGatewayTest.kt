package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.enterprise.trip.EnterpriseMissionHttpGateway
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.trip.TripMissionLoad
import com.anpfuel.domain.enterprise.trip.TripMissionResponse
import com.anpfuel.domain.enterprise.trip.TripMissionState
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

/** EMP-TRIP-021 (B-BR-EMP-527, EDR-342): the own mission list on the account cookie plane, parsed strictly and bound to the member. */
class EnterpriseMissionHttpGatewayTest {
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val org = "7b0e2a54-0000-4000-8000-000000000002"
    private val member = "7b0e2a54-0000-4000-8000-000000000003"
    private val employee = "7b0e2a54-0000-4000-8000-000000000005"
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val origin = "https://api.example.invalid"
    private val cookie = "__Host-empresas-session=abv_company_${"c".repeat(64)}; Path=/; Expires=Thu, 01 Jan 2099 08:00:00 GMT; Secure; HttpOnly; SameSite=Strict"
    private val context = EnterpriseContextSummary(EnterprisePartition.of(a, origin, org), member, "Synthetic company", employee)
    private fun login() = JSONObject().put("account_id", a).put("csrf_token", "abv_company_csrf_${"d".repeat(64)}")
        .put("authenticated_at", "2098-12-31T23:55:00Z").put("expires_at", "2099-01-01T08:00:00Z")
    private fun mission(n: Int, state: String = "ASSIGNED", revision: Int = 3) = JSONObject().put("mission_id", id(n)).put("unit_id", id(81))
        .put("asset_id", id(70)).put("title", "Entrega no cliente Norte").put("starts_at", "2026-11-20T11:00:00Z").put("ends_at", "2026-11-20T15:00:00Z")
        .put("state", state).put("revision", revision).put("assignee_employee_id", employee)
        .put("created_at", "2026-11-15T10:00:00.123456Z").put("updated_at", "2026-11-15T10:30:00.123456Z")
        .put("plan", JSONObject().put("revision", 2).put("destination", JSONObject().put("kind", "TEXT").put("text", "Cliente Norte, doca 3"))
            .put("reason", "Entrega programada").put("cost_centre_id", id(82)).put("load", JSONObject().put("kind", "CARGO").put("description", "Caixas"))
            .put("planned_at", "2026-11-15T10:10:00.123456Z"))
    private fun page(vararg items: JSONObject, cursor: String = "") = JSONObject().put("items", JSONArray().apply { items.forEach { put(it) } })
        .put("next_cursor", cursor).put("data_mode", "SYNTHETIC")

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
        val gateway = EnterpriseMissionHttpGateway(client)
    }
    private fun response(request: Request, body: String, status: Int = 200, cookie: Boolean = false): Response =
        Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(status).message("Synthetic response")
            .header("Cache-Control", "no-store").body(body.toResponseBody("application/json".toMediaType()))
            .apply { if (cookie) addHeader("Set-Cookie", this@EnterpriseMissionHttpGatewayTest.cookie) }.build()
    private fun body(request: Request): JSONObject = JSONObject(Buffer().also { request.body!!.writeTo(it) }.readUtf8())
    private fun answering(value: JSONObject, status: Int = 200) = Fixture { request, index ->
        if (index == 0) response(request, login().toString(), cookie = true) else response(request, value.toString(), status)
    }

    @Test fun `the own list is read with one page of 100 and parsed into exactly what the inbox keeps`() = runBlocking {
        val declined = mission(71, "DECLINED", 4).put("answer", JSONObject().put("response", "DECLINE").put("reason", "Folga marcada"))
            .apply { remove("asset_id") }.put("plan", mission(71).getJSONObject("plan").put("destination", JSONObject().put("kind", "UNIT")
                .put("unit_id", id(83))).put("load", JSONObject().put("kind", "PASSENGERS").put("count", 4)).apply { remove("cost_centre_id") })
        val fixture = answering(page(mission(70), declined, mission(72, "IN_PROGRESS", 6).put("answer", JSONObject().put("response", "ACCEPT"))
            .apply { remove("plan") }))
        val read = (fixture.gateway.mine(context) as EnterpriseAccessResult.Ok).value
        assertEquals(listOf("/v1/org-access/session/login", "/v1/orgs/$org/missions/mine/list"), fixture.requests.map { it.url.encodedPath })
        val request = fixture.requests[1]
        assertEquals(origin, request.header("Origin"))
        assertEquals(cookie.substringBefore(';'), request.header("Cookie"))
        assertEquals(JSONObject().put("limit", 100).toString(), body(request).toString())
        assertFalse(read.truncated)
        assertEquals(listOf(id(70), id(71), id(72)), read.items.map { it.missionId })
        val first = read.items[0]
        assertEquals(listOf(id(81), id(70), "Entrega no cliente Norte", employee), listOf(first.unitId, first.assetId, first.title, first.assigneeEmployeeId))
        assertEquals(TripMissionState.ASSIGNED, first.state); assertEquals(3L, first.revision); assertNull(first.answer)
        assertEquals(Instant.parse("2026-11-20T11:00:00Z"), first.startsAt)
        assertEquals(Instant.parse("2026-11-15T10:30:00.123456Z"), first.updatedAt)
        val plan = first.plan!!
        assertEquals(listOf("Cliente Norte, doca 3", "Entrega programada", id(82), "Caixas"), listOf(plan.destinationText, plan.reason, plan.costCentreId, plan.cargoDescription))
        assertEquals(TripMissionLoad.CARGO, plan.load); assertEquals(2L, plan.revision)
        val second = read.items[1]
        assertNull(second.assetId); assertEquals(TripMissionResponse.DECLINE, second.answer); assertEquals("Folga marcada", second.answerReason)
        assertEquals(id(83), second.plan!!.destinationUnitId); assertNull(second.plan!!.destinationText); assertEquals(4, second.plan!!.passengerCount)
        assertNull(second.plan!!.costCentreId)
        assertNull(read.items[2].plan); assertEquals(TripMissionResponse.ACCEPT, read.items[2].answer)
        assertFalse(read.toString().contains("Norte") || first.toString().contains(id(70)))
        assertTrue((answering(page(mission(70), cursor = "next-page")).gateway.mine(context) as EnterpriseAccessResult.Ok).value.truncated)
    }

    @Test fun `a malformed page is refused whole and another member's mission is a denial`() = runBlocking {
        val malformed = listOf<JSONObject.() -> Unit>(
            { put("extra", 1) }, { remove("data_mode") }, { put("data_mode", "REAL") }, { remove("next_cursor") },
            { getJSONArray("items").getJSONObject(0).put("note", "x") }, { getJSONArray("items").getJSONObject(0).put("state", "STARTED") },
            { getJSONArray("items").getJSONObject(0).put("revision", 0) }, { getJSONArray("items").getJSONObject(0).put("revision", "3") },
            { getJSONArray("items").getJSONObject(0).put("starts_at", "2026-11-20 11:00:00") },
            { getJSONArray("items").getJSONObject(0).put("mission_id", id(70).uppercase()) },
            { getJSONArray("items").getJSONObject(0).put("title", "") },
            { getJSONArray("items").getJSONObject(0).put("state", "ACCEPTED") },
            { getJSONArray("items").getJSONObject(0).getJSONObject("plan").getJSONObject("load").put("count", 3) },
            { getJSONArray("items").getJSONObject(0).getJSONObject("plan").getJSONObject("destination").put("kind", "COORDINATES") },
            { getJSONArray("items").getJSONObject(0).getJSONObject("plan").put("passenger_name", "Fulano") },
            { getJSONArray("items").getJSONObject(0).put("answer", JSONObject().put("response", "MAYBE")) },
        )
        for ((index, edit) in malformed.withIndex()) {
            assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE), answering(page(mission(70)).apply(edit)).gateway.mine(context), "case $index")
        }
        // An item of another assignee, or one without an assignee, makes the whole page a denial: nothing of it is returned.
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED),
            answering(page(mission(70), mission(71).put("assignee_employee_id", id(99)))).gateway.mine(context))
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED),
            answering(page(mission(70).apply { remove("assignee_employee_id") })).gateway.mine(context))
        // A refused read is the company client's safe UNAVAILABLE: the copy is kept and the screen says nothing was updated.
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE),
            answering(JSONObject().put("error", JSONObject().put("code", "FORBIDDEN")), 403).gateway.mine(context))
    }

    @Test fun `another account, no employee record or no session sends nothing`() = runBlocking {
        val fixture = answering(page(mission(70)))
        val other = EnterpriseContextSummary(EnterprisePartition.of(member, origin, org), member, "Synthetic company", employee)
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED), fixture.gateway.mine(other))
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE), fixture.gateway.mine(context.copy(employeeId = null)))
        assertTrue(fixture.requests.isEmpty())
        fixture.stored = null
        assertEquals(EnterpriseAccessResult.Err(EnterpriseAccessError.SIGN_IN_REQUIRED), fixture.gateway.mine(context))
        assertTrue(fixture.requests.isEmpty())
    }
}
