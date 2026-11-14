package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.domain.enterprise.EnterprisePartition
import org.json.JSONObject

/** Minimal account-context display metadata only. Web permission hints never become Android authority. */
class EnterpriseContextHttpGateway(private val client: CompanyAccessClient) : EnterpriseContextGateway {
    private val owner = CompanyAccessClient.Owner()
    override fun currentBinding(): EnterpriseAccessBinding? = client.currentBinding()
    override suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding> = client.open(owner)
    override fun invalidate() = client.invalidate(owner)

    override suspend fun list(binding: EnterpriseAccessBinding, afterOrganizationId: String?): EnterpriseAccessResult<EnterpriseContextPage> {
        val body = JSONObject().put("limit", 100)
        if (afterOrganizationId != null) {
            try { EnterprisePartition.requireId(afterOrganizationId) }
            catch (_: Exception) { return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
            body.put("after_org_id", afterOrganizationId)
        }
        return decode(client.post(owner, binding, PREFIX + "list", body), binding) { root ->
            val values = root.getJSONArray("organizations")
            require(values.length() <= 100)
            val contexts = (0 until values.length()).map { summary(values.getJSONObject(it), binding) }
            require(contexts.zipWithNext().all { (a, b) -> a.partition.organizationId < b.partition.organizationId })
            require(afterOrganizationId == null || contexts.all { it.partition.organizationId > afterOrganizationId })
            val next = root.strictString("next_after_org_id").ifEmpty { null }
            require(next == null || afterOrganizationId == null || next > afterOrganizationId)
            EnterpriseContextPage(contexts, next)
        }
    }

    override suspend fun read(binding: EnterpriseAccessBinding, context: EnterpriseContextSummary): EnterpriseAccessResult<EnterpriseContextSummary> {
        if (context.partition.ownerAccount != binding.accountId || context.partition.origin != binding.origin) {
            return EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED)
        }
        return decode(client.post(owner, binding, PREFIX + "read", JSONObject().put("organization_id", context.partition.organizationId)
            .put("membership_id", context.membershipId)), binding) { root ->
            summary(root.getJSONObject("organization"), binding).also {
                if (it.partition != context.partition || it.membershipId != context.membershipId) throw BindingDenied()
            }
        }
    }

    private fun summary(value: JSONObject, binding: EnterpriseAccessBinding): EnterpriseContextSummary = EnterpriseContextSummary(
        EnterprisePartition.of(binding.accountId, binding.origin, value.strictString("id")),
        value.strictString("membership_id"), value.strictString("name"),
    )

    private class BindingDenied : IllegalArgumentException("Enterprise context response unavailable")

    private fun <T> decode(result: EnterpriseAccessResult<JSONObject>, binding: EnterpriseAccessBinding, parser: (JSONObject) -> T): EnterpriseAccessResult<T> = when (result) {
        is EnterpriseAccessResult.Err -> result
        is EnterpriseAccessResult.Ok -> try {
            if (result.value.strictString("account_id") != binding.accountId) throw BindingDenied()
            EnterpriseAccessResult.Ok(parser(result.value))
        } catch (_: BindingDenied) { EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED) }
        catch (_: Exception) { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
    }

    override fun toString(): String = "EnterpriseContextHttpGateway(redacted)"
    companion object { private const val PREFIX = "/v1/org-access/contexts/" }
}
