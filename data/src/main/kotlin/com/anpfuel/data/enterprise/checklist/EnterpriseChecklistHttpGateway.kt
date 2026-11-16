package com.anpfuel.data.enterprise.checklist

import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistGateway
import com.anpfuel.data.enterprise.CompanyAccessClient
import com.anpfuel.data.enterprise.strictBoolean
import com.anpfuel.data.enterprise.strictString
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.maintenance.checklist.Checklist
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistApplicable
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistCachedPolicy
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistCachedTemplate
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistContext
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistItem
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistMatch
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistOutcome
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistRunOperations
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistServerResult
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistServerRun
import com.anpfuel.domain.enterprise.maintenance.checklist.PolicyMode
import java.time.Instant
import org.json.JSONArray
import org.json.JSONObject

/**
 * EMP-CHECK-002 (EDR-283): the EMP-CHECK-001 applicable read and the run read on the account cookie plane the app
 * already uses for its context reads. Responses are parsed strictly and bound to the asked asset, moment and run; a
 * response for anything else is refused, never cached. JSON stays inside this adapter.
 */
class EnterpriseChecklistHttpGateway(private val client: CompanyAccessClient) : EnterpriseChecklistGateway {
    private val owner = CompanyAccessClient.Owner()
    private class BindingDenied : IllegalArgumentException("Checklist response unavailable")

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

    private fun <T> decode(result: EnterpriseAccessResult<JSONObject>, parser: (JSONObject) -> T): EnterpriseAccessResult<T> = when (result) {
        is EnterpriseAccessResult.Err -> result
        is EnterpriseAccessResult.Ok -> try { EnterpriseAccessResult.Ok(parser(result.value)) }
        catch (_: BindingDenied) { EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED) }
        catch (_: Exception) { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
    }

    override suspend fun applicable(context: EnterpriseContextSummary, assetId: String, checklistContext: ChecklistContext): EnterpriseAccessResult<ChecklistApplicable> {
        if (!Checklist.canonicalId(assetId)) return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
        val path = "/v1/orgs/${context.partition.organizationId}/checklists/assets/$assetId/applicable"
        return decode(post(context, path, JSONObject().put("context", checklistContext.name))) { root ->
            keys(root, APPLICABLE_KEYS, setOf("template", "policy"))
            if (root.strictString("asset_id") != assetId || root.strictString("context") != checklistContext.name) throw BindingDenied()
            require(root.strictString("data_mode") == DATA_MODE)
            val match = ChecklistMatch.entries.first { it.name == root.strictString("match") }
            val required = root.strictBoolean("required")
            require(required == (match != ChecklistMatch.NONE) && root.has("template") == required && root.has("policy") == required)
            ChecklistApplicable(assetId, checklistContext, match, if (required) template(root.getJSONObject("template")) else null,
                if (required) policy(root.getJSONObject("policy")) else null, instant(root.strictString("evaluated_at")))
        }
    }

    override suspend fun run(context: EnterpriseContextSummary, assetId: String, runId: String): EnterpriseAccessResult<ChecklistServerRun> {
        if (!Checklist.canonicalId(assetId) || !Checklist.canonicalId(runId)) return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
        val path = "/v1/orgs/${context.partition.organizationId}/checklists/assets/$assetId/runs/$runId/read"
        return decode(post(context, path, JSONObject())) { root ->
            keys(root, RUN_KEYS, setOf("released_at"))
            if (root.strictString("run_id") != runId || root.strictString("asset_id") != assetId) throw BindingDenied()
            require(root.strictString("data_mode") == DATA_MODE)
            val templateId = root.strictString("template_id").also { require(Checklist.canonicalId(it)) }
            val differs = root.strictBoolean("outcome_differs")
            require(root.strictBoolean("review_required") == differs)
            val result = ChecklistServerResult(counter(root, "revision"), counter(root, "server_policy_revision"), instant(root.strictString("received_at")),
                outcome(root, "device_outcome"), outcome(root, "recheck_outcome"), outcome(root, "outcome"), differs, differs,
                root.strictBoolean("template_superseded"), positions(root.getJSONArray("issues")), positions(root.getJSONArray("critical_issues")),
                if (root.has("released_at")) instant(root.strictString("released_at")) else null)
            ChecklistServerRun(runId, assetId, templateId, ChecklistContext.entries.first { it.name == root.strictString("context") },
                counter(root, "template_version"), counter(root, "policy_revision"), instant(root.strictString("started_at")),
                instant(root.strictString("completed_at")), root.strictBoolean("acknowledged"), result)
        }
    }

    private fun keys(value: JSONObject, required: Set<String>, optional: Set<String> = emptySet()) {
        val present = value.keys().asSequence().toSet()
        require(present.containsAll(required) && (required + optional).containsAll(present))
    }
    private fun counter(value: JSONObject, key: String): Long {
        val raw = value.get(key)
        val number = when (raw) { is Int -> raw.toLong(); is Long -> raw; else -> throw IllegalArgumentException() }
        require(number in 1..ChecklistRunOperations.MAX_COUNTER)
        return number
    }
    private fun outcome(value: JSONObject, key: String): ChecklistOutcome = ChecklistOutcome.entries.first { it.name == value.strictString(key) }
    private fun positions(values: JSONArray): List<Int> = (0 until values.length()).map { values.get(it) as? Int ?: throw IllegalArgumentException() }
    private fun instant(raw: String): Instant = EnterpriseTime.parseInstant(raw).also { require(EnterpriseTime.formatInstant(it) == raw) }
    private fun template(value: JSONObject): ChecklistCachedTemplate {
        keys(value, setOf("template_id", "name", "version", "published_at", "items"))
        val items = value.getJSONArray("items")
        require(items.length() in 1..40)
        return ChecklistCachedTemplate(value.strictString("template_id"), value.strictString("name"), counter(value, "version"),
            instant(value.strictString("published_at")), (0 until items.length()).map { index ->
                val item = items.getJSONObject(index)
                keys(item, setOf("position", "label", "critical", "optional", "photo_on_not_ok"))
                require(item.get("position") == index + 1)
                ChecklistItem(item.strictString("label"), item.strictBoolean("critical"), item.strictBoolean("optional"), item.strictBoolean("photo_on_not_ok"))
            })
    }
    private fun policy(value: JSONObject): ChecklistCachedPolicy {
        keys(value, setOf("revision", "mode", "validity_seconds", "effective_at"))
        val validity = value.get("validity_seconds") as? Int ?: throw IllegalArgumentException()
        return ChecklistCachedPolicy(counter(value, "revision"), PolicyMode.entries.first { it.name == value.strictString("mode") }, validity.toLong(),
            instant(value.strictString("effective_at")))
    }

    override fun toString(): String = "EnterpriseChecklistHttpGateway(redacted)"

    private companion object {
        const val DATA_MODE = "SYNTHETIC"
        val APPLICABLE_KEYS = setOf("asset_id", "context", "required", "match", "evaluated_at", "data_mode")
        val RUN_KEYS = setOf("run_id", "asset_id", "template_id", "context", "template_version", "policy_revision", "server_policy_revision", "revision",
            "started_at", "completed_at", "received_at", "device_outcome", "recheck_outcome", "outcome", "outcome_differs", "review_required",
            "template_superseded", "acknowledged", "issues", "critical_issues", "data_mode")
    }
}
