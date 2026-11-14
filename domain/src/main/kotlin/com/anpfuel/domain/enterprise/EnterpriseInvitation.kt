package com.anpfuel.domain.enterprise

/** Reviewed invitation contract only; this is never membership or offline authority. */
object EnterpriseInvitationContract {
    const val NOTICE_VERSION = "enterprise-invitation-v1"
    const val NOTICE_TEXT = "The organization sees your employee record, accepted account alias and actions within this organization. It cannot see your personal credentials, personal vehicles, private history or other organizations."
    private val digest = Regex("[0-9a-f]{64}")
    internal fun validDigest(value: String): Boolean = digest.matches(value)
    fun knownNotice(version: String, text: String): Boolean = version == NOTICE_VERSION && text == NOTICE_TEXT

    /** Only the locator's optional UUID hyphens are normalized; the secret is opaque. */
    fun normalizeCode(raw: String): String {
        val value = raw.trim()
        val parts = value.split('.')
        if (parts.size != 3 || parts[0] != "AIN1" || !Regex("[0-9a-f]{32}").matches(parts[1])) return value
        val id = parts[1]
        return "AIN1.${id.substring(0, 8)}-${id.substring(8, 12)}-${id.substring(12, 16)}-${id.substring(16, 20)}-${id.substring(20)}.${parts[2]}"
    }
}

data class EnterpriseInvitationGrant(val role: String, val scope: String) {
    init {
        require(role in setOf("OWNER", "ADMIN", "FLEET_MANAGER", "SUPERVISOR", "FINANCE", "APPROVER", "DRIVER", "OPERATIONS_STAFF", "AUDITOR", "DATA_MANAGER") &&
            scope in setOf("ORG", "SELF") && (role != "OWNER" || scope == "ORG")) { "Invalid enterprise invitation grant" }
    }
    override fun toString(): String = "EnterpriseInvitationGrant(redacted)"
}

class EnterpriseInvitationPreview(
    val organizationName: String,
    val legalEntityVerified: Boolean,
    grants: List<EnterpriseInvitationGrant>,
    val confirmationRequired: Boolean,
    val consentDigest: String,
    val noticeVersion: String = EnterpriseInvitationContract.NOTICE_VERSION,
) {
    val grants: List<EnterpriseInvitationGrant> = grants.toList()
    init {
        require(organizationName.isNotBlank() && organizationName.length <= 200 && organizationName.none { it.isISOControl() } &&
            this.grants.size in 1..10 && this.grants.distinct().size == this.grants.size &&
            EnterpriseInvitationContract.validDigest(consentDigest) && noticeVersion == EnterpriseInvitationContract.NOTICE_VERSION) { "Invalid enterprise invitation preview" }
    }
    override fun toString(): String = "EnterpriseInvitationPreview(redacted)"
}

class EnterpriseInvitationAcknowledgement(
    val organizationId: String,
    val invitationId: String,
    val membershipId: String,
    val state: String,
    val membershipState: String,
    val replayed: Boolean,
) {
    init {
        listOf(organizationId, invitationId, membershipId).forEach(EnterprisePartition::requireId)
        require((state == "ACCEPTED" && membershipState in setOf("ACTIVE", "INVITED")) ||
            (state == "DECLINED" && membershipState == "ENDED")) { "Invalid enterprise invitation acknowledgement" }
    }
    val active: Boolean get() = state == "ACCEPTED" && membershipState == "ACTIVE"
    val awaitingConfirmation: Boolean get() = state == "ACCEPTED" && membershipState == "INVITED"
    override fun toString(): String = "EnterpriseInvitationAcknowledgement(redacted)"
}
