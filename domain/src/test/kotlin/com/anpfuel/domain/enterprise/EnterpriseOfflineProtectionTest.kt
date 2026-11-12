package com.anpfuel.domain.enterprise

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseOfflineProtectionTest {
    private val time = EnterpriseDeviceClock("boot-7", 1000, 10000)
    private val lease = EnterpriseOfflineLease("7b0e2a54-0000-4000-8000-000000000001", 10000, time)

    @Test
    fun monotonicDeadlineIncludesExpiryAndCannotBeExtendedByWallRollback() {
        val duration = EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS
        assertEquals(EnterpriseLeaseDecision.ALLOW, EnterpriseOfflineProtection.evaluate(lease, time.copy(elapsedMillis = 1000 + duration - 1)))
        assertEquals(EnterpriseLeaseDecision.ERASE, EnterpriseOfflineProtection.evaluate(lease, time.copy(elapsedMillis = 1000 + duration)))
        assertEquals(EnterpriseLeaseDecision.ERASE, EnterpriseOfflineProtection.evaluate(lease, time.copy(elapsedMillis = 1000 + duration, wallMillis = 1)))
    }

    @Test
    fun rebootOrClockRollbackRequiresContactWithoutDestroyingUnsentWork() {
        assertEquals(EnterpriseLeaseDecision.RECONFIRM, EnterpriseOfflineProtection.evaluate(lease, time.copy(boot = "boot-8")))
        assertEquals(EnterpriseLeaseDecision.RECONFIRM, EnterpriseOfflineProtection.evaluate(lease, time.copy(elapsedMillis = 999)))
        assertEquals(EnterpriseLeaseDecision.RECONFIRM, EnterpriseOfflineProtection.evaluate(lease, time.copy(wallMillis = 9999)))
        assertEquals(EnterpriseLeaseDecision.ALLOW, EnterpriseOfflineProtection.evaluate(lease, time.copy(wallMillis = Long.MAX_VALUE)))
    }

    @Test
    fun wallTimePastValidityAfterRebootErasesButARolledBackClockOnlyReconfirms() {
        val duration = EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS
        val rebooted = time.copy(boot = "boot-8", elapsedMillis = 0)
        assertEquals(EnterpriseLeaseDecision.RECONFIRM, EnterpriseOfflineProtection.evaluate(lease, rebooted.copy(wallMillis = 10000 + duration - 1)))
        assertEquals(EnterpriseLeaseDecision.ERASE, EnterpriseOfflineProtection.evaluate(lease, rebooted.copy(wallMillis = 10000 + duration)))
        assertEquals(EnterpriseLeaseDecision.ERASE, EnterpriseOfflineProtection.evaluate(lease, rebooted.copy(wallMillis = Long.MAX_VALUE)))
        // Setting the wall clock back can never extend validity; it only forces contact.
        assertEquals(EnterpriseLeaseDecision.RECONFIRM, EnterpriseOfflineProtection.evaluate(lease, rebooted.copy(wallMillis = 0)))
        val regressed = time.copy(elapsedMillis = 999)
        assertEquals(EnterpriseLeaseDecision.ERASE, EnterpriseOfflineProtection.evaluate(lease, regressed.copy(wallMillis = 10000 + duration)))
        assertEquals(EnterpriseLeaseDecision.RECONFIRM, EnterpriseOfflineProtection.evaluate(lease, regressed.copy(wallMillis = 10000 + duration - 1)))
    }

    @Test
    fun explicitValidityAndLargeMonotonicValuesDoNotOverflow() {
        val short = lease.copy(validityMillis = 1)
        assertEquals(EnterpriseLeaseDecision.ERASE, EnterpriseOfflineProtection.evaluate(short, time.copy(elapsedMillis = 1001)))
        val large = lease.copy(contactClock = time.copy(elapsedMillis = Long.MAX_VALUE - 2), validityMillis = 2)
        assertEquals(EnterpriseLeaseDecision.ALLOW, EnterpriseOfflineProtection.evaluate(large, large.contactClock.copy(elapsedMillis = Long.MAX_VALUE - 1)))
        assertEquals(EnterpriseLeaseDecision.ERASE, EnterpriseOfflineProtection.evaluate(large, large.contactClock.copy(elapsedMillis = Long.MAX_VALUE)))
    }

    @Test
    fun invalidAnchorsLimitsAndReferencesFailBeforeUse() {
        assertThrows(IllegalArgumentException::class.java) { time.copy(boot = "") }
        assertThrows(IllegalArgumentException::class.java) { time.copy(elapsedMillis = -1) }
        assertThrows(IllegalArgumentException::class.java) { time.copy(wallMillis = -1) }
        assertThrows(IllegalArgumentException::class.java) { lease.copy(sessionFamily = "bad") }
        assertThrows(IllegalArgumentException::class.java) { lease.copy(validityMillis = 0) }
        assertThrows(IllegalArgumentException::class.java) { lease.copy(serverContactMillis = -1) }
        assertThrows(IllegalArgumentException::class.java) { lease.copy(managedUnlockSeconds = 0) }
        assertEquals(30L * 86400000, lease.validityMillis)
        assertEquals(1, lease.copy(managedUnlockSeconds = 1).managedUnlockSeconds)
        assertFalse(lease.toString().contains(lease.sessionFamily))
        assertFalse(time.toString().contains(time.boot))
    }
}
