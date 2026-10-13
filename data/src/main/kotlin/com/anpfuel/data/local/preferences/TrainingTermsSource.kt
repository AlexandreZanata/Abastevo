package com.anpfuel.data.local.preferences

import android.content.Context
import com.anpfuel.domain.repository.TrainingTermsDocument
import dagger.hilt.android.qualifiers.ApplicationContext
import java.security.MessageDigest
import javax.inject.Inject

class TrainingTermsSource @Inject constructor(@ApplicationContext private val context:Context) {
    fun read():TrainingTermsDocument = context.assets.open(FILE).use { input ->
        val out=java.io.ByteArrayOutputStream()
        val buffer=ByteArray(4096)
        while(out.size()<=65536) { val n=input.read(buffer,0,minOf(buffer.size,65537-out.size()));if(n<0)break;out.write(buffer,0,n) }
        val bytes=out.toByteArray()
        require(bytes.size in 1..65536)
        decode(bytes)
    }
    companion object {
        const val FILE="training-consent-pt-BR.txt"
        internal fun decode(bytes:ByteArray):TrainingTermsDocument {
            require(bytes.size in 1..65536)
            val text=bytes.toString(Charsets.UTF_8)
            require(text.toByteArray(Charsets.UTF_8).contentEquals(bytes))
            require(text.contains("Versão: training-terms-v1 |"))
            val digest=MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }
            return TrainingTermsDocument("training-terms-v1",digest,text)
        }
    }
}
