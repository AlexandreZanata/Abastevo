package com.anpfuel.domain.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PortableAuthTest {

    @Test
    fun allowlistsProvidersAndIssuers() {
        assertTrue(PortableAuth.isProvider("google"))
        assertTrue(PortableAuth.isProvider("apple"))
        assertFalse(PortableAuth.isProvider("github"))
        assertFalse(PortableAuth.isProvider(""))
        assertEquals(
            "https://accounts.google.com",
            PortableAuth.issuerOf("google"),
        )
        assertEquals(
            "https://appleid.apple.com",
            PortableAuth.issuerOf("apple"),
        )
        assertEquals("", PortableAuth.issuerOf("github"))
    }

    @Test
    fun checksCodeShapeOnly() {
        assertTrue(PortableAuth.isCodeShape("482916"))
        assertTrue(PortableAuth.isCodeShape("000000"))
        assertFalse(PortableAuth.isCodeShape("48291"))
        assertFalse(PortableAuth.isCodeShape("4829167"))
        assertFalse(PortableAuth.isCodeShape("48a916"))
        assertFalse(PortableAuth.isCodeShape(""))
        assertFalse(PortableAuth.isCodeShape(" 48291"))
    }

    @Test
    fun computesSessionLiveness() {
        val session = PortableAuth.Session(
            familyId = "f", accountId = "a",
            accessToken = "at", refreshToken = "rt",
            accessExpiresAt = 1_000_900L, absoluteExpiresAt = 4_259_200L,
        )
        assertTrue(PortableAuth.isAccessLive(session, 1_000_000L))
        assertFalse(PortableAuth.isAccessLive(session, 1_000_900L))
        assertTrue(PortableAuth.isRefreshLive(session, 1_000_901L))
        assertFalse(PortableAuth.isRefreshLive(session, 4_259_200L))
    }

    @Test
    fun bindsExactCallbackAndRefusesSubstitution() {
        val attempt = PortableAuth.LoginAttempt("google", "n-1", "s-1")
        val valid = PortableAuth.ProviderCallback("google", "tok", "n-1", "s-1")
        assertNotNull(PortableAuth.bind(attempt, valid))
        assertNull(PortableAuth.bind(attempt, valid.copy(nonce = "n-2")))
        assertNull(PortableAuth.bind(attempt, valid.copy(state = "s-2")))
        assertNull(PortableAuth.bind(attempt, valid.copy(provider = "apple")))
        assertNull(PortableAuth.bind(attempt, valid.copy(idToken = "")))
        assertNull(PortableAuth.bind(attempt, valid.copy(provider = "github")))
    }

    @Test
    fun parsesCallbackUrlStrictly() {
        val parsed = PortableAuth.parseCallback(
            "anpfuel://auth/callback?provider=google&id_token=tok&nonce=n-1&state=s-1",
        )
        assertNotNull(parsed)
        assertEquals("google", parsed!!.provider)
        assertEquals("n-1", parsed.nonce)
        assertEquals("s-1", parsed.state)
        assertNull(PortableAuth.parseCallback("https://auth/callback?provider=google&id_token=t&nonce=n&state=s"))
        assertNull(PortableAuth.parseCallback("anpfuel://other/callback?provider=google&id_token=t&nonce=n&state=s"))
        assertNull(PortableAuth.parseCallback("anpfuel://auth/callback?provider=google&nonce=n&state=s"))
        assertNull(PortableAuth.parseCallback("anpfuel://auth/callback?provider=&id_token=t&nonce=n&state=s"))
        assertNull(
            PortableAuth.parseCallback(
                "anpfuel://auth/callback?provider=google&provider=apple&id_token=t&nonce=n&state=s",
            ),
        )
        assertNull(PortableAuth.parseCallback("not a url"))
    }

}
