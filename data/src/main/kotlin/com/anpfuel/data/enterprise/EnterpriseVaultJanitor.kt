package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseLeaseDecision
import com.anpfuel.domain.enterprise.EnterpriseOfflineProtection

/** Counts contain no identity, path, operation or credential. */
data class EnterpriseVaultMaintenanceResult(val checked: Int, val erased: Int, val failed: Int, val hasMore: Boolean) {
    val complete: Boolean get() = failed == 0 && !hasMore
}

class EnterpriseVaultSweep internal constructor() {
    internal var after: String? = null
    internal var finished: Boolean = false
    override fun toString(): String = "EnterpriseVaultSweep(redacted)"
}

/** Opaque discovery is not authority: each expiry decision requires the exact trusted-head descriptor. */
class EnterpriseVaultJanitor(
    private val files: EnterpriseVaultFiles,
    private val keys: EnterpriseVaultKeys,
    private val fence: EnterpriseSessionFence,
    private val erasure: EnterpriseVaultErasure,
    private val clock: () -> EnterpriseDeviceClock,
) {
    fun newSweep(): EnterpriseVaultSweep = EnterpriseVaultSweep()
    fun runOnce(limit: Int = 50): EnterpriseVaultMaintenanceResult = runSlice(newSweep(), limit)
    fun runSlice(sweep: EnterpriseVaultSweep, limit: Int = 50): EnterpriseVaultMaintenanceResult = synchronized(sweep) {
        require(limit in 1..50)
        if (sweep.finished) return@synchronized EnterpriseVaultMaintenanceResult(0, 0, 0, false)
        var failures = 0
        val tokens = mutableSetOf<String>()
        try { tokens += files.allTokens() } catch (_: Exception) { failures++ }
        try { tokens += keys.aliases(EnterpriseVaultGenerationNames.PREFIX).mapNotNull(EnterpriseVaultGenerationNames::token) }
        catch (_: Exception) { failures++ }
        try { tokens += keys.aliases("abastevo_enterprise_v1_").map { it.removePrefix("abastevo_enterprise_v1_") }
            .filter { Regex("[0-9a-f]{64}-[0-9a-f]{64}").matches(it) } } catch (_: Exception) { failures++ }
        try { tokens += erasure.knownIntents() } catch (_: Exception) { failures++ }
        val sorted = tokens.sorted()
        val candidates = sorted.filter { sweep.after == null || it > sweep.after!! }.take(limit)
        var erased = 0
        candidates.forEach { token ->
            try {
                fence.access {
                    files.locked(token.substringBefore('-')) {
                        if (token.length == 64) {
                            if (!erasure.settleAccountLocked(token)) failures++
                        } else if (erasure.deniedLocked(token)) {
                            if (erasure.retryLocked(token)) erased++ else failures++
                        } else {
                            val generations = EnterpriseVaultGenerations(files, keys)
                            val descriptor = generations.descriptor(token)
                            when (descriptor?.let { EnterpriseOfflineProtection.evaluate(it.lease.time, clock()) }) {
                                EnterpriseLeaseDecision.ERASE -> if (erasure.eraseLocked(token)) erased++ else failures++
                                EnterpriseLeaseDecision.RECONFIRM -> {
                                    erasure.requireContactLocked(token)
                                    if (!generations.cleanup(token)) failures++
                                }
                                EnterpriseLeaseDecision.ALLOW, null -> if (!generations.cleanup(token)) failures++
                            }
                        }
                    }
                }
            } catch (_: Exception) { failures++ }
        }
        val more = candidates.isNotEmpty() && sorted.any { it > candidates.last() }
        sweep.after = if (more) candidates.last() else null
        sweep.finished = !more
        EnterpriseVaultMaintenanceResult(candidates.size, erased, failures, more)
    }
}
