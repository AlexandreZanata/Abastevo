package com.anpfuel.domain.enterprise

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseBoundOfflineLeaseTest {
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val b = "7b0e2a54-0000-4000-8000-000000000002"
    private val time = EnterpriseOfflineLease(a, 1, EnterpriseDeviceClock("boot", 0, 0))
    @Test fun exactMembershipEnrollmentAndRevisionAreRequiredAndRedacted() {
        val lease = EnterpriseBoundOfflineLease(a, b, Long.MAX_VALUE, time)
        assertEquals(a, lease.membershipId); assertEquals(b, lease.enrollmentId)
        assertEquals(Long.MAX_VALUE, lease.enrollmentRevision); assertEquals(time, lease.time)
        assertFalse(lease.toString().contains(a)); assertFalse(lease.toString().contains(b))
        for (invalid in listOf("", "7B0E2A54-0000-4000-8000-000000000001", "00000000-0000-0000-0000-000000000000", a + " ")) {
            assertThrows(IllegalArgumentException::class.java) { EnterpriseBoundOfflineLease(invalid, b, 1, time) }
            assertThrows(IllegalArgumentException::class.java) { EnterpriseBoundOfflineLease(a, invalid, 1, time) }
        }
        for (invalid in listOf(0L, -1L, Long.MIN_VALUE)) {
            assertThrows(IllegalArgumentException::class.java) { EnterpriseBoundOfflineLease(a, b, invalid, time) }
        }
    }
}
