package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.domain.enterprise.EnterprisePartition
import java.time.Instant
import org.json.JSONObject

class EnterpriseInvitationHttpGateway(private val client: CompanyAccessClient) : EnterpriseInvitationGateway {
    override fun currentBinding(): EnterpriseAccessBinding? = client.currentBinding()
    override suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding> = client.open()
    override fun invalidate() = client.invalidate()

    override suspend fun preview(binding: EnterpriseAccessBinding, code: String): EnterpriseAccessResult<EnterpriseInvitationPreview> =
        decode(client.post(binding, PREFIX + "preview", JSONObject().put("code", code))) { root ->
            val notice = root.getJSONObject("notice")
            require(EnterpriseInvitationContract.knownNotice(notice.strictString("version"), notice.strictString("text")))
            val organization = root.getJSONObject("organization")
            val raw = root.getJSONArray("grants")
            require(raw.length() in 1..10)
            val grants = (0 until raw.length()).map { index -> raw.getJSONObject(index).let {
                require(it.keys().asSequence().toSet() == setOf("role", "scope"))
                EnterpriseInvitationGrant(it.strictString("role"), it.strictString("scope"))
            } }
            EnterpriseInvitationPreview(organization.strictString("name"), organization.strictBoolean("legal_entity_verified"), grants,
                root.strictBoolean("confirmation_required"), root.strictString("consent_digest"), notice.strictString("version"))
        }

    override suspend fun accept(binding: EnterpriseAccessBinding, operationId: String, code: String, consentDigest: String): EnterpriseAccessResult<EnterpriseInvitationAcknowledgement> =
        acknowledgement(binding, "accept", JSONObject().put("schema_version", 1).put("operation_id", operationId).put("code", code)
            .put("consent", true).put("consent_digest", consentDigest))

    override suspend fun decline(binding: EnterpriseAccessBinding, operationId: String, code: String): EnterpriseAccessResult<EnterpriseInvitationAcknowledgement> =
        acknowledgement(binding, "decline", JSONObject().put("schema_version", 1).put("operation_id", operationId).put("code", code))

    private suspend fun acknowledgement(binding: EnterpriseAccessBinding, action: String, body: JSONObject): EnterpriseAccessResult<EnterpriseInvitationAcknowledgement> =
        decode(client.post(binding, PREFIX + action, body)) { root ->
            val ack = EnterpriseInvitationAcknowledgement(root.strictString("organization_id"), root.strictString("invitation_id"),
                root.strictString("membership_id"), root.strictString("state"), root.strictString("membership_state"), root.strictBoolean("replayed"))
            val locator = EnterpriseInvitationContract.normalizeCode(body.strictString("code")).split('.')
            require(locator.size == 3 && locator[0] == "AIN1" && EnterprisePartition.requireId(locator[1]) == ack.invitationId)
            require(ack.state == if (action == "accept") "ACCEPTED" else "DECLINED")
            if (ack.state == "ACCEPTED") {
                val accepted = Instant.parse(root.strictString("accepted_at"))
                require(Instant.parse(root.strictString("confirmation_deadline")) == accepted.plusSeconds(168 * 60 * 60))
            }
            ack
        }

    private fun <T> decode(result: EnterpriseAccessResult<JSONObject>, parser: (JSONObject) -> T): EnterpriseAccessResult<T> = when (result) {
        is EnterpriseAccessResult.Err -> result
        is EnterpriseAccessResult.Ok -> try {
            require(result.value.strictString("data_mode") == "SYNTHETIC")
            EnterpriseAccessResult.Ok(parser(result.value))
        } catch (_: Exception) { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
    }

    override fun toString(): String = "EnterpriseInvitationHttpGateway(redacted)"
    companion object { private const val PREFIX = "/v1/org-access/invitations/" }
}
