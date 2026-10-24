package com.anpfuel.domain.repository

/** One server-side ledger row for restore preview and conflict rebase. */
data class LedgerServerRecord(
    val dataset: String,
    val id: String,
    val vehicleId: String,
    val occurredAt: String,
    val revision: Long,
    val voided: Boolean,
    val payloadJson: String,
)

data class LedgerRecordsPage(
    val items: List<LedgerServerRecord>,
    val nextCursor: String?,
)

/**
 * MEX-15 server reads for conflict resolution and restore preview.
 * Read-only, bounded pages, cancellable; the executor never invents
 * revisions and never writes through this port.
 */
interface LedgerRecordsGateway {

    suspend fun list(
        familyId: String,
        accessToken: String,
        dataset: String,
        vehicleId: String?,
        limit: Int,
        cursor: String?,
    ): LedgerRecordsPage
}
