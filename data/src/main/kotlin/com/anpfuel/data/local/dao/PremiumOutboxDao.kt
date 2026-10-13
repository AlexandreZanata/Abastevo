package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.PremiumOutboxEntity

/**
 * PRM09-T01 owner-scoped durable queue.
 *
 * All reads filter by `owner_account` + `origin`; there is no unscoped
 * list-all. Dispatch uses bounded keyset pages ordered by rowid so workers
 * stay off the main thread and never load full payloads to pick one row.
 */
@Dao
interface PremiumOutboxDao {

    @Query(
        "SELECT * FROM premium_outbox WHERE command_id = :commandId " +
            "AND owner_account = :ownerAccount AND origin = :origin LIMIT 1",
    )
    suspend fun findById(
        commandId: String,
        ownerAccount: String,
        origin: String,
    ): PremiumOutboxEntity?

    @Query(
        "SELECT * FROM premium_outbox WHERE owner_account = :ownerAccount AND origin = :origin " +
            "ORDER BY rowid ASC LIMIT :limit",
    )
    suspend fun listOwned(
        ownerAccount: String,
        origin: String,
        limit: Int,
    ): List<PremiumOutboxEntity>

    @Query(
        "SELECT * FROM premium_outbox WHERE owner_account = :ownerAccount AND origin = :origin " +
            "AND (state = 'QUEUED' OR (state = 'IN_FLIGHT' AND next_eligible_tick <= :nowMillis)) " +
            "ORDER BY rowid ASC LIMIT :limit",
    )
    suspend fun listDispatchable(
        ownerAccount: String,
        origin: String,
        limit: Int,
        nowMillis: Long,
    ): List<PremiumOutboxEntity>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(entity: PremiumOutboxEntity)

    @Query(
        "UPDATE premium_outbox SET state = 'IN_FLIGHT', nonce = :nonce, " +
            "next_eligible_tick = :leaseUntil WHERE command_id = :commandId " +
            "AND owner_account = :ownerAccount AND origin = :origin AND state = 'QUEUED'",
    )
    suspend fun claim(
        commandId: String,
        ownerAccount: String,
        origin: String,
        nonce: String,
        leaseUntil: Long,
    ): Int

    @Query(
        "UPDATE premium_outbox SET state = 'FAILED', attempts = :attempts, " +
            "next_eligible_tick = :nextTick WHERE command_id = :commandId " +
            "AND owner_account = :ownerAccount AND origin = :origin " +
            "AND state = 'IN_FLIGHT' AND nonce = :nonce",
    )
    suspend fun failClaim(
        commandId: String,
        ownerAccount: String,
        origin: String,
        nonce: String,
        attempts: Int,
        nextTick: Long,
    ): Int

    @Query(
        "DELETE FROM premium_outbox WHERE command_id = :commandId " +
            "AND owner_account = :ownerAccount AND origin = :origin",
    )
    suspend fun deleteById(commandId: String, ownerAccount: String, origin: String)

    @Query(
        "DELETE FROM premium_outbox WHERE owner_account = :ownerAccount AND origin = :origin",
    )
    suspend fun clearOwner(ownerAccount: String, origin: String)
}
