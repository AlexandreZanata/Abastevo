package com.anpfuel.data.remote.quality

import com.anpfuel.domain.quality.*
import com.anpfuel.data.local.quality.QualityPublicCache
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import com.anpfuel.domain.repository.QualityGateway
import java.io.IOException
import java.time.Instant
import java.time.LocalDate
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.suspendCancellableCoroutine
import okhttp3.*
import okhttp3.HttpUrl.Companion.toHttpUrl
import org.json.JSONArray
import org.json.JSONObject

/** Bounded, anonymous and origin-fixed official reads. No raw server errors escape. */
class QualityHttpClient(http: OkHttpClient, baseUrl: String, private val cache:QualityPublicCache?=null) : QualityGateway {
    private val origin = baseUrl.toHttpUrl()
    private val client = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).callTimeout(20, TimeUnit.SECONDS).build()

    override suspend fun summary(stationId: String, fuel: String, revisionId: String?): QualitySummary {
        val selection = QualitySelection(stationId, fuel, revisionId)
        val response = read("summary", query(selection))
        return response.validated {
            val root=response.body
            if (root.text("station_id") != stationId) invalid()
            val lease = root.lease(revisionId)
            val cells = root.array("fuel_cells", 32).map { cell ->
                val product = cell.text("fuel_product")
                if (product != fuel) invalid()
                QualityAssayCell(product, cell.text("conformity"), cell.count("count"), cell.day("last_observed"))
        }
        val derived = root.optJSONObject("sample_dimensions")
        val dimensions = derived?.array("items", 8)?.map { cell ->
            if (cell.text("fuel_product") != fuel || !cell.has("score") || !cell.isNull("score")) invalid()
            val samples=cell.count("samples")
            val conforming=cell.count("conforming")
            val nonconforming=cell.count("nonconforming")
            val unresolved=cell.count("unresolved")
            if (conforming>samples || nonconforming>samples-conforming || unresolved!=samples-conforming-nonconforming) invalid()
            val visits=cell.count("visits")
            if (visits>samples) invalid()
            QualityDimensions(fuel,samples,visits,conforming,nonconforming,unresolved,
                cell.text("oldest_observed_on"),cell.text("newest_observed_on"),cell.text("score_state"))
        }.orEmpty()
        val available=derived?.bool("available") ?: false
        if (available != dimensions.isNotEmpty() || (!lease.available && (cells.isNotEmpty() || available))) invalid()
        val result=QualitySummary(stationId,lease,cells,dimensions,available,derived?.text("methodology").orEmpty(),root.text("generated_at"))
        result
        }
    }

    override suspend fun assays(stationId: String, fuel: String, revisionId: String, cursor: String?): QualityPage<QualityAssay> {
        val response=read("assays",query(QualitySelection(stationId,fuel,revisionId))+mapOf("limit" to "50","cursor" to cursor))
        return response.validated {
            val root=response.body
            if (root.text("station_id")!=stationId) invalid()
            val lease=root.lease(revisionId)
            val items=root.array("items",50).map(::assay)
            if (items.any { it.fuel!=fuel } || items.map {it.id}.distinct().size!=items.size) invalid()
            val result=QualityPage(items,root.cursor(cursor),lease)
            result
        }
    }

    override suspend fun revisions(cursor: String?): QualityPage<QualityRevision> {
        val response=read("revisions",mapOf("stream" to "pmqc-assay-v1","limit" to "20","cursor" to cursor))
        return response.validated {
            val root=response.body
            if(root.text("source")!="pmqc" || root.text("stream")!="pmqc-assay-v1") invalid()
            val items=root.array("items",20).map { row ->
                QualityRevision(row.uuid("revision_id"),row.text("edition"),row.text("revision_status"),row.text("provenance_state"),row.bool("is_current_stream"))
        }
        if(items.map {it.id}.distinct().size!=items.size) invalid()
        val result=QualityPage(items,root.cursor(cursor))
        result
        }
    }

    override suspend fun sample(sampleRef: String, revisionId: String): QualitySample {
        require(sampleRef.length in 1..128)
        val response=read("samples",mapOf("revision_id" to revisionId),sampleRef)
        return response.validated {
            val root=response.body
            if(root.text("sample_ref")!=sampleRef) invalid()
            val lease=root.lease(revisionId)
            val items=root.array("assays",1000).map(::assay)
            if(!lease.available || items.isEmpty() || items.any {it.sampleRef!=sampleRef}) invalid()
            val result=QualitySample(sampleRef,items,lease)
            result
        }
    }

    override suspend fun provenance(revisionId: String): QualityProvenance {
        val response=read("revisions",mapOf("stream" to "pmqc-assay-v1"),revisionId)
        return response.validated {
            val root=response.body
            val lease=root.lease(revisionId)
            val resources=root.array("resources",32).map { row ->
                val url=row.text("url")
                val parsed=try {url.toHttpUrl()}catch (_:Exception){invalid()}
                if(parsed.scheme!="https" || parsed.host!="www.gov.br" || parsed.port!=443 || parsed.username.isNotEmpty() || parsed.password.isNotEmpty() || parsed.query!=null || parsed.fragment!=null || !parsed.encodedPath.startsWith("/anp/")) invalid()
                val hash=row.text("sha256")
                if(!Regex("[0-9a-f]{64}").matches(hash)) invalid()
                QualityResource(url,hash,row.optionalText("member_name").orEmpty())
        }
        if(lease.available && resources.isEmpty())invalid()
        val result=QualityProvenance(lease,resources)
        result
        }
    }

    private fun assay(row:JSONObject):QualityAssay=QualityAssay(row.uuid("id"),row.text("sample_ref"),row.text("assay_ref"),row.day("observed_on"),row.text("fuel_product"),row.text("result_text"),row.text("unit"),row.text("conformity"),row.optionalText("source_observed_at"))

    private fun query(selection:QualitySelection)=mapOf("station_id" to selection.stationId,"fuel" to selection.fuel,"revision_id" to selection.revisionId)

    private suspend fun <T> PublicRead.validated(parse:()->T):T {
        val result=try {parse()}catch(_:Exception){throw QualityReadUnavailable()}
        accept()
        return result
    }

    private data class PublicRead(val body:JSONObject,val accept:suspend ()->Unit)
    private class TransportFailure:IOException()
    private class RevokedFailure(val disabled:Boolean):IOException()
    private suspend fun read(route:String, params:Map<String,String?>, detail:String?=null):PublicRead {
        val url=origin.newBuilder().addPathSegments("v1/quality/$route").apply {
            detail?.let(::addPathSegment)
            params.forEach {(key,value)->value?.let {addQueryParameter(key,it)}}
        }.build()
        val cached=if(cache==null)null else withContext(Dispatchers.IO){runCatching {cache.load(url.toString())}.getOrNull()}
        var etag:String?=null
        var cacheable=true
        val body=try { suspendCancellableCoroutine<JSONObject> { continuation ->
            val request=Request.Builder().url(url).header("Accept","application/json").header("Cache-Control","no-cache")
            cached?.etag?.let {request.header("If-None-Match",it)}
            val call=client.newCall(request.get().build())
            continuation.invokeOnCancellation {call.cancel()}
            call.enqueue(object:Callback {
                override fun onFailure(call:Call,e:IOException) {
                    if(continuation.isActive) continuation.resumeWithException(TransportFailure())
                }
                override fun onResponse(call:Call,response:Response) {
                    response.use {
                        if(!continuation.isActive) return
                        val body=try {
                            cacheable=!response.cacheControl.noStore
                            if(!cacheable)runCatching {cache?.remove(url.toString())}
                            etag=if(response.code==304)response.header("ETag") ?: cached?.etag else response.header("ETag")
                            if(response.code==304 && cached!=null) {
                                continuation.resume(JSONObject(cached.body))
                                return
                            }
                            if(response.code>=500)throw TransportFailure()
                            if(!response.isSuccessful) throw RevokedFailure(response.code==404)
                            val source=response.body?.source() ?: invalid()
                            if(source.request(1024L*1024L+1)) invalid()
                            JSONObject(source.readUtf8())
                        } catch(e:Exception) {
                            if(continuation.isActive) continuation.resumeWithException(when(e){is QualityReadUnavailable->e;is RevokedFailure->e;is IOException->TransportFailure();else->QualityReadUnavailable()})
                            return
                        }
                        continuation.resume(body)
                    }
                }
            })
        } } catch(e:CancellationException){throw e}
        catch(e:RevokedFailure) {
            withContext(Dispatchers.IO){runCatching {cache?.clearOrigin(origin.resolve("/")!!.toString().trimEnd('/'))}}
            throw QualityReadUnavailable(e.disabled)
        } catch(_:TransportFailure) {
            val fallback=withContext(Dispatchers.IO){runCatching {cache?.load(url.toString())}.getOrNull()} ?: throw QualityReadUnavailable()
            val stale=try {JSONObject(fallback.body)}catch(_:Exception){throw QualityReadUnavailable()}
            stale.optJSONObject("lease")?.put("stale",true)
            return PublicRead(stale,{})
        }
        return PublicRead(body,{withContext(Dispatchers.IO){runCatching {if(cacheable)cache?.save(url.toString(),body.toString(),etag) else cache?.remove(url.toString())}};Unit})
    }

    private fun JSONObject.lease(expected:String?):QualityLease {
        val row=opt("lease") as? JSONObject ?: invalid()
        if(row.text("source")!="pmqc" || row.text("stream")!="pmqc-assay-v1") invalid()
        val revision=row.optionalText("revision_id")
        if(expected!=null && expected!=revision) invalid()
        if(revision!=null) row.uuid("revision_id")
        val available=row.bool("available")
        val published=row.optionalText("published_at")
        if(available && (revision==null || published==null || row.text("revision_status") !in setOf("published","superseded"))) invalid()
        published?.let {try {Instant.parse(it)}catch (_:Exception){invalid()}}
        return QualityLease("pmqc","pmqc-assay-v1",row.text("edition"),revision,available,row.bool("stale"),published,row.text("revision_status"),row.bool("is_current_stream"),row.text("coverage_note"))
    }

    private fun JSONObject.text(key:String):String=(opt(key) as? String)?.takeIf {it.length<=8192} ?: invalid()
    private fun JSONObject.optionalText(key:String):String?=if(!has(key)||isNull(key))null else text(key)
    private fun JSONObject.bool(key:String):Boolean=opt(key) as? Boolean ?: invalid()
    private fun JSONObject.uuid(key:String):String=text(key).also {if(!Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}").matches(it))invalid()}
    private fun JSONObject.day(key:String):String=text(key).also {try {if(LocalDate.parse(it).toString()!=it)invalid()}catch (_:Exception){invalid()}}
    private fun JSONObject.count(key:String):Long {
        val raw=opt(key)
        if(raw !is Int && raw !is Long) invalid()
        return (raw as Number).toLong().also {if(it<0)invalid()}
    }
    private fun JSONObject.cursor(previous:String?):String?=optionalText("next_cursor")?.also {if(it.isBlank()||it.length>8192||it==previous)invalid()}
    private fun JSONObject.array(key:String,limit:Int):List<JSONObject> {
        val rows=opt(key) as? JSONArray ?: invalid()
        if(rows.length()>limit)invalid()
        return (0 until rows.length()).map {rows.opt(it) as? JSONObject ?: invalid()}
    }
    private fun invalid():Nothing=throw QualityReadUnavailable()
}
