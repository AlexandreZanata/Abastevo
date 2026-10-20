package com.anpfuel.data.local.quality

import com.anpfuel.data.local.entity.QualityReportOutboxEntity
import com.anpfuel.data.local.media.PhotoBlobSeal
import com.anpfuel.domain.quality.QualityReportCommand
import java.security.MessageDigest
import java.time.Instant
import java.time.ZoneOffset
import javax.crypto.SecretKey
import org.json.JSONArray
import org.json.JSONObject

/** Reuses the established AES/GCM envelope, with a separate quality key alias. */
object QualityReportSeal {
    const val RETENTION_MILLIS=7L*24*60*60*1000
    fun freeze(ownerKey:String,command:QualityReportCommand,key:SecretKey,now:Long,expiresAt:Long):QualityReportOutboxEntity {
        require(ownerKey.matches(Regex("[0-9a-f]{64}")) && now>=0 && expiresAt>now && expiresAt-now<=RETENTION_MILLIS)
        command.validate(Instant.ofEpochMilli(now).atZone(ZoneOffset.UTC).toLocalDate())
        val array=encode(command)
        val payload=JSONObject().put("v",1).put("owner_key",ownerKey).put("command",array).toString().toByteArray()
        val sealed=PhotoBlobSeal.seal(PhotoBlobSeal.Entry(now,payload),key) ?: throw IllegalStateException("quality secure storage unavailable")
        require(sealed.length<=65536)
        return QualityReportOutboxEntity(ownerKey,command.commandId,hash(array.toString()),sealed,now,expiresAt,"QUEUED",0,now,"",0)
    }
    fun open(row:QualityReportOutboxEntity,key:SecretKey,now:Long):QualityReportCommand?=try {
        if(row.sealedCommand.length>65536 || now<row.createdAt || now>=row.expiresAt)null
        else {
            val entry=PhotoBlobSeal.open(row.sealedCommand,key)
            if(entry==null || entry.capturedAtMillis!=row.createdAt)null
            else {
                val doc=JSONObject(entry.bytes.toString(Charsets.UTF_8))
                val fields=doc.getJSONArray("command")
                if(doc.getInt("v")!=1 || doc.getString("owner_key")!=row.ownerKey || fields.length()!=12 || hash(fields.toString())!=row.payloadHash)null
                else decode(fields).takeIf {it.commandId==row.commandId}?.also {it.validate(Instant.ofEpochMilli(now).atZone(ZoneOffset.UTC).toLocalDate())}
            }
        }
    }catch(_:Exception){null}
    private fun encode(c:QualityReportCommand)=JSONArray(listOf(c.commandId,c.reportId,c.action,c.expectedRevision,c.kind,c.stationId,c.fuel,c.occurredOn,c.symptom,c.text,c.officialRevisionId,c.sampleRef))
    private fun decode(a:JSONArray):QualityReportCommand {
        fun text(index:Int)=a.get(index) as String
        return QualityReportCommand(text(0),text(1),text(2),a.get(3) as Int,text(4),text(5),text(6),text(7),text(8),text(9),text(10),text(11))
    }
    private fun hash(text:String)=MessageDigest.getInstance("SHA-256").digest(text.toByteArray()).joinToString(""){"%02x".format(it)}
}
