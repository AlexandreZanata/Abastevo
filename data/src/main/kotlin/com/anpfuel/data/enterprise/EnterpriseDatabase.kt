package com.anpfuel.data.enterprise

import androidx.room.Dao
import androidx.room.Database
import androidx.room.Entity
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.RoomDatabase
import androidx.room.Transaction
import androidx.room.Upsert
import com.anpfuel.application.enterprise.EnterpriseOperationReused

/** Separate schema: never add enterprise rows to AnpFuelDatabase/personal outboxes. */
@Entity(tableName = "enterprise_records", primaryKeys = ["ownerAccount", "origin", "organizationId", "id"])
data class EnterpriseRecordRow(
    val ownerAccount: String,
    val origin: String,
    val organizationId: String,
    val id: String,
    val envelope: ByteArray,
) {
    override fun toString(): String = "EnterpriseRecordRow(redacted)"
}

@Entity(tableName = "enterprise_pending", primaryKeys = ["ownerAccount", "origin", "organizationId", "id"])
data class EnterpriseOperationRow(
    val ownerAccount: String,
    val origin: String,
    val organizationId: String,
    val id: String,
    val targetId: String,
    val envelope: ByteArray,
) {
    override fun toString(): String = "EnterpriseOperationRow(redacted)"
}

@Dao
abstract class EnterpriseDao {
    @Query("SELECT * FROM enterprise_records WHERE ownerAccount=:owner AND origin=:origin AND organizationId=:org AND id=:id LIMIT 1")
    abstract suspend fun record(owner: String, origin: String, org: String, id: String): EnterpriseRecordRow?

    @Upsert
    abstract suspend fun put(record: EnterpriseRecordRow)

    @Query("SELECT * FROM enterprise_pending WHERE ownerAccount=:owner AND origin=:origin AND organizationId=:org AND id=:id LIMIT 1")
    abstract suspend fun operation(owner: String, origin: String, org: String, id: String): EnterpriseOperationRow?

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    abstract suspend fun insertOperation(operation: EnterpriseOperationRow): Long

    @Query("SELECT * FROM enterprise_pending WHERE ownerAccount=:owner AND origin=:origin AND organizationId=:org ORDER BY rowid LIMIT :limit")
    abstract suspend fun pending(owner: String, origin: String, org: String, limit: Int): List<EnterpriseOperationRow>

    @Transaction
    open suspend fun enqueue(row: EnterpriseOperationRow): Boolean {
        if (insertOperation(row) != -1L) return true
        val prior = operation(row.ownerAccount, row.origin, row.organizationId, row.id)
        if (prior == null || prior.targetId != row.targetId || !prior.envelope.contentEquals(row.envelope)) {
            throw EnterpriseOperationReused()
        }
        return false
    }
}

@Database(entities = [EnterpriseRecordRow::class, EnterpriseOperationRow::class], version = 1, exportSchema = true)
abstract class EnterpriseDatabase : RoomDatabase() {
    abstract fun enterpriseDao(): EnterpriseDao
}
