package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Transaction
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.data.local.entity.ContributionOutboxEntity

@Dao
interface ContributionOutboxDao {

    @Query("SELECT * FROM contribution_outbox WHERE command_id = :commandId LIMIT 1")
    suspend fun findById(commandId: String): ContributionOutboxEntity?

    @Query("SELECT * FROM contribution_outbox ORDER BY rowid ASC")
    suspend fun listAll(): List<ContributionOutboxEntity>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(entity: ContributionOutboxEntity)

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertReview(rows: List<ContributionOutboxEntity>)

    @Transaction
    suspend fun freezeReview(rows: List<ContributionOutboxEntity>): List<ContributionOutboxEntity> {
        val frozen = rows.map { row ->
            val existing = findById(row.commandId)
            if (existing != null && (existing.payload != row.payload || existing.kind != row.kind)) {
                throw DomainException("confirmed review cannot change")
            }
            existing ?: row
        }
        val missing = rows.filter { findById(it.commandId) == null }
        if (missing.isNotEmpty()) insertReview(missing)
        return frozen
    }

    @Query("""UPDATE contribution_outbox SET state='IN_FLIGHT', nonce=:nonce, next_eligible_tick=:leaseUntil
        WHERE command_id=:commandId AND revision=:revision AND (
        state='QUEUED' OR (state IN ('FAILED','IN_FLIGHT') AND next_eligible_tick<=:nowMillis)
        OR (state='ACKED' AND remote_status IN ('RECEIVED','VALIDATING') AND next_eligible_tick<=:nowMillis))""")
    suspend fun claim(commandId: String, revision: Int, nonce: String, nowMillis: Long, leaseUntil: Long): Int

    @Query("""UPDATE contribution_outbox SET state='ACKED', observation_id=:observationId,
        remote_status=:status, failure_reason=:reason, next_eligible_tick=:nextPoll
        WHERE command_id=:commandId AND revision=:revision AND
        state!='CANCELLED' AND (observation_id IS NULL OR observation_id=:observationId) AND
        (remote_status IS NULL OR remote_status IN ('RECEIVED','VALIDATING') OR remote_status=:status)""")
    suspend fun saveReceipt(commandId: String, revision: Int, observationId: String?, status: String, reason: String?, nextPoll: Long): Int

    @Query("""UPDATE contribution_outbox SET state='FAILED', attempts=:attempts, next_eligible_tick=:nextTick
        WHERE command_id=:commandId AND revision=:revision AND state='IN_FLIGHT' AND nonce=:nonce""")
    suspend fun failClaim(commandId: String, revision: Int, nonce: String, attempts: Int, nextTick: Long): Int

    @Query("UPDATE contribution_outbox SET state='CANCELLED' WHERE command_id=:commandId AND state IN ('QUEUED','FAILED') AND remote_status IS NULL")
    suspend fun cancelPending(commandId: String): Int

    @Query("DELETE FROM contribution_outbox WHERE command_id = :commandId")
    suspend fun deleteById(commandId: String)

    @Query("DELETE FROM contribution_outbox")
    suspend fun clear()
}
