package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.ContributionOutboxEntity

@Dao
interface ContributionOutboxDao {

    @Query("SELECT * FROM contribution_outbox WHERE command_id = :commandId LIMIT 1")
    suspend fun findById(commandId: String): ContributionOutboxEntity?

    @Query("SELECT * FROM contribution_outbox ORDER BY rowid ASC")
    suspend fun listAll(): List<ContributionOutboxEntity>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(entity: ContributionOutboxEntity)

    @Query("DELETE FROM contribution_outbox WHERE command_id = :commandId")
    suspend fun deleteById(commandId: String)

    @Query("DELETE FROM contribution_outbox")
    suspend fun clear()
}
