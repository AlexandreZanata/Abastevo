package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.Index

@Entity(
    tableName = "driver_shift",
    primaryKeys = ["owner_account", "origin", "shift_id"],
    indices = [Index(value = ["owner_account", "origin", "state"], name = "index_driver_shift_owner_state")],
)
data class DriverShiftEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "shift_id") val shiftId: String,
    @ColumnInfo(name = "vehicle_id") val vehicleId: String,
    @ColumnInfo(name = "timezone") val timezone: String,
    @ColumnInfo(name = "state") val state: String,
    @ColumnInfo(name = "version") val version: Long,
    @ColumnInfo(name = "last_boot_id") val lastBootId: String?,
    @ColumnInfo(name = "last_elapsed_millis") val lastElapsedMillis: Long?,
    @ColumnInfo(name = "last_utc_millis") val lastUtcMillis: Long?,
    @ColumnInfo(name = "active_millis") val activeMillis: Long,
    @ColumnInfo(name = "manually_reviewed_millis") val manuallyReviewedMillis: Long,
    @ColumnInfo(name = "start_odometer_metres") val startOdometerMetres: Long? = null,
    @ColumnInfo(name = "end_odometer_metres") val endOdometerMetres: Long? = null,
    @ColumnInfo(name = "created_at_millis") val createdAtMillis: Long,
    @ColumnInfo(name = "updated_at_millis") val updatedAtMillis: Long,
)

@Entity(
    tableName = "driver_shift_gap",
    primaryKeys = ["owner_account", "origin", "shift_id", "gap_id"],
    indices = [Index(value = ["owner_account", "origin", "shift_id"], name = "index_driver_shift_gap_owner_shift")],
)
data class DriverShiftGapEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "shift_id") val shiftId: String,
    @ColumnInfo(name = "gap_id") val gapId: String,
    @ColumnInfo(name = "reason") val reason: String,
    @ColumnInfo(name = "previous_boot_id") val previousBootId: String,
    @ColumnInfo(name = "previous_elapsed_millis") val previousElapsedMillis: Long,
    @ColumnInfo(name = "previous_utc_millis") val previousUtcMillis: Long,
    @ColumnInfo(name = "next_boot_id") val nextBootId: String,
    @ColumnInfo(name = "next_elapsed_millis") val nextElapsedMillis: Long,
    @ColumnInfo(name = "next_utc_millis") val nextUtcMillis: Long,
    @ColumnInfo(name = "reviewed") val reviewed: Boolean,
    @ColumnInfo(name = "reviewed_work_millis") val reviewedWorkMillis: Long?,
    @ColumnInfo(name = "created_at_millis") val createdAtMillis: Long,
)

@Entity(
    tableName = "driver_earning",
    primaryKeys = ["owner_account", "origin", "record_id"],
    indices = [Index(value = ["owner_account", "origin", "vehicle_id", "from_millis"], name = "index_driver_earning_owner_window")],
)
data class DriverEarningEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "record_id") val recordId: String,
    @ColumnInfo(name = "vehicle_id") val vehicleId: String,
    @ColumnInfo(name = "platform") val platform: String,
    @ColumnInfo(name = "coverage") val coverage: String,
    @ColumnInfo(name = "currency") val currency: String,
    @ColumnInfo(name = "from_millis") val fromMillis: Long,
    @ColumnInfo(name = "to_millis") val toMillis: Long,
    @ColumnInfo(name = "timezone") val timezone: String,
    @ColumnInfo(name = "basis") val basis: String,
    @ColumnInfo(name = "gross_centavos") val grossCentavos: Long?,
    @ColumnInfo(name = "fees_centavos") val feesCentavos: Long?,
    @ColumnInfo(name = "tips_centavos") val tipsCentavos: Long?,
    @ColumnInfo(name = "bonus_centavos") val bonusCentavos: Long?,
    @ColumnInfo(name = "refund_centavos") val refundCentavos: Long?,
    @ColumnInfo(name = "net_centavos") val netCentavos: Long?,
    @ColumnInfo(name = "revision") val revision: Long,
    @ColumnInfo(name = "content_sha256") val contentSha256: String,
    @ColumnInfo(name = "updated_at_millis") val updatedAtMillis: Long,
)

@Entity(
    tableName = "driver_earning_revision",
    primaryKeys = ["owner_account", "origin", "record_id", "revision"],
    indices = [Index(value = ["owner_account", "origin", "record_id"], name = "index_driver_earning_revision_owner_record")],
)
data class DriverEarningRevisionEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "record_id") val recordId: String,
    @ColumnInfo(name = "revision") val revision: Long,
    @ColumnInfo(name = "body_json") val bodyJson: String,
    @ColumnInfo(name = "content_sha256") val contentSha256: String,
    @ColumnInfo(name = "created_at_millis") val createdAtMillis: Long,
)

