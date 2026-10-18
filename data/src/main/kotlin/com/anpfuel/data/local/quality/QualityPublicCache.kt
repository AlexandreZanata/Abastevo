package com.anpfuel.data.local.quality

import java.io.File
import java.io.FileOutputStream
import java.security.MessageDigest
import java.util.Base64
import org.json.JSONObject

/** Public official JSON only; caller stores after complete typed contract validation. */
class QualityPublicCache(private val directory:File,private val now:()->Long={System.currentTimeMillis()}) {
    data class Entry(val body:String,val etag:String?)
    fun load(url:String):Entry?=synchronized(lock) {
        val tick=clock() ?: return@synchronized null
        val file=file(url)
        val row=read(file) ?: return@synchronized null
        val stamp=row.optLong("at",-1)
        if(stamp<0 || tick<stamp || tick-stamp>=RETENTION_MILLIS || row.optString("key")!=hash(url.toByteArray())) {file.delete();return@synchronized null}
        try {
            val bytes=Base64.getDecoder().decode(row.getString("body"))
            if(bytes.size>MAX_BODY || row.getString("sha256")!=hash(bytes))throw IllegalArgumentException()
            Entry(bytes.toString(Charsets.UTF_8),row.optString("etag").takeIf {it.isNotEmpty() && it.length<=256 && it.none {c->c=='\r'||c=='\n'}})
        }catch(_:Exception){file.delete();null}
    }
    fun save(url:String,body:String,etag:String?)=synchronized(lock) {
        val bytes=body.toByteArray(Charsets.UTF_8)
        if(bytes.size>MAX_BODY)return@synchronized
        val tick=clock() ?: return@synchronized
        val row=JSONObject().put("v",1).put("at",tick).put("key",hash(url.toByteArray()))
            .put("origin",java.net.URI(url).let {it.scheme+"://"+it.rawAuthority})
            .put("sha256",hash(bytes)).put("body",Base64.getEncoder().encodeToString(bytes))
        if(etag!=null && etag.length<=256 && etag.none {it=='\r'||it=='\n'})row.put("etag",etag)
        atomic(file(url),row.toString())
        val entries=entries().sortedWith(compareBy<File> {it.lastModified()}.thenBy {it.name}).toMutableList()
        var size=entries.sumOf {it.length()}
        while(entries.size>MAX_ENTRIES || size>MAX_TOTAL) {
            val oldest=entries.removeAt(0);size-=oldest.length();oldest.delete()
        }
    }
    fun clearOrigin(origin:String)=synchronized(lock) {
        entries().forEach {file -> val row=read(file);if(row==null || row.optString("origin").trimEnd('/')==origin.trimEnd('/'))file.delete()}
    }
    private fun clock():Long? {
        directory.mkdirs()
        directory.listFiles {file -> file.name.matches(Regex("([0-9a-f]{64}\\.json|clock-v1)\\.tmp"))}?.forEach {it.delete()}
        val tick=now()
        val watermark=File(directory,"clock-v1")
        val prior=if(watermark.exists())watermark.takeIf {it.length()<=32}?.readText()?.toLongOrNull() else null
        if(tick<0 || (prior!=null && tick<prior) || (prior==null && entries().isNotEmpty())) {
            entries().forEach {it.delete()};watermark.delete()
            if(tick>=0)atomic(watermark,tick.toString())
            return null
        }
        atomic(watermark,tick.toString())
        return tick
    }
    private fun read(file:File):JSONObject?=try {
        if(!file.isFile || file.length()>MAX_ENVELOPE){file.delete();null}
        else JSONObject(file.readText()).takeIf {it.optInt("v")==1}.also {if(it==null)file.delete()}
    }catch(_:Exception){file.delete();null}
    private fun entries()=directory.listFiles {file -> file.name.matches(Regex("[0-9a-f]{64}\\.json"))}?.toList().orEmpty()
    private fun file(url:String)=File(directory,hash(url.toByteArray())+".json")
    private fun atomic(file:File,text:String) {
        val temp=File(directory,file.name+".tmp")
        FileOutputStream(temp).use {stream -> stream.write(text.toByteArray(Charsets.UTF_8));stream.fd.sync()}
        if(!temp.renameTo(file)){temp.delete();throw java.io.IOException("quality cache unavailable")}
    }
    companion object {
        private val lock=Any()
        const val MAX_BODY=1024*1024
        const val MAX_ENTRIES=64
        const val MAX_TOTAL=16L*1024*1024
        const val MAX_ENVELOPE=2L*1024*1024
        const val RETENTION_MILLIS=7L*24*60*60*1000
        private fun hash(bytes:ByteArray)=MessageDigest.getInstance("SHA-256").digest(bytes).joinToString(""){"%02x".format(it)}
    }
}
