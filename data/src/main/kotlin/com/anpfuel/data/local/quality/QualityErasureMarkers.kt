package com.anpfuel.data.local.quality

import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.security.MessageDigest

/** Independent no-backup erasure intent survives a temporary Room outage. No claim/auth data. */
interface QualityErasureMarkers {
    fun block(local:String)
    fun blocked(local:String):Boolean
    fun pending():List<String>
    fun clear(local:String)
}
class FileQualityErasureMarkers(private val directory:File):QualityErasureMarkers {
    @Synchronized override fun block(local:String) {
        require(local.startsWith("account:") && local.length<=256 && local.none {it.isISOControl()})
        directory.mkdirs()
        val target=file(local)
        if(!target.exists() && !File(directory,target.name+".tmp").exists() && files().map {it.name.removeSuffix(".tmp")}.distinct().size>=64)throw IOException("quality cleanup capacity unavailable")
        val temp=File(directory,target.name+".tmp")
        FileOutputStream(temp).use {it.write(local.toByteArray());it.fd.sync()}
        if(!temp.renameTo(target)){temp.delete();throw IOException("quality cleanup unavailable")}
    }
    override fun blocked(local:String)=file(local).let {it.exists() || File(directory,it.name+".tmp").exists()}
    @Synchronized override fun pending():List<String> = files().groupBy {it.name.removeSuffix(".tmp")}.map {(_,copies) ->
        val entry=copies.firstOrNull {it.name.endsWith(".erase")} ?: copies.first()
        if(entry.length()>256)throw IOException("quality cleanup unavailable")
        val local=entry.readText()
        val target=file(local)
        if(!local.startsWith("account:") || local.any {it.isISOControl()} || target.name!=entry.name.removeSuffix(".tmp"))throw IOException("quality cleanup unavailable")
        if(entry.name.endsWith(".tmp") && !entry.renameTo(target))throw IOException("quality cleanup unavailable")
        copies.filter {it.name.endsWith(".tmp")}.forEach {it.delete()}
        local
    }
    @Synchronized override fun clear(local:String) {val file=file(local);if(file.exists() && !file.delete())throw IOException("quality cleanup unavailable")}
    private fun files()=directory.listFiles {file ->file.name.matches(Regex("[0-9a-f]{64}\\.erase(\\.tmp)?"))}?.toList().orEmpty().also {if(it.map {file->file.name.removeSuffix(".tmp")}.distinct().size>64)throw IOException("quality cleanup unavailable")}
    private fun file(local:String)=File(directory,MessageDigest.getInstance("SHA-256").digest(local.toByteArray()).joinToString(""){"%02x".format(it)}+".erase")
}
class MemoryQualityErasureMarkers:QualityErasureMarkers {
    private val owners=java.util.concurrent.ConcurrentHashMap.newKeySet<String>()
    override fun block(local:String){owners.add(local)}
    override fun blocked(local:String)=local in owners
    override fun pending()=owners.toList()
    override fun clear(local:String){owners.remove(local)}
}