@Entity(
    tableName = "driver_distance",
    primaryKeys = ["owner_account", "origin", "observation_id"],
    indices = [Index(value = ["owner_account", "origin", "vehicle_id", "from_millis"], name = "index_driver_distance_owner_window")],
)
data class DriverDistanceEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "observation_id") val observationId: String,
    @ColumnInfo(name = "vehicle_id") val vehicleId: String,
    @ColumnInfo(name = "segment") val segment: String,
    @ColumnInfo(name = "basis") val basis: String,
    @ColumnInfo(name = "from_millis") val fromMillis: Long,
    @ColumnInfo(name = "to_millis") val toMillis: Long,
    @ColumnInfo(name = "timezone") val timezone: String,
    @ColumnInfo(name = "metres") val metres: Long?,
    @ColumnInfo(name = "complete") val complete: Boolean,
    @ColumnInfo(name = "has_gap") val hasGap: Boolean,
    @ColumnInfo(name = "quality") val quality: String,
    @ColumnInfo(name = "quality_version") val qualityVersion: Int?,
    @ColumnInfo(name = "quality_note") val qualityNote: String?,
    @ColumnInfo(name = "start_odometer_metres") val startOdometerMetres: Long?,
    @ColumnInfo(name = "end_odometer_metres") val endOdometerMetres: Long?,
    @ColumnInfo(name = "content_sha256") val contentSha256: String,
    @ColumnInfo(name = "updated_at_millis") val updatedAtMillis: Long,
)

@Entity(
    tableName = "driver_local_command",
    primaryKeys = ["owner_account", "origin", "operation_id"],
    indices = [Index(value = ["owner_account", "origin", "state", "next_eligible_millis"], name = "index_driver_command_owner_state")],
)
data class DriverLocalCommandEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "operation_id") val operationId: String,
    @ColumnInfo(name = "kind") val kind: String,
    @ColumnInfo(name = "body_sha256") val bodySha256: String,
    @ColumnInfo(name = "expected_revision") val expectedRevision: Long,
    @ColumnInfo(name = "state") val state: String,
    @ColumnInfo(name = "attempts") val attempts: Int,
    @ColumnInfo(name = "next_eligible_millis") val nextEligibleMillis: Long,
    @ColumnInfo(name = "last_error") val lastError: String?,
    @ColumnInfo(name = "lease_until_millis") val leaseUntilMillis: Long = 0,
    @ColumnInfo(name = "updated_at_millis") val updatedAtMillis: Long,
)

@Entity(
    tableName = "driver_draft",
    primaryKeys = ["owner_account", "origin", "draft_id"],
    indices = [Index(value = ["owner_account", "origin", "updated_at_millis"], name = "index_driver_draft_owner_updated")],
)
data class DriverDraftEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "draft_id") val draftId: String,
    @ColumnInfo(name = "kind") val kind: String,
    @ColumnInfo(name = "record_id") val recordId: String,
    @ColumnInfo(name = "body_json") val bodyJson: String,
    @ColumnInfo(name = "distance_metres") val distanceMetres: Long?,
    @ColumnInfo(name = "amount_centavos") val amountCentavos: Long?,
    @ColumnInfo(name = "duration_seconds") val durationSeconds: Long?,
    @ColumnInfo(name = "unit_version") val unitVersion: String,
    @ColumnInfo(name = "updated_at_millis") val updatedAtMillis: Long,
)

@Entity(
    tableName = "driver_payout",
    primaryKeys = ["owner_account", "origin", "payout_id"],
    indices = [Index(value = ["owner_account", "origin", "vehicle_id", "occurred_at_millis"], name = "index_driver_payout_owner")],
)
data class DriverPayoutEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "payout_id") val payoutId: String,
    @ColumnInfo(name = "vehicle_id") val vehicleId: String,
    @ColumnInfo(name = "occurred_at_millis") val occurredAtMillis: Long,
    @ColumnInfo(name = "amount_centavos") val amountCentavos: Long,
    @ColumnInfo(name = "kind") val kind: String,
    @ColumnInfo(name = "source") val source: String,
    @ColumnInfo(name = "note") val note: String?,
    @ColumnInfo(name = "created_at_millis") val createdAtMillis: Long,
)

@Entity(
    tableName = "driver_cost_setting",
    primaryKeys = ["owner_account", "origin", "vehicle_id"],
)
data class DriverCostSettingEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "vehicle_id") val vehicleId: String,
    @ColumnInfo(name = "ownership") val ownership: String,
    @ColumnInfo(name = "monthly_amount_centavos") val monthlyAmountCentavos: Long?,
    @ColumnInfo(name = "personal_fraction_percent") val personalFractionPercent: Int,
    @ColumnInfo(name = "effective_from") val effectiveFrom: String,
    @ColumnInfo(name = "updated_at_millis") val updatedAtMillis: Long,
)

@Entity(
    tableName = "driver_gps_observation",
    primaryKeys = ["owner_account", "origin", "observation_id"],
    indices = [Index(value = ["owner_account", "origin", "shift_id"], name = "index_driver_gps_owner_shift")],
)
data class DriverGpsObservationEntity(
    @ColumnInfo(name = "owner_account") val ownerAccount: String,
    @ColumnInfo(name = "origin") val origin: String,
    @ColumnInfo(name = "observation_id") val observationId: String,
    @ColumnInfo(name = "shift_id") val shiftId: String,
    @ColumnInfo(name = "from_at_millis") val fromAtMillis: Long,
    @ColumnInfo(name = "to_at_millis") val toAtMillis: Long,
    @ColumnInfo(name = "metres") val metres: Long?,
    @ColumnInfo(name = "complete") val complete: Boolean,
    @ColumnInfo(name = "has_gap") val hasGap: Boolean,
    @ColumnInfo(name = "created_at_millis") val createdAtMillis: Long,
)
