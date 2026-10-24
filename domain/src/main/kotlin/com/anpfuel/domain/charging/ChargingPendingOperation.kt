package com.anpfuel.domain.charging

import java.util.UUID

enum class PendingChargingKind { SUBMIT, CORRECT, WITHDRAW }

/**
 * EE-22 durable community operation. The client ID is the idempotency key:
 * a lost reply or process restart re-sends the exact same ID, never a
 * duplicate. Votes/reports are immediate and are not queued offline.
 */
data class PendingChargingOperation(
    val operationId: String,
    val kind: PendingChargingKind,
    val targetId: String?,
    val expectedRevision: Long?,
    val draft: ChargingObservationDraft?,
    val createdAtMillis: Long,
    val lastError: String? = null,
) {
    init {
        require(UUID.fromString(operationId).toString() == operationId)
        require(targetId == null || UUID.fromString(targetId).toString() == targetId)
        require(expectedRevision == null || expectedRevision > 0)
        if (kind == PendingChargingKind.SUBMIT) require(draft != null && targetId == null)
        if (kind == PendingChargingKind.CORRECT) require(draft != null && targetId != null && expectedRevision != null)
        if (kind == PendingChargingKind.WITHDRAW) require(targetId != null && expectedRevision != null)
    }
}

interface ChargingPendingStore {
    fun load(accountId: String, origin: String): List<PendingChargingOperation>
    fun save(accountId: String, origin: String, operations: List<PendingChargingOperation>)
    fun clear(accountId: String, origin: String)
}
