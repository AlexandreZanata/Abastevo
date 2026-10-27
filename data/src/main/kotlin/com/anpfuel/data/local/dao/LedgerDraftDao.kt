package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.LedgerDraftEntity

/**
 * MEX-11 owner/origin ledger drafts. All reads filter by owner + origin;
 * there is no unscoped list. Re-saving the same draft id replaces the
 * row, so process death between edits cannot duplicate entries.
 */
@Dao
interface LedgerDraftDao {

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(entity: LedgerDraftEntity)

    @Query(
        "SELECT * FROM ledger_draft WHERE owner_account = :owner AND origin = :origin " +
            "AND dataset = :dataset AND record_id = :recordId LIMIT 1",
    )
    suspend fun findByRecord(owner: String, origin: String, dataset: String, recordId: String): LedgerDraftEntity?

    @Query(
        "SELECT * FROM ledger_draft WHERE owner_account = :owner AND origin = :origin " +
            "ORDER BY updated_at_millis ASC",
    )
    suspend fun listOwned(owner: String, origin: String): List<LedgerDraftEntity>

    @Query(
        "UPDATE ledger_draft SET body_json = :replacementBody, updated_at_millis = :nowMillis " +
            "WHERE owner_account = :owner AND origin = :origin AND dataset = :dataset " +
            "AND record_id = :recordId AND expected_revision = :expectedRevision AND body_json = :expectedBody AND draft_id = :draftId",
    )
    suspend fun compareAndSetBody(
        owner: String, origin: String, dataset: String, recordId: String,
        expectedRevision: Long, expectedBody: String, replacementBody: String, nowMillis: Long, draftId: String,
    ): Int

    @Query(
        "DELETE FROM ledger_draft WHERE owner_account = :owner AND origin = :origin " +
            "AND dataset = :dataset AND record_id = :recordId",
    )
    suspend fun deleteByRecord(owner: String, origin: String, dataset: String, recordId: String)

    @Query("DELETE FROM ledger_draft WHERE owner_account = :owner AND origin = :origin")
    suspend fun clearOwner(owner: String, origin: String)
}
