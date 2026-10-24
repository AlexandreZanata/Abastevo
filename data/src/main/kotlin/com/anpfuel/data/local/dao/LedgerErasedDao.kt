package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.LedgerErasedEntity

/**
 * MEX-15 tombstone memory. Inserts ignore conflicts so the first
 * erasure timestamp wins and repeat deletes stay idempotent.
 */
@Dao
interface LedgerErasedDao {

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun record(erasure: LedgerErasedEntity): Long

    @Query(
        "SELECT COUNT(*) FROM ledger_erased WHERE owner_account = :owner AND origin = :origin " +
            "AND dataset = :dataset AND record_id = :recordId LIMIT 1",
    )
    suspend fun isErased(owner: String, origin: String, dataset: String, recordId: String): Boolean

    @Query("DELETE FROM ledger_erased WHERE owner_account = :owner AND origin = :origin")
    suspend fun clearOwner(owner: String, origin: String)
}
