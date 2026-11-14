package com.anpfuel.domain.enterprise.sync

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock

/** Protected retry metadata is separate from the immutable intent and final receipt. */
class QueueRetryState(val attempts: Long = 0, val clock: EnterpriseDeviceClock? = null, val delayMillis: Long = 0) {
    init { require(attempts >= 0 && delayMillis >= 0 && (clock != null || delayMillis == 0L)) }
    fun isDue(now: EnterpriseDeviceClock): Boolean {
        val captured = clock ?: return true
        if (now.boot != captured.boot || now.elapsedMillis < captured.elapsedMillis) return false
        return now.elapsedMillis - captured.elapsedMillis >= delayMillis
    }
    override fun toString(): String = "QueueRetryState(redacted)"
}

class EnterpriseQueueEntry(val operation: EnterpriseOperation, val retry: QueueRetryState = QueueRetryState(),
    val acknowledgement: OperationAcknowledgement? = null, val dismissedRejected: Boolean = false) {
    init {
        require(acknowledgement == null || acknowledgement.matches(operation) && acknowledgement.isFinal)
        require(!dismissedRejected || acknowledgement?.result == SyncResult.REJECTED)
    }
    override fun toString(): String = "EnterpriseQueueEntry(redacted)"
}

class EnterpriseQueueStats(val queuedCount: Int, val queuedBytes: Long, val legacyCount: Int,
    val totalEntries: Int, val totalBytes: Long) {
    init { require(queuedCount >= 0 && queuedBytes >= 0 && legacyCount >= 0 && totalEntries >= 0 && totalBytes >= 0) }
    val pressure: Boolean get() = totalEntries >= 8000 || totalBytes >= (32L * 1024 * 1024 * 4 + 4) / 5
    override fun toString(): String = "EnterpriseQueueStats(redacted)"
}

class QueueRetryPolicy(val baseMillis: Long = 30_000, val capMillis: Long = 21_600_000) {
    init { require(baseMillis > 0 && capMillis >= baseMillis) }
    fun delayMillis(failureCount: Long, entropy: (Long) -> Long, retryAfterMillis: Long? = null): Long {
        require(failureCount > 0 && (retryAfterMillis == null || retryAfterMillis >= 0))
        var calculated = baseMillis
        var remaining = failureCount - 1
        while (remaining > 0 && calculated < capMillis) {
            calculated = if (calculated > capMillis / 2) capMillis else calculated * 2
            remaining--
        }
        val half = calculated / 2
        val bound = calculated - half + 1
        val sample = entropy(bound)
        require(sample in 0 until bound)
        return maxOf(half + sample, retryAfterMillis ?: 0)
    }
    fun nextAttempts(attempts: Long): Long {
        require(attempts >= 0)
        return if (attempts == Long.MAX_VALUE) attempts else attempts + 1
    }
}

class QueueOrder internal constructor(ordered: List<EnterpriseOperation>, blocked: Set<String>) {
    private val values = ordered.toList()
    private val blockedValues = blocked.toSet()
    val ordered: List<EnterpriseOperation> get() = values.toList()
    /** Includes operations whose local ancestor is part of an unorderable cycle. */
    val cycleBlocked: Set<String> get() = blockedValues.toSet()
}

object EnterpriseQueueOrdering {
    fun order(operations: List<EnterpriseOperation>): QueueOrder {
        require(operations.size <= 10000)
        val byId = linkedMapOf<String, EnterpriseOperation>()
        for (operation in operations) require(byId.put(operation.operationId, operation) == null)
        val degree = linkedMapOf<String, Int>()
        val children = mutableMapOf<String, MutableList<String>>()
        for (operation in operations) {
            val parents = operation.dependsOn.filter { it in byId }
            degree[operation.operationId] = parents.size
            for (parent in parents) children.getOrPut(parent) { mutableListOf() }.add(operation.operationId)
        }
        val ready = java.util.ArrayDeque(degree.filterValues { it == 0 }.keys)
        val ordered = mutableListOf<EnterpriseOperation>()
        while (ready.isNotEmpty()) {
            val id = ready.removeFirst()
            ordered.add(byId.getValue(id))
            for (child in children[id].orEmpty()) {
                val remaining = degree.getValue(child) - 1
                degree[child] = remaining
                if (remaining == 0) ready.add(child)
            }
        }
        return QueueOrder(ordered, degree.filterValues { it != 0 }.keys)
    }
}
