package com.anpfuel.domain.repository

/** Last server acknowledgement per dataset for honest sync status. */
data class LedgerAck(
    val dataset: String,
    val revision: Long,
    val atMillis: Long,
)

interface LedgerSyncAckStore {

    suspend fun recordAck(accountId: String, origin: String, dataset: String, revision: Long, nowMillis: Long)

    suspend fun lastAck(accountId: String, origin: String, dataset: String): LedgerAck?

    suspend fun clearOwner(accountId: String, origin: String)
}
