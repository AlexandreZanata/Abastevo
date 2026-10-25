package com.anpfuel.app.ui.premium

import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusSnapshot
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Short-lived, account-scoped display cache for the last server-verified
 * Premium status. It never creates a local entitlement and is cleared on
 * logout, account switch, verification failure or an explicit store return.
 */
@Singleton
class PremiumStatusCache @Inject constructor() {
    private var entry: Entry? = null

    @Synchronized
    fun get(accountId: String, nowMillis: Long): PremiumStatus? {
        val current = entry ?: return null
        if (current.snapshot.accountId != accountId) return null
        val age = nowMillis - current.storedAtMillis
        if (age !in 0..TTL_MILLIS) return null
        return current.snapshot.status
    }

    @Synchronized
    fun put(snapshot: PremiumStatusSnapshot, nowMillis: Long) {
        entry = Entry(snapshot, nowMillis)
    }

    @Synchronized
    fun clear() {
        entry = null
    }

    private data class Entry(
        val snapshot: PremiumStatusSnapshot,
        val storedAtMillis: Long,
    )

    companion object {
        const val TTL_MILLIS = 60_000L
    }
}
