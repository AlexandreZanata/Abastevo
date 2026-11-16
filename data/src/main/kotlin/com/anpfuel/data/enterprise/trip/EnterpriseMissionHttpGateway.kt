package com.anpfuel.data.enterprise.trip

import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.trip.EnterpriseMissionGateway
import com.anpfuel.data.enterprise.CompanyAccessClient
import com.anpfuel.data.enterprise.strictString
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.trip.MissionInbox
import com.anpfuel.domain.enterprise.trip.MissionInboxItem
import com.anpfuel.domain.enterprise.trip.MissionInboxPage
import com.anpfuel.domain.enterprise.trip.MissionPlanView
import com.anpfuel.domain.enterprise.trip.TripMissionLoad
import com.anpfuel.domain.enterprise.trip.TripMissionResponse
import com.anpfuel.domain.enterprise.trip.TripMissionState
import java.time.Instant
import org.json.JSONObject

/**
 * EMP-TRIP-021 (EDR-342): the EMP-TRIP-008 own list `POST /v1/orgs/{org_id}/missions/mine/list` on the account cookie plane
 * the app already uses for its context reads. One page of 100; the answer is parsed strictly and refused whole when any item
 * is assigned to another employee record than the context's own; JSON stays inside this adapter.
 */
class EnterpriseMissionHttpGateway(private val client: CompanyAccessClient) : EnterpriseMissionGateway {
    private val owner = CompanyAccessClient.Owner()
    private class BindingDenied : IllegalArgumentException("Mission response unavailable")

    /** Reuses this owner's company channel; a missing or stolen channel is opened once, then the request is made. */
    private suspend fun post(context: EnterpriseContextSummary, path: String, body: JSONObject): EnterpriseAccessResult<JSONObject> {
        val binding = client.currentBinding() ?: return EnterpriseAccessResult.Err(EnterpriseAccessError.SIGN_IN_REQUIRED)
        if (binding.accountId != context.partition.ownerAccount || binding.origin != context.partition.origin) {
            return EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED)
        }
        val first = client.post(owner, binding, path, body)
        if (first !is EnterpriseAccessResult.Err || first.error != EnterpriseAccessError.STALE) return first
        when (val opened = client.open(owner)) {
            is EnterpriseAccessResult.Err -> return opened
            is EnterpriseAccessResult.Ok -> if (opened.value != binding) return EnterpriseAccessResult.Err(EnterpriseAccessError.STALE)
        }
        return client.post(owner, binding, path, body)
    }

    override suspend fun mine(context: EnterpriseContextSummary): EnterpriseAccessResult<MissionInboxPage> {
        val employee = context.employeeId ?: return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
        val path = "/v1/orgs/${context.partition.organizationId}/missions/mine/list"
        return when (val result = post(context, path, JSONObject().put("limit", MissionInbox.MAX_ITEMS))) {
            is EnterpriseAccessResult.Err -> result
            is EnterpriseAccessResult.Ok -> try { EnterpriseAccessResult.Ok(page(result.value, employee)) }
                catch (_: BindingDenied) { EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED) }
                catch (_: Exception) { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
        }
    }

    private fun page(root: JSONObject, employee: String): MissionInboxPage {
        keys(root, setOf("items", "next_cursor", "data_mode"))
        require(root.strictString("data_mode") == DATA_MODE)
        val cursor = root.strictString("next_cursor")
        val items = root.getJSONArray("items")
        require(items.length() <= MissionInbox.MAX_ITEMS)
        val parsed = (0 until items.length()).map { item(items.getJSONObject(it)) }
        // The own list names only the member's own missions; any other assignee is a foreign answer, kept nowhere.
        if (parsed.any { it.second != employee }) throw BindingDenied()
        return MissionInboxPage(parsed.map { it.first }, cursor.isNotEmpty())
    }

    private fun item(value: JSONObject): Pair<MissionInboxItem, String?> {
        keys(value, ITEM_KEYS, setOf("asset_id", "assignee_employee_id", "answer", "plan"))
        val assignee = if (value.has("assignee_employee_id")) id(value.strictString("assignee_employee_id")) else null
        val answer = if (value.has("answer")) value.getJSONObject("answer").also { keys(it, setOf("response"), setOf("reason")) } else null
        val item = MissionInboxItem(id(value.strictString("mission_id")), id(value.strictString("unit_id")),
            if (value.has("asset_id")) id(value.strictString("asset_id")) else null, value.strictString("title"),
            instant(value.strictString("starts_at")), instant(value.strictString("ends_at")),
            TripMissionState.entries.first { it.name == value.strictString("state") }, counter(value, "revision"),
            assignee ?: throw BindingDenied(),
            answer?.let { a -> TripMissionResponse.entries.first { it.name == a.strictString("response") } },
            answer?.takeIf { it.has("reason") }?.strictString("reason"),
            if (value.has("plan")) plan(value.getJSONObject("plan")) else null,
            instant(value.strictString("created_at")), instant(value.strictString("updated_at")))
        return item to assignee
    }

    private fun plan(value: JSONObject): MissionPlanView {
        keys(value, setOf("revision", "destination", "reason", "load", "planned_at"), setOf("cost_centre_id"))
        val destination = value.getJSONObject("destination")
        val (text, unit) = when (destination.strictString("kind")) {
            "TEXT" -> { keys(destination, setOf("kind", "text")); destination.strictString("text") to null }
            "UNIT" -> { keys(destination, setOf("kind", "unit_id")); null to id(destination.strictString("unit_id")) }
            else -> throw IllegalArgumentException()
        }
        val load = value.getJSONObject("load")
        val kind = TripMissionLoad.entries.first { it.name == load.strictString("kind") }
        when (kind) {
            TripMissionLoad.NONE -> keys(load, setOf("kind"))
            TripMissionLoad.CARGO -> keys(load, setOf("kind", "description"))
            TripMissionLoad.PASSENGERS -> keys(load, setOf("kind", "count"))
        }
        return MissionPlanView(counter(value, "revision"), text, unit, value.strictString("reason"),
            if (value.has("cost_centre_id")) id(value.strictString("cost_centre_id")) else null, kind,
            if (kind == TripMissionLoad.CARGO) load.strictString("description") else null,
            if (kind == TripMissionLoad.PASSENGERS) (load.get("count") as? Int ?: throw IllegalArgumentException()) else null,
            instant(value.strictString("planned_at")))
    }

    private fun keys(value: JSONObject, required: Set<String>, optional: Set<String> = emptySet()) {
        val present = value.keys().asSequence().toSet()
        require(present.containsAll(required) && (required + optional).containsAll(present))
    }
    private fun id(raw: String): String = EnterprisePartition.requireId(raw)
    private fun counter(value: JSONObject, key: String): Long = when (val raw = value.get(key)) {
        is Int -> raw.toLong()
        is Long -> raw
        else -> throw IllegalArgumentException()
    }.also { require(it >= 1) }
    private fun instant(raw: String): Instant = EnterpriseTime.parseInstant(raw).also { require(EnterpriseTime.formatInstant(it) == raw) }

    override fun toString(): String = "EnterpriseMissionHttpGateway(redacted)"

    private companion object {
        const val DATA_MODE = "SYNTHETIC"
        val ITEM_KEYS = setOf("mission_id", "unit_id", "title", "starts_at", "ends_at", "state", "revision", "created_at", "updated_at")
    }
}
