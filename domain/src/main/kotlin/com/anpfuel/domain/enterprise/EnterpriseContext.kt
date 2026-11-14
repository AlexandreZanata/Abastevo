package com.anpfuel.domain.enterprise

/** Account-bound selector metadata. It contains no credentials, permissions, or offline grant. */
data class EnterpriseContextSummary(
    val partition: EnterprisePartition,
    val membershipId: String,
    val organizationName: String,
) {
    init {
        EnterprisePartition.requireId(membershipId)
        require(organizationName.isNotBlank() && organizationName.codePointCount(0, organizationName.length) <= 120 &&
            organizationName.none { it.isISOControl() }) { "Invalid enterprise context metadata" }
    }
    override fun toString(): String = "EnterpriseContextSummary(redacted)"
}

/** One bounded scan page. An empty eligible page can still have another scanned reference. */
class EnterpriseContextPage(contexts: List<EnterpriseContextSummary>, val nextAfterOrganizationId: String? = null) {
    val contexts: List<EnterpriseContextSummary> = contexts.toList()
    init {
        val first = this.contexts.firstOrNull()?.partition
        require(this.contexts.size <= 100 && this.contexts.map { it.partition.organizationId }.distinct().size == this.contexts.size &&
            this.contexts.all { first != null && it.partition.ownerAccount == first.ownerAccount && it.partition.origin == first.origin }) { "Invalid enterprise context page" }
        if (nextAfterOrganizationId != null) {
            EnterprisePartition.requireId(nextAfterOrganizationId)
            require(this.contexts.all { it.partition.organizationId <= nextAfterOrganizationId }) { "Invalid enterprise context cursor" }
        }
    }
    override fun toString(): String = "EnterpriseContextPage(redacted)"
}
