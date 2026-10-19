package com.anpfuel.data.local.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.Index
import androidx.room.PrimaryKey

@Entity(tableName="quality_owner_gate",indices=[Index(value=["owner_local"])])
data class QualityOwnerGateEntity(
    @PrimaryKey @ColumnInfo(name="owner_key") val ownerKey:String,
    @ColumnInfo(name="owner_local") val ownerLocal:String,
    @ColumnInfo(name="origin") val origin:String,
    @ColumnInfo(name="blocked") val blocked:Boolean,
    @ColumnInfo(name="last_seen") val lastSeen:Long,
    @ColumnInfo(name="expires_at") val expiresAt:Long,
    @ColumnInfo(name="key_erased",defaultValue="0") val keyErased:Boolean=false,
)

/** Ciphertext only; no auth session, proof nonce or raw claim appears in this row. */
@Entity(tableName="quality_report_outbox",indices=[Index(value=["command_id"],unique=true)])
data class QualityReportOutboxEntity(
    @PrimaryKey @ColumnInfo(name="owner_key") val ownerKey:String,
    @ColumnInfo(name="command_id") val commandId:String,
    @ColumnInfo(name="payload_hash") val payloadHash:String,
    @ColumnInfo(name="sealed_command") val sealedCommand:String,
    @ColumnInfo(name="created_at") val createdAt:Long,
    @ColumnInfo(name="expires_at") val expiresAt:Long,
    @ColumnInfo(name="state") val state:String,
    @ColumnInfo(name="attempts") val attempts:Int,
    @ColumnInfo(name="retry_at") val retryAt:Long,
    @ColumnInfo(name="claim_id") val claimId:String,
    @ColumnInfo(name="lease_until") val leaseUntil:Long,
)
