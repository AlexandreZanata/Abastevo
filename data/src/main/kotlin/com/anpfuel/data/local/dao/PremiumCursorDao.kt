package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.PremiumCursorEntity

/**
 * PRM09-T01 owner-scoped opaque cursor store.
 */
@Dao
interface PremiumCursorDao {

    @Query(
        "SELECT * FROM premium_cursor WHERE owner_account = :ownerAccount " +
            "AND origin = :origin AND dataset = :dataset LIMIT 1",
    )
    suspend fun find(
        ownerAccount: String,
        origin: String,
        dataset: String,
    ): PremiumCursorEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(entity: PremiumCursorEntity)

    @Query(
        "DELETE FROM premium_cursor WHERE owner_account = :ownerAccount AND origin = :origin",
    )
    suspend fun clearOwner(ownerAccount: String, origin: String)
}
