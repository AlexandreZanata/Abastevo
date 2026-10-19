package com.anpfuel.data.local.quality

import com.anpfuel.data.local.media.AndroidPhotoKeystoreKeys
import java.security.KeyStore
import javax.crypto.SecretKey

interface QualityReportKeys {
    fun getOrCreate(ownerKey:String,local:String):SecretKey?
    fun delete(ownerKey:String,local:String):Boolean
    fun eraseOwner(local:String):Boolean
}
class AndroidQualityReportKeys:QualityReportKeys {
    @Synchronized override fun getOrCreate(ownerKey:String,local:String):SecretKey? =
        AndroidPhotoKeystoreKeys().getOrCreateKey(alias(ownerKey,local))
    @Synchronized override fun delete(ownerKey:String,local:String):Boolean=try {
        val store=KeyStore.getInstance("AndroidKeyStore");store.load(null)
        val alias=alias(ownerKey,local)
        store.deleteEntry(alias)
        !store.containsAlias(alias)
    }catch(_:Exception){false}
    @Synchronized override fun eraseOwner(local:String):Boolean=try {
        val store=KeyStore.getInstance("AndroidKeyStore");store.load(null)
        val prefix=prefix(local)
        val names=mutableListOf<String>()
        val aliases=store.aliases()
        while(aliases.hasMoreElements() && names.size<64) {
            val name=aliases.nextElement();if(name.startsWith(prefix))names.add(name)
        }
        names.forEach {store.deleteEntry(it)}
        val remaining=store.aliases()
        var absent=true
        while(remaining.hasMoreElements())if(remaining.nextElement().startsWith(prefix)){absent=false;break}
        absent
    }catch(_:Exception){false}
    private fun prefix(local:String):String {
        require(local.startsWith("account:") && local.length<=256)
        val digest=java.security.MessageDigest.getInstance("SHA-256").digest(local.toByteArray()).joinToString(""){"%02x".format(it)}
        return "com.anpfuel.quality.outbox.v1.$digest."
    }
    private fun alias(key:String,local:String):String {
        require(key.matches(Regex("[0-9a-f]{64}")))
        return prefix(local)+key
    }
}
