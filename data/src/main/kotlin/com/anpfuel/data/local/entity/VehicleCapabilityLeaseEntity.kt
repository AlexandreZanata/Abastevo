package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity

/**
 * MEX-08 owner/origin capability lease for extra local vehicles.
 *
 * One row per (owner, origin) partition; partitions are never merged.
 * The row records a server-verified `vehicles.multiple` grant with its
 * verification clock and paid-through bound. Offline use is bounded by
 * min(verified + 24h, paid-through); a missing row simply means no
 * extra creation. No secret is stored here.
 */
@Entity(
    tableName = "vehicle_capability_lease",
    primaryKeys = ["owner_account", "origin"],
)
data class VehicleCapabilityLeaseEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "policy_version") val policyVersion: Int,
    @ColumnInfo(name = "capability") val capability: String,
    @ColumnInfo(name = "verified_at_millis") val verifiedAtMillis: Long,
    @ColumnInfo(name = "paid_through_millis") val paidThroughMillis: Long,
    @ColumnInfo(name = "revoked") val revoked: Boolean,
    @ColumnInfo(name = "saved_at_millis") val savedAtMillis: Long,
)
