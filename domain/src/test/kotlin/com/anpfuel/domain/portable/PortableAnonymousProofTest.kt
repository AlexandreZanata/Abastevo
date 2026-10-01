package com.anpfuel.domain.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T03: portable anonymous-proof shape and window rules.
 *
 * Pure shape/window checks mirrored from the frozen backend profile
 * (P03-T01); signature math lives in `:data AnonymousProofCryptoTest`,
 * which replays the frozen vectors.
 */
class PortableAnonymousProofTest {

    @Test
    fun `bodyless base has eight lines in frozen order`() {
        val lines = PortableAnonymousProof.buildBaseLines(
            method = "POST",
            authority = "api.example.invalid",
            path = "/v1/contributors",
            query = "",
            contentType = null,
            contentDigest = null,
            created = 1735689600L,
            expires = 1735689900L,
            keyId = "fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624",
            nonce = "ch-1.client-1",
        )
        assertEquals(8, lines.size)
        assertTrue(PortableAnonymousProof.isLinesShapeValid(lines))
        assertEquals("\"@method\": POST", lines[0])
        assertEquals("\"nonce\": \"ch-1.client-1\"", lines[7])
    }

    @Test
    fun `body base has ten lines and reordered or duplicated sets refuse`() {
        val body = PortableAnonymousProof.buildBaseLines(
            method = "POST",
            authority = "api.example.invalid",
            path = "/v1/contributors",
            query = "",
            contentType = "application/json",
            contentDigest = "sha-512=:MEUCIQ==:",
            created = 1735689600L,
            expires = 1735689900L,
            keyId = "fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624",
            nonce = "ch-1.client-1",
        )
        assertEquals(10, body.size)
        assertTrue(PortableAnonymousProof.isLinesShapeValid(body))
        val swapped = body.toMutableList()
        val first = swapped[0]
        swapped[0] = swapped[1]
        swapped[1] = first
        assertFalse(PortableAnonymousProof.isLinesShapeValid(swapped))
        assertFalse(PortableAnonymousProof.isLinesShapeValid(body + body[0]))
        assertFalse(PortableAnonymousProof.isLinesShapeValid(body.dropLast(1)))
    }

    @Test
    fun `purpose fingerprint nonce and window parse`() {
        assertEquals("REGISTER", PortableAnonymousProof.parsePurpose("register"))
        assertEquals("SIGN", PortableAnonymousProof.parsePurpose(" SIGN "))
        assertEquals(null, PortableAnonymousProof.parsePurpose("bearer"))
        assertTrue(
            PortableAnonymousProof.isFingerprintShape(
                "fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624",
            ),
        )
        assertFalse(PortableAnonymousProof.isFingerprintShape("fp:XYZ"))
        assertEquals(
            Pair("ch-1", "client-1"),
            PortableAnonymousProof.splitNonce("ch-1.client-1"),
        )
        assertEquals(null, PortableAnonymousProof.splitNonce("no-dot"))
        assertEquals(null, PortableAnonymousProof.splitNonce("ch-1.client 1"))
        assertTrue(PortableAnonymousProof.isWindowValid(1735689600L, 1735689900L, 1735689700L))
        assertFalse(PortableAnonymousProof.isWindowValid(1735689600L, 1735689901L, 1735689700L))
        assertFalse(PortableAnonymousProof.isWindowValid(1735689600L, 1735689900L, 1735689900L))
        assertFalse(PortableAnonymousProof.isWindowValid(1735690100L, 1735690400L, 1735689700L))
    }

    @Test
    fun `rotation intent and thumbprint inputs freeze exact bytes`() {
        assertEquals(
            "{\"new_jwk\":{\"kty\":\"EC\",\"crv\":\"P-256\",\"x\":\"X\",\"y\":\"Y\"}," +
                "\"old_challenge_id\":\"old-1\",\"new_challenge_id\":\"new-1\"}",
            PortableAnonymousProof.rotationIntentJson("X", "Y", "old-1", "new-1"),
        )
        assertEquals(
            "{\"crv\":\"P-256\",\"kty\":\"EC\",\"x\":\"X\",\"y\":\"Y\"}",
            PortableAnonymousProof.thumbprintInput("X", "Y"),
        )
    }
}
