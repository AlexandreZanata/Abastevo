package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Transaction
import com.anpfuel.data.local.entity.QualityOwnerGateEntity
import com.anpfuel.data.local.entity.QualityReportOutboxEntity

@Dao
abstract class QualityReportOutboxDao {
    @Query("SELECT * FROM quality_owner_gate WHERE owner_key=:key LIMIT 1")
    abstract suspend fun gate(key:String):QualityOwnerGateEntity?
    @Query("SELECT * FROM quality_report_outbox WHERE owner_key=:key LIMIT 1")
    abstract suspend fun pending(key:String):QualityReportOutboxEntity?
    @Query("SELECT count(*) FROM quality_owner_gate")
    abstract suspend fun gatesCount():Int
    @Query("SELECT count(*) FROM quality_report_outbox")
    abstract suspend fun pendingCount():Int
    @Insert(onConflict=OnConflictStrategy.IGNORE)
    abstract suspend fun insertGate(row:QualityOwnerGateEntity):Long
    @Insert(onConflict=OnConflictStrategy.ABORT)
    abstract suspend fun insertPending(row:QualityReportOutboxEntity)
    @Query("DELETE FROM quality_report_outbox WHERE expires_at<=:now AND lease_until<=:now")
    abstract suspend fun pruneExpired(now:Long):Int
    @Query("DELETE FROM quality_owner_gate WHERE expires_at<=:now AND owner_key NOT IN (SELECT owner_key FROM quality_report_outbox)")
    abstract suspend fun pruneGates(now:Long):Int
    @Query("UPDATE quality_owner_gate SET last_seen=:now WHERE owner_key=:key AND blocked=0 AND last_seen<=:now AND expires_at>:now")
    abstract suspend fun touch(key:String,now:Long):Int
    @Query("UPDATE quality_report_outbox SET state='IN_FLIGHT',attempts=attempts+1,claim_id=:claim,lease_until=:until WHERE owner_key=:key AND command_id=:command AND attempts<25 AND created_at<=:now AND expires_at>:now AND :until<=expires_at AND :until>:now AND ((state='QUEUED' OR state='FAILED') AND retry_at<=:now OR state='IN_FLIGHT' AND lease_until<=:now) AND EXISTS (SELECT 1 FROM quality_owner_gate g WHERE g.owner_key=:key AND g.blocked=0 AND g.last_seen<=:now AND g.expires_at>:now)")
    abstract suspend fun claimRow(key:String,command:String,claim:String,now:Long,until:Long):Int
    @Query("DELETE FROM quality_report_outbox WHERE owner_key=:key AND command_id=:command AND claim_id=:claim AND state='IN_FLIGHT' AND lease_until>:now AND expires_at>:now AND EXISTS (SELECT 1 FROM quality_owner_gate g WHERE g.owner_key=:key AND g.blocked=0 AND g.last_seen<=:now AND g.expires_at>:now)")
    abstract suspend fun acknowledge(key:String,command:String,claim:String,now:Long):Int
    @Query("UPDATE quality_report_outbox SET state='FAILED',retry_at=:retry,claim_id='',lease_until=0 WHERE owner_key=:key AND command_id=:command AND claim_id=:claim AND state='IN_FLIGHT' AND lease_until>:now AND EXISTS (SELECT 1 FROM quality_owner_gate g WHERE g.owner_key=:key AND g.blocked=0 AND g.last_seen<=:now AND g.expires_at>:now)")
    abstract suspend fun failClaim(key:String,command:String,claim:String,now:Long,retry:Long):Int
    @Query("DELETE FROM quality_report_outbox WHERE owner_key=:key AND command_id=:command AND claim_id=:claim AND state='IN_FLIGHT'")
    abstract suspend fun refuseClaim(key:String,command:String,claim:String):Int
    @Query("SELECT owner_key FROM quality_owner_gate WHERE owner_local=:local ORDER BY owner_key LIMIT 64")
    abstract suspend fun ownerKeys(local:String):List<String>
    @Query("UPDATE quality_owner_gate SET blocked=1,last_seen=MAX(last_seen,:now),expires_at=MAX(expires_at,:until) WHERE owner_local=:local")
    abstract suspend fun blockOwner(local:String,now:Long,until:Long):Int
    @Query("DELETE FROM quality_report_outbox WHERE owner_key IN (SELECT owner_key FROM quality_owner_gate WHERE owner_local=:local)")
    abstract suspend fun eraseRows(local:String):Int

    @Transaction
    open suspend fun activate(row:QualityOwnerGateEntity,now:Long):Boolean {
        require(row.ownerKey.matches(Regex("[0-9a-f]{64}")) && row.ownerLocal.length in 1..256 && row.origin.length in 1..256)
        require(!row.blocked && row.lastSeen==now && now>=0 && row.expiresAt>now && row.expiresAt-now<=com.anpfuel.data.local.quality.QualityReportSeal.RETENTION_MILLIS)
        pruneExpired(now);pruneGates(now)
        val prior=gate(row.ownerKey)
        if(prior==null) {
            if(gatesCount()>=64)return false
            insertGate(row)
        } else if(prior.ownerLocal!=row.ownerLocal || prior.origin!=row.origin)return false
        return touch(row.ownerKey,now)==1
    }
    @Transaction
    open suspend fun freeze(row:QualityReportOutboxEntity,now:Long):QualityReportOutboxEntity? {
        require(row.sealedCommand.length in 1..65536 && row.payloadHash.matches(Regex("[0-9a-f]{64}")))
        require(row.state=="QUEUED" && row.attempts==0 && row.claimId.isEmpty() && row.leaseUntil==0L && row.retryAt==now && row.createdAt==now && row.expiresAt>now)
        if(touch(row.ownerKey,now)!=1 || row.expiresAt>(gate(row.ownerKey)?.expiresAt ?: return null))return null
        val prior=pending(row.ownerKey)
        if(prior!=null)return prior.takeIf {it.commandId==row.commandId && it.payloadHash==row.payloadHash && it.expiresAt>now}
        if(pendingCount()>=25)return null
        insertPending(row)
        return row
    }
    @Transaction
    open suspend fun claim(key:String,command:String,claim:String,now:Long,until:Long):QualityReportOutboxEntity? {
        require(com.anpfuel.domain.quality.qualityReportId(claim))
        if(touch(key,now)!=1 || claimRow(key,command,claim,now,until)!=1)return null
        return pending(key)
    }
    @Transaction
    open suspend fun complete(key:String,command:String,claim:String,now:Long):Boolean =
        touch(key,now)==1 && acknowledge(key,command,claim,now)==1

    @Transaction
    open suspend fun failed(key:String,command:String,claim:String,now:Long,retry:Long):Boolean =
        touch(key,now)==1 && failClaim(key,command,claim,now,retry)==1

    @Transaction
    open suspend fun refused(key:String,command:String,claim:String,now:Long):Boolean =
        touch(key,now)==1 && refuseClaim(key,command,claim)==1

    @Transaction
    open suspend fun erase(local:String,now:Long,until:Long,barrier:QualityOwnerGateEntity?=null):Int {
        if(barrier!=null && gate(barrier.ownerKey)==null && gatesCount()<64) {
            require(barrier.ownerLocal==local && barrier.blocked)
            insertGate(barrier)
        }
        blockOwner(local,now,until)
        return eraseRows(local)
    }
}
