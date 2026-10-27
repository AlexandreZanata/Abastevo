package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.DriverLocalDao
import com.anpfuel.data.local.entity.DriverLocalCommandEntity
import javax.inject.Inject
import javax.inject.Singleton

/**
 * DRV-14 bounded local outbox. Claims are owner/origin scoped and lease-bound;
 * an expired lease can be reclaimed, a foreign owner can never claim. Retry
 * delays grow exponentially up to [maxDelayMillis], then park as CONFLICT.
 */
@Singleton
class DriverOutbox @Inject constructor(
    private val dao: DriverLocalDao,
) {
    suspend fun claim(
        owner: String, origin: String, operationId: String,
        nowMillis: Long, leaseMillis: Long = 60_000,
    ): DriverLocalCommandEntity? {
        if (leaseMillis <= 0 || nowMillis < 0) return null
        val row = dao.findClaimable(owner, origin, operationId, nowMillis) ?: return null
        return if (dao.claim(owner, origin, operationId, nowMillis, nowMillis + leaseMillis) == 1) {
            row.copy(leaseUntilMillis = nowMillis + leaseMillis, updatedAtMillis = nowMillis)
        } else {
            null
        }
    }

    suspend fun synced(owner: String, origin: String, operationId: String, nowMillis: Long): Int =
        dao.settle(owner, origin, operationId, STATE_SYNCED, null, nowMillis)

    suspend fun conflict(owner: String, origin: String, operationId: String, error: String, nowMillis: Long): Int =
        dao.settle(owner, origin, operationId, STATE_CONFLICT, error.take(280), nowMillis)

    suspend fun retry(
        owner: String, origin: String, operationId: String, error: String,
        nowMillis: Long, baseDelayMillis: Long = 5_000, maxDelayMillis: Long = 15 * 60_000,
        maxAttempts: Int = 6,
    ): Int {
        val row = dao.findCommand(owner, origin, operationId) ?: return 0
        val attempts = row.attempts + 1
        if (attempts >= maxAttempts) {
            return dao.settle(owner, origin, operationId, STATE_CONFLICT, "retry-limit", nowMillis)
        }
        val delay = (baseDelayMillis shl (attempts - 1).coerceAtMost(20)).coerceAtMost(maxDelayMillis)
        return dao.retry(owner, origin, operationId, STATE_PENDING, attempts, nowMillis + delay, error.take(280), nowMillis)
    }

    companion object {
        const val STATE_PENDING = "PENDING"
        const val STATE_SYNCED = "SYNCED"
        const val STATE_CONFLICT = "CONFLICT"
    }
}
