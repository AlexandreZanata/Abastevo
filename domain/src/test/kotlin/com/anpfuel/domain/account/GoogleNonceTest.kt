package com.anpfuel.domain.account

import java.util.Base64
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class GoogleNonceTest {
    @Test
    fun hashBindsTheExactRawTextUsingLowercaseHex() {
        assertEquals(
            "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
            GoogleNonce.claimFor("abc"),
        )
    }

    @Test
    fun newNonceContains32RandomBytesEncodedWithoutPadding() {
        val first = GoogleNonce.generate()
        val second = GoogleNonce.generate()
        assertTrue(Regex("[A-Za-z0-9_-]{43}").matches(first.raw))
        assertEquals(32, Base64.getUrlDecoder().decode(first.raw).size)
        assertEquals(GoogleNonce.claimFor(first.raw), first.claim)
        assertNotEquals(first.raw, second.raw)
    }
}
