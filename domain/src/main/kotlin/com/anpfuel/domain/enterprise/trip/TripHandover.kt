package com.anpfuel.domain.enterprise.trip

import java.time.Instant

/**
 * One recorded handover (EMP-TRIP-012, B-BR-EMP-394): the recording member, the receiving member and its employee
 * record, the expected revision, the reason and the server instant (never a device claim, BR-TIME-003).
 */
data class TripHandoverInput(
    val recorder: MembershipRef? = null,
    val receiver: MembershipRef? = null,
    val receiverEmployee: EmployeeRef? = null,
    val expected: Long = 0,
    val reason: String? = null,
    val at: Instant? = null,
) {
    override fun toString(): String = "EnterpriseTripHandoverInput(redacted)"
}

/** One accepted handover of a trip; earlier facts keep their actors. */
class TripHandoverFact internal constructor(
    val revision: Long,
    val from: MembershipRef,
    val to: MembershipRef,
    val receiverEmployee: EmployeeRef,
    val recorder: MembershipRef,
    val at: Instant,
    val reason: String,
) {
    override fun toString(): String = "EnterpriseTripHandoverFact(redacted)"
}
