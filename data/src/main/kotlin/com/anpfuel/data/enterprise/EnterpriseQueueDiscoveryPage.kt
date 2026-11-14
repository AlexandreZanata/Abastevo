package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseEnrollmentBinding

/** Scoped source selectors only; no ciphertext payload or cursor reaches system work input. */
class EnterpriseQueueDiscoveryPage(
    enrollments: List<EnterpriseEnrollmentBinding>,
    val nextAfterOpaque: String?,
    val unavailable: Int,
    /** Partitions waiting for the person's explicit reconfirmation: attention, never a worker failure. */
    val contactRequired: Int = 0,
) {
    private val values = enrollments.toList()
    val enrollments: List<EnterpriseEnrollmentBinding> get() = values.toList()
    init { require(unavailable >= 0 && contactRequired >= 0) }
    override fun toString(): String = "EnterpriseQueueDiscoveryPage(redacted)"
}
