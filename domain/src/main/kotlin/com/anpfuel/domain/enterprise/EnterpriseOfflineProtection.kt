package com.anpfuel.domain.enterprise

/** Device observations are not server time or membership authority. */
data class EnterpriseDeviceClock(val boot: String, val elapsedMillis: Long, val wallMillis: Long) {
    init {
        require(boot.isNotBlank() && boot.length <= 128 && elapsedMillis >= 0 && wallMillis >= 0)
    }
    override fun toString(): String = "EnterpriseDeviceClock(redacted)"
}

data class EnterpriseOfflineLease(
    val sessionFamily: String,
    val serverContactMillis: Long,
    val contactClock: EnterpriseDeviceClock,
    val validityMillis: Long = DEFAULT_VALIDITY_MILLIS,
    val managedUnlockSeconds: Int? = null,
) {
    init {
        EnterprisePartition.requireId(sessionFamily)
        require(serverContactMillis >= 0 && validityMillis > 0)
        require(managedUnlockSeconds == null || managedUnlockSeconds > 0)
    }
    override fun toString(): String = "EnterpriseOfflineLease(redacted)"

    companion object {
        const val DEFAULT_VALIDITY_MILLIS = 30L * 24 * 60 * 60 * 1000
    }
}

enum class EnterpriseLeaseDecision { ALLOW, RECONFIRM, ERASE }

object EnterpriseOfflineProtection {
    fun evaluate(lease: EnterpriseOfflineLease, now: EnterpriseDeviceClock): EnterpriseLeaseDecision {
        val sameBoot = now.boot == lease.contactClock.boot && now.elapsedMillis >= lease.contactClock.elapsedMillis
        if (sameBoot && now.elapsedMillis - lease.contactClock.elapsedMillis >= lease.validityMillis) {
            return EnterpriseLeaseDecision.ERASE
        }
        // Without a comparable monotonic clock, wall time can only shorten validity (BR-PRIV-018):
        // a clock moved forward erases early, a clock set back still requires server contact.
        if (!sameBoot && now.wallMillis - lease.contactClock.wallMillis >= lease.validityMillis) {
            return EnterpriseLeaseDecision.ERASE
        }
        if (!sameBoot || now.wallMillis < lease.contactClock.wallMillis) return EnterpriseLeaseDecision.RECONFIRM
        return EnterpriseLeaseDecision.ALLOW
    }
}
