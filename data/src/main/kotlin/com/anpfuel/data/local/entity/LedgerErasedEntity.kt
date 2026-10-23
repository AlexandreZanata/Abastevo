package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity

/**
 * MEX-15 local erasure log: records the app deleted (or acknowledged as
 * deleted) so a later restore of older data can never resurrect them as
 * live rows. Append-only per owner; restore consults before adopting.
 */
@Entity(
    tableName = "ledger_erased",
    primaryKeys = ["owner_account", "origin", "dataset", "record_id"],
)
data class LedgerErasedEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "dataset") val dataset: String,
    @ColumnInfo(name = "record_id") val recordId: String,
    @ColumnInfo(name = "erased_at_millis") val erasedAtMillis: Long,
)
