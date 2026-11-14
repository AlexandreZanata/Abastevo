package com.anpfuel.domain.enterprise

/** Exact source-owned custody identity; a rehire never inherits an earlier member's work. */
data class EnterpriseBoundOfflineLease(
    val membershipId: String,
    val enrollmentId: String,
    val enrollmentRevision: Long,
    val time: EnterpriseOfflineLease,
) {
    init {
        EnterprisePartition.requireId(membershipId)
        EnterprisePartition.requireId(enrollmentId)
        require(enrollmentRevision > 0)
    }
    override fun toString(): String = "EnterpriseBoundOfflineLease(redacted)"
}
