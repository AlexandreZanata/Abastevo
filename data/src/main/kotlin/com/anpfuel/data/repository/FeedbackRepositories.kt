package com.anpfuel.data.repository

import com.anpfuel.domain.repository.FeedbackCacheRepository
import com.anpfuel.domain.repository.FeedbackOutboxPort
import com.anpfuel.domain.repository.FeedbackPage
import com.anpfuel.domain.repository.PendingFeedbackOp
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P17-T02 in-memory feedback adapters.
 *
 * The cache keeps the last first-page per station/product for the
 * offline fallback; it is never presented as fresh. The outbox is
 * bounded ([MAX_PENDING]): when full, the oldest op drops so new
 * writes never block — drops stay visible via [dropped].
 */
@Singleton
class FeedbackCacheMemory @Inject constructor() : FeedbackCacheRepository {

    private val lock = Any()
    private val pages = HashMap<String, FeedbackPage>()

    override fun loadComments(stationId: String, product: String): FeedbackPage? =
        synchronized(lock) { pages[keyOf(stationId, product)] }

    override fun saveComments(stationId: String, product: String, page: FeedbackPage) {
        synchronized(lock) { pages[keyOf(stationId, product)] = page }
    }

    private fun keyOf(stationId: String, product: String): String = "$stationId|$product"
}

/** Bounded in-memory outbox for transport-failed feedback writes. */
@Singleton
class FeedbackOutboxMemory @Inject constructor() : FeedbackOutboxPort {

    private val lock = Any()
    private val ops = ArrayDeque<PendingFeedbackOp>()
    private var droppedCount = 0L

    override fun enqueue(op: PendingFeedbackOp) {
        synchronized(lock) {
            if (ops.size >= MAX_PENDING) {
                ops.removeFirst()
                droppedCount++
            }
            ops.addLast(op)
        }
    }

    override fun pending(): List<PendingFeedbackOp> =
        synchronized(lock) { ops.toList() }

    /** Lifetime count of ops dropped by the bound (stays visible). */
    fun dropped(): Long = synchronized(lock) { droppedCount }

    companion object {
        const val MAX_PENDING = 100
    }
}
