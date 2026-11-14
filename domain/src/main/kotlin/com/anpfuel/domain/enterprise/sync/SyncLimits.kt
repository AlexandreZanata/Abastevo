package com.anpfuel.domain.enterprise.sync

/** Captured trusted configuration can narrow protocol ceilings; requests carry no limits. */
class SyncLimits private constructor(
    val maxOperations: Int,
    val maxBatchBytes: Int,
    val maxOperationBytes: Int,
    val maxTransportDepth: Int,
) {
    val maxOperationDepth: Int get() = maxTransportDepth - 2
    companion object {
        val DEFAULT: SyncLimits = trusted()
        fun trusted(
            maxOperations: Int = 50,
            maxBatchBytes: Int = 65536,
            maxOperationBytes: Int = 8192,
            maxTransportDepth: Int = 64,
        ): SyncLimits {
            if (maxOperations !in 1..50 || maxBatchBytes !in 1..65536 || maxOperationBytes !in 1..8192 ||
                maxOperationBytes > maxBatchBytes || maxTransportDepth !in 4..64) throw invalid()
            return SyncLimits(maxOperations, maxBatchBytes, maxOperationBytes, maxTransportDepth)
        }
    }
}
