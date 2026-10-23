package com.anpfuel.domain.repository

/**
 * MEX-15 local erasure log. Records the app deleted (or acknowledged
 * as deleted server-side) so a later restore of older data never
 * resurrects them as live rows. Append-only per owner; restore consults
 * it before adopting any server row.
 */
interface LedgerErasureLog {

    suspend fun recordErased(accountId: String, origin: String, dataset: String, recordId: String, nowMillis: Long)

    suspend fun isErased(accountId: String, origin: String, dataset: String, recordId: String): Boolean

    suspend fun clearOwner(accountId: String, origin: String)
}
