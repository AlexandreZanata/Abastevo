package com.anpfuel.domain.profile

/**
 * P32-T04 — Offline/restart recovery for profile flows.
 *
 * No cached authority is accepted across restarts: recovery revalidates
 * grant and declaration freshness before any replay.
 */
enum class OfflineRecovery {
    Idle,
    Requeue,
    DropExpired,
    BlockedRevoked,
}

object ProfileOfflineRule {

    fun recover(
        hasPending: Boolean,
        grantRevoked: Boolean,
        declarationExpired: Boolean,
    ): OfflineRecovery {
        if (!hasPending) return OfflineRecovery.Idle
        if (grantRevoked) return OfflineRecovery.BlockedRevoked
        if (declarationExpired) return OfflineRecovery.DropExpired
        return OfflineRecovery.Requeue
    }
}
