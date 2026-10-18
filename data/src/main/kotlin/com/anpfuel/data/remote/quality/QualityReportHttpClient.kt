package com.anpfuel.data.remote.quality

import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.remote.PhotoOperationRefused
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.ContributionRateLimited
import com.anpfuel.domain.repository.QualityReportGateway
import java.time.Clock
import java.time.LocalDate
import java.util.UUID
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject

/** Ephemeral session and fresh exact-body proof on every attempt, stable domain command ID. */
class QualityReportHttpClient internal constructor(
    private val resolveOwner:suspend ()->String,
    private val scope:()->String?,
    private val request:suspend (String,String,JSONObject,String)->JSONObject,
    private val clock:Clock=Clock.systemUTC(),
):QualityReportGateway {
    override fun currentOwner()=scope()
    override suspend fun owner():String=safe {resolveOwner().also {if(it!=scope())fail(QualityReportFailure.OWNER_CHANGED)}}

    override suspend fun execute(owner:String,command:QualityReportCommand):QualityReportReceipt {
        try {command.validate(LocalDate.now(clock))}catch(_:IllegalArgumentException){fail(QualityReportFailure.INVALID)}
        val doc=JSONObject().put("command_id",command.commandId).put("report_id",command.reportId)
            .put("action",command.action).put("expected_revision",command.expectedRevision)
        mapOf("kind" to command.kind,"station_id" to command.stationId,"fuel" to command.fuel,"occurred_on" to command.occurredOn,
            "symptom" to command.symptom,"text" to command.text,"official_revision_id" to command.officialRevisionId,"sample_ref" to command.sampleRef)
            .filterValues {it.isNotEmpty()}.forEach {(key,value)->doc.put(key,value)}
        return safe {
            val row=post("commands",command.commandId,doc,owner)
            val revision=row.positiveRevision()
            if(row.id("report_id")!=command.reportId || revision!=command.expectedRevision+1)fail(QualityReportFailure.UNAVAILABLE)
            QualityReportReceipt(command.reportId,revision,row.text("status",64))
        }
    }
    override suspend fun get(owner:String,reportId:String):QualityReport {
        if(!qualityReportId(reportId))fail(QualityReportFailure.INVALID)
        return safe {parse(post("get",UUID.randomUUID().toString(),JSONObject().put("report_id",reportId),owner)).also {if(it.id!=reportId)fail(QualityReportFailure.UNAVAILABLE)}}
    }
    override suspend fun list(owner:String,afterId:String?):QualityReportPage {
        if(afterId!=null && !qualityReportId(afterId))fail(QualityReportFailure.INVALID)
        return safe {
            val row=post("list",UUID.randomUUID().toString(),JSONObject().apply {afterId?.let {put("after_id",it)}},owner)
            // Go encodes a nil result as null. It is a legitimate empty owner list.
            val rows=if(row.isNull("items") && row.has("items"))null else row.optJSONArray("items") ?: fail(QualityReportFailure.UNAVAILABLE)
            if(rows!=null && rows.length()>50)fail(QualityReportFailure.UNAVAILABLE)
            val items=if(rows==null)emptyList() else (0 until rows.length()).map {parse(rows.optJSONObject(it) ?: fail(QualityReportFailure.UNAVAILABLE))}
            if(items.map {it.id}.distinct().size!=items.size || items.zipWithNext().any {(a,b)->a.id>=b.id} || (afterId!=null && items.any {it.id<=afterId}))fail(QualityReportFailure.UNAVAILABLE)
            val next=row.optional("next_after_id",36)?.also {if(!qualityReportId(it) || items.size!=50 || it!=items.last().id || it==afterId)fail(QualityReportFailure.UNAVAILABLE)}
            QualityReportPage(items,next)
        }
    }
    private suspend fun post(route:String,key:String,body:JSONObject,owner:String):JSONObject {
        if(owner!=scope())fail(QualityReportFailure.OWNER_CHANGED)
        val row=request("/v1/quality/reports/$route",key,body,owner)
        if(owner!=scope())fail(QualityReportFailure.OWNER_CHANGED)
        return row
    }
    private fun parse(row:JSONObject):QualityReport {
        val day=row.text("occurred_on",10)
        if(runCatching {LocalDate.parse(day).toString()!=day}.getOrDefault(true))fail(QualityReportFailure.UNAVAILABLE)
        val official=row.optional("official_revision_id",36).orEmpty()
        val sample=row.optional("sample_ref",128).orEmpty()
        if((official.isEmpty()!=sample.isEmpty()) || (official.isNotEmpty() && !qualityReportId(official)))fail(QualityReportFailure.UNAVAILABLE)
        val fuel=row.text("fuel",64)
        if(fuel !in QUALITY_FUELS)fail(QualityReportFailure.UNAVAILABLE)
        return QualityReport(row.id("id"),row.positiveRevision(),row.text("kind",64),row.id("station_id"),fuel,day,row.text("symptom",64),row.text("text",2000),official,sample,row.text("status",64),row.text("evidence_level",64),row.optional("review_reason",128).orEmpty())
    }
    private fun JSONObject.text(key:String,max:Int):String=(opt(key) as? String)?.takeIf {it.length<=max} ?: fail(QualityReportFailure.UNAVAILABLE)
    private fun JSONObject.optional(key:String,max:Int):String?=if(!has(key)||isNull(key))null else text(key,max)
    private fun JSONObject.id(key:String):String=text(key,36).also {if(!qualityReportId(it))fail(QualityReportFailure.UNAVAILABLE)}
    private fun JSONObject.positiveRevision():Int {
        val value=opt("revision")
        return (value as? Int)?.takeIf {it>0} ?: fail(QualityReportFailure.UNAVAILABLE)
    }
    private suspend fun <T> safe(block:suspend ()->T):T=try {block()}
        catch(e:CancellationException){throw e}
        catch(e:QualityReportException){throw e}
        catch(_:ContributionRateLimited){fail(QualityReportFailure.QUOTA)}
        catch(e:PhotoOperationRefused){fail(when(e.status){401,403->QualityReportFailure.AUTH_REQUIRED;409->QualityReportFailure.CONFLICT;404->QualityReportFailure.NOT_FOUND;429->QualityReportFailure.QUOTA;else->QualityReportFailure.UNAVAILABLE})}
        catch(_:Exception){fail(QualityReportFailure.UNAVAILABLE)}
    private fun fail(kind:QualityReportFailure):Nothing=throw QualityReportException(kind)

    companion object {
        fun signed(proof:PhotoProofTransport,auth:AuthFlow):QualityReportHttpClient {
            fun scope():String?=proof.ownerScope()?.takeIf {it.startsWith("account:")}?.let {proof.origin+"|"+it}
            suspend fun session(expected:String?):PortableAuth.Session=withContext(Dispatchers.IO) {
                val live=(auth.refreshSession() as? AuthApiResult.Ok)?.value ?: throw QualityReportException(QualityReportFailure.AUTH_REQUIRED)
                if(!PortableAuth.isAccessLive(live,System.currentTimeMillis()/1000))throw QualityReportException(QualityReportFailure.AUTH_REQUIRED)
                val actual=scope() ?: throw QualityReportException(QualityReportFailure.AUTH_REQUIRED)
                val local=proof.ownerScope().orEmpty()
                if(!local.startsWith("account:${live.accountId}:") || (expected!=null && actual!=expected))throw QualityReportException(QualityReportFailure.OWNER_CHANGED)
                live
            }
            return QualityReportHttpClient(
                resolveOwner={
                    proof.localScope()
                    session(null)
                    scope() ?: throw QualityReportException(QualityReportFailure.AUTH_REQUIRED)
                },
                scope=::scope,
                request={path,key,body,expected ->
                    val live=session(expected)
                    val local=proof.ownerScope() ?: throw QualityReportException(QualityReportFailure.OWNER_CHANGED)
                    val doc=JSONObject(body.toString()).put("family_id",live.familyId).put("access_token",live.accessToken)
                    proof.post(path,key,doc,local)
                },
            )
        }
    }
}
