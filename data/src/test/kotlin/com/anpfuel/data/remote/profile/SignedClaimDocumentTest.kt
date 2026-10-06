package com.anpfuel.data.remote.profile

import com.anpfuel.data.profile.SignedClaimDocument
import java.io.ByteArrayInputStream
import java.io.IOException
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class SignedClaimDocumentTest {
    @Test fun `exact cap remains byte identical and oversize unknown-length input refuses`() {
        val pdf = ByteArray(SignedClaimDocument.MAX_BYTES) { 65 }
        "%PDF-".toByteArray().copyInto(pdf)
        assertArrayEquals(pdf, SignedClaimDocument.readBounded(ByteArrayInputStream(pdf)))
        assertThrows(IOException::class.java) { SignedClaimDocument.readBounded(ByteArrayInputStream(pdf + byteArrayOf(0))) }
        assertThrows(IOException::class.java) { SignedClaimDocument.readBounded(ByteArrayInputStream("not a pdf".toByteArray())) }
    }
    @Test fun `expired read permission propagates without parsing or rewriting`() {
        val denied = object : java.io.InputStream() {
            override fun read(): Int = throw SecurityException("revoked")
        }
        assertThrows(SecurityException::class.java) { SignedClaimDocument.readBounded(denied) }
    }
}
