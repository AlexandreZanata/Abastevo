package com.anpfuel.data.profile

import android.content.ContentResolver
import android.net.Uri
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.io.InputStream
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** Original bytes only, bounded even for providers reporting unknown/false sizes. */
object SignedClaimDocument {
    const val MAX_BYTES = 5 * 1024 * 1024

    fun readBounded(input: InputStream): ByteArray {
        val output = ByteArrayOutputStream()
        val buffer = ByteArray(8192)
        while (true) {
            val count = input.read(buffer, 0, minOf(buffer.size, MAX_BYTES + 1 - output.size()))
            if (count < 0) break
            if (count == 0) throw IOException("Document read made no progress")
            output.write(buffer, 0, count)
            if (output.size() > MAX_BYTES) throw IOException("Document exceeds limit")
        }
        val bytes = output.toByteArray()
        if (bytes.size < 5 || !bytes.copyOfRange(0, 5).contentEquals("%PDF-".toByteArray())) throw IOException("PDF required")
        return bytes
    }

    suspend fun read(resolver: ContentResolver, uri: Uri): ByteArray = withContext(Dispatchers.IO) {
        require(uri.scheme == "content")
        if (resolver.getType(uri) != "application/pdf") throw IOException("PDF required")
        val stream = resolver.openInputStream(uri) ?: throw IOException("Document unavailable")
        stream.use(::readBounded)
    }

    suspend fun export(resolver: ContentResolver, uri: Uri, declaration: String) = withContext(Dispatchers.IO) {
        require(uri.scheme == "content" && declaration.isNotBlank() && declaration.length <= 8192)
        val stream = resolver.openOutputStream(uri, "wt") ?: throw IOException("Document unavailable")
        stream.use { it.write(declaration.toByteArray(Charsets.UTF_8)) }
    }
}
