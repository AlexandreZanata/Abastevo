package com.anpfuel.data.repository

import com.anpfuel.application.port.QualityReportLocalErasure
import com.anpfuel.application.portable.OutboxCommand
import com.anpfuel.application.portable.PortableOutbox
import com.anpfuel.data.local.dao.QualityReportOutboxDao
import com.anpfuel.data.local.entity.QualityOwnerGateEntity
import com.anpfuel.data.local.quality.*
import com.anpfuel.data.local.quality.QualityReportSeal
import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.QualityReportGateway
import java.security.MessageDigest
import java.time.Instant
import java.time.ZoneOffset
import java.util.UUID
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** Exactly one frozen confirmed intent per opaque owner/family, fresh proof on dispatch. */
class DurableQualityReports(
    private val remote:QualityReportGateway,
    private val dao:QualityReportOutboxDao,
    private val keys:QualityReportKeys,
    private val schedule:()->Unit,
    private val now:()->Long={System.currentTimeMillis()},
    private val enabled:()->Boolean={true},
    private val markers:QualityErasureMarkers=MemoryQualityErasureMarkers(),
):QualityReportGateway,QualityReportLocalErasure {
    private val erasedFamilies=java.util.Collections.synchronizedMap(object:LinkedHashMap<String,Boolean>() {
        override fun removeEldestEntry(eldest:MutableMap.MutableEntry<String,Boolean>?)=size>64
    })
    override fun currentOwner():String?=remote.currentOwner()?.takeUnless {erasedFamilies.containsKey(it) || markers.blocked(parts(it).second)}
    private fun key(owner:String)=MessageDigest.getInstance("SHA-256").digest(owner.toByteArray()).joinToString(""){"%02x".format(it)}
    private fun parts(owner:String):Pair<String,String> {
        val split=owner.split('|')
        if(split.size!=3 || !split[1].startsWith("account:") || !Regex("family:[0-9a-f]{64}").matches(split[2]))fail(QualityReportFailure.AUTH_REQUIRED)
        return split[0] to split[1]
    }
    private fun requireOwner(owner:String) {if(owner!=currentOwner())fail(QualityReportFailure.OWNER_CHANGED)}
    override suspend fun owner():String=withContext(Dispatchers.IO) {
        repairKeys()
        val owner=remote.currentOwner() ?: remote.owner()
        requireOwner(owner)
        val (origin,local)=parts(owner)
        val tick=now()
        if(tick<0 || tick>Long.MAX_VALUE-QualityReportSeal.RETENTION_MILLIS)fail(QualityReportFailure.UNAVAILABLE)
        if(!dao.activate(QualityOwnerGateEntity(key(owner),local,origin,false,tick,tick+QualityReportSeal.RETENTION_MILLIS),tick))fail(QualityReportFailure.AUTH_REQUIRED)
        requireOwner(owner)
        owner
    }
    override suspend fun pending(owner:String):QualityReportCommand?=withContext(Dispatchers.IO) {
        requireOwner(owner)
        val row=dao.pending(key(owner)) ?: return@withContext null
        val gate=dao.gate(row.ownerKey) ?: return@withContext null
        if(gate.blocked || gate.keyErased || now()<gate.lastSeen)return@withContext null
        val key=keys.getOrCreate(row.ownerKey,gate.ownerLocal) ?: fail(QualityReportFailure.UNAVAILABLE)
        QualityReportSeal.open(row,key,now()).also {requireOwner(owner)}
    }
    override suspend fun list(owner:String,afterId:String?):QualityReportPage {
        requireOwner(owner);return remote.list(owner,afterId).also {requireOwner(owner)}
    }
    override suspend fun get(owner:String,reportId:String):QualityReport {
        requireOwner(owner);return remote.get(owner,reportId).also {requireOwner(owner)}
    }
    override suspend fun execute(owner:String,command:QualityReportCommand):QualityReportReceipt=withContext(Dispatchers.IO) {
        requireOwner(owner)
        if(!enabled() && command.action !in setOf("WITHDRAW","APPEAL"))fail(QualityReportFailure.UNAVAILABLE)
        val tick=now()
        try {command.validate(Instant.ofEpochMilli(tick).atZone(ZoneOffset.UTC).toLocalDate())}catch(_:Exception){fail(QualityReportFailure.INVALID)}
        val gate=dao.gate(key(owner)) ?: fail(QualityReportFailure.AUTH_REQUIRED)
        if(gate.blocked || gate.keyErased || gate.lastSeen>tick || gate.expiresAt<=tick)fail(QualityReportFailure.AUTH_REQUIRED)
        val secret=keys.getOrCreate(gate.ownerKey,gate.ownerLocal) ?: fail(QualityReportFailure.UNAVAILABLE)
        val row=QualityReportSeal.freeze(gate.ownerKey,command,secret,tick,minOf(gate.expiresAt,tick+QualityReportSeal.RETENTION_MILLIS))
        val frozen=dao.freeze(row,tick) ?: fail(QualityReportFailure.CONFLICT)
        requireOwner(owner)
        runCatching(schedule)
        dispatch(owner,frozen.commandId)
    }
    private suspend fun dispatch(owner:String,commandId:String):QualityReportReceipt {
        requireOwner(owner)
        val tick=now()
        val row=dao.pending(key(owner)) ?: fail(QualityReportFailure.NOT_FOUND)
        if(row.commandId!=commandId)fail(QualityReportFailure.CONFLICT)
        val claim=UUID.randomUUID().toString()
        val claimed=dao.claim(row.ownerKey,commandId,claim,tick,minOf(tick+120000,row.expiresAt)) ?: fail(QualityReportFailure.UNAVAILABLE)
        try {
            val secret=keys.getOrCreate(row.ownerKey,parts(owner).second) ?: fail(QualityReportFailure.UNAVAILABLE)
            val command=QualityReportSeal.open(claimed,secret,now()) ?: fail(QualityReportFailure.INVALID)
            requireOwner(owner)
            val receipt=remote.execute(owner,command)
            requireOwner(owner)
            if(!dao.complete(row.ownerKey,commandId,claim,now()))fail(QualityReportFailure.OWNER_CHANGED)
            return receipt
        }catch(e:CancellationException){throw e}
        catch(e:Exception) {
            val error=e as? QualityReportException ?: QualityReportException(QualityReportFailure.UNAVAILABLE)
            val failedAt=now()
            if(error.kind in setOf(QualityReportFailure.INVALID,QualityReportFailure.CONFLICT,QualityReportFailure.NOT_FOUND))dao.refused(row.ownerKey,commandId,claim,failedAt)
            else {
                val policy=PortableOutbox.markFailed(OutboxCommand(commandId,"quality-report","",attempts=claimed.attempts-1),failedAt)
                val hint=error.retryAfterSeconds?.coerceIn(0,PortableOutbox.MAX_DELAY_TICKS/1000)?.times(1000) ?: 0
                val next=if(claimed.attempts>=PortableOutbox.MAX_DISPATCH_ATTEMPTS || error.kind in setOf(QualityReportFailure.AUTH_REQUIRED,QualityReportFailure.OWNER_CHANGED))Long.MAX_VALUE
                    else maxOf(policy.nextEligibleTick,if(failedAt>Long.MAX_VALUE-hint)Long.MAX_VALUE else failedAt+hint)
                dao.failed(row.ownerKey,commandId,claim,failedAt,next)
            }
            throw error
        }
    }
    /** One current owner intent at most. No foreign account enumeration or dispatch. */
    suspend fun runOnce():Boolean=withContext(Dispatchers.IO) {
        val repaired=repairKeys()
        if(!enabled())return@withContext repaired
        val owner=currentOwner() ?: return@withContext repaired
        val row=dao.pending(key(owner)) ?: return@withContext repaired
        if(row.attempts>=PortableOutbox.MAX_DISPATCH_ATTEMPTS || row.retryAt==Long.MAX_VALUE)return@withContext repaired
        if(now()>=row.expiresAt)return@withContext repairKeys()
        try {dispatch(owner,row.commandId);repaired}
        catch(e:CancellationException){throw e}
        catch(e:QualityReportException){e.kind !in setOf(QualityReportFailure.UNAVAILABLE,QualityReportFailure.QUOTA)}
    }
    override suspend fun eraseOwner(ownerScope:String):Boolean=withContext(Dispatchers.IO) {
        remote.currentOwner()?.takeIf {parts(it).second==ownerScope}?.let {erasedFamilies[it]=true}
        val marker=runCatching {markers.block(ownerScope)}.isSuccess
        val cleared=eraseStoredOwner(ownerScope)
        if(cleared && marker)markers.clear(ownerScope)
        runCatching(schedule)
        cleared && marker
    }
    private suspend fun eraseStoredOwner(local:String):Boolean {
        val tick=now().coerceAtLeast(0)
        val owner=remote.currentOwner()?.takeIf {parts(it).second==local}
        val until=if(tick>Long.MAX_VALUE-QualityReportSeal.RETENTION_MILLIS)Long.MAX_VALUE else tick+QualityReportSeal.RETENTION_MILLIS
        val barrier=owner?.let {QualityOwnerGateEntity(key(it),local,parts(it).first,true,tick,until)}
        val blocked=runCatching {dao.erase(local,tick,until,barrier)}.isSuccess
        // Crypto erasure is attempted even while Room is unavailable.
        val deleted=keys.eraseOwner(local)
        if(blocked && deleted)for(ownerKey in dao.ownerKeys(local))dao.keyErased(ownerKey,tick)
        return blocked && deleted
    }
    suspend fun repairKeys():Boolean=withContext(Dispatchers.IO) {
        val tick=now()
        var complete=true
        for(local in markers.pending()) {
            if(eraseStoredOwner(local))markers.clear(local) else complete=false
        }
        for(ownerKey in dao.keysToErase(tick)) {
            val gate=dao.gate(ownerKey) ?: continue
            if(keys.delete(ownerKey,gate.ownerLocal))dao.keyErased(ownerKey,tick) else complete=false
        }
        dao.pruneExpired(tick);dao.pruneGates(tick)
        complete
    }
    private fun fail(kind:QualityReportFailure):Nothing=throw QualityReportException(kind)
}
