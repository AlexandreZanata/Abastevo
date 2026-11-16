package com.anpfuel.data.enterprise.tracking

import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingPolicyGateway
import com.anpfuel.data.enterprise.CompanyAccessClient
import com.anpfuel.data.enterprise.strictString
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.tracking.TrackingPolicy
import com.anpfuel.domain.enterprise.tracking.TrackingPolicyRead
import java.time.Instant
import org.json.JSONObject

/**
 * EMP-GPS-002 (B-BR-EMP-495, EDR-321): the driver's read of the tracking policy in force,
 * `POST /v1/orgs/{org_id}/tracking/policy/mine`, on the account cookie plane the app already uses for its context reads.
 * The answer is parsed strictly; an inconsistent policy, revision or notice is refused, never cached.
 */
class EnterpriseTrackingPolicyHttpGateway(private val client: CompanyAccessClient) : EnterpriseTrackingPolicyGateway {
    private val owner = CompanyAccessClient.Owner()

    private suspend fun post(context: EnterpriseContextSummary, path: String): EnterpriseAccessResult<JSONObject> {
        val binding = client.currentBinding() ?: return EnterpriseAccessResult.Err(EnterpriseAccessError.SIGN_IN_REQUIRED)
        if (binding.accountId != context.partition.ownerAccount || binding.origin != context.partition.origin) {
            return EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED)
        }
        val first = client.post(owner, binding, path, JSONObject())
        if (first !is EnterpriseAccessResult.Err || first.error != EnterpriseAccessError.STALE) return first
        when (val opened = client.open(owner)) {
            is EnterpriseAccessResult.Err -> return opened
            is EnterpriseAccessResult.Ok -> if (opened.value != binding) return EnterpriseAccessResult.Err(EnterpriseAccessError.STALE)
        }
        return client.post(owner, binding, path, JSONObject())
    }

    override suspend fun mine(context: EnterpriseContextSummary, readAt: Instant): EnterpriseAccessResult<TrackingPolicyRead> {
        val path = "/v1/orgs/${context.partition.organizationId}/tracking/policy/mine"
        return when (val result = post(context, path)) {
            is EnterpriseAccessResult.Err -> result
            is EnterpriseAccessResult.Ok -> try {
                val root = result.value
                require(root.keys().asSequence().toSet() == setOf("revision", "policy", "notice_version", "notice_in_force_at", "data_mode") &&
                    root.strictString("data_mode") == "SYNTHETIC")
                val revision = root.get("revision").let { require(it is Int || it is Long); (it as Number).toLong() }
                val policy = TrackingPolicy.parse(root.strictString("policy"))
                val notice = if (root.isNull("notice_version")) null else root.strictString("notice_version")
                val inForce = if (root.isNull("notice_in_force_at")) null else root.strictString("notice_in_force_at").let {
                    EnterpriseTime.parseInstant(it).also { at -> require(EnterpriseTime.formatInstant(at) == it) }
                }
                EnterpriseAccessResult.Ok(TrackingPolicyRead(policy, revision, notice, inForce, readAt))
            } catch (_: Exception) { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
        }
    }

    override fun toString(): String = "EnterpriseTrackingPolicyHttpGateway(redacted)"
}
