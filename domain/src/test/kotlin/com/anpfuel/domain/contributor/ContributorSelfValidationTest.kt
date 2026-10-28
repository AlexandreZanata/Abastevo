package com.anpfuel.domain.contributor

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ContributorSelfValidationTest {

    @Test fun `github login mirrors CTB-03`() {
        assertTrue(isValidGithubLogin("octocat"))
        assertTrue(isValidGithubLogin("a"))
        assertTrue(isValidGithubLogin("A1-b2"))
        for (bad in listOf("", "a".repeat(40), "-abc", "abc-", "a--b", "a_b", "a b", "a.b")) {
            assertFalse(isValidGithubLogin(bad), bad)
        }
    }

    @Test fun `gist id is a compact token`() {
        assertTrue(isValidGistId("abc123"))
        assertFalse(isValidGistId(""))
        assertFalse(isValidGistId("has space"))
        assertFalse(isValidGistId("x".repeat(129)))
    }

    @Test fun `display name bounds and charset`() {
        assertNull(displayNameError("Ana"))
        assertNull(displayNameError("  José Silva  "))
        assertNull(displayNameError("O'Neil-2."))
        assertEquals("length", displayNameError("A"))
        assertEquals("length", displayNameError("a".repeat(41)))
        assertEquals("charset", displayNameError("ana@x"))
        assertEquals("charset", displayNameError("a/b"))
        assertEquals("length", displayNameError("😀"))
        assertEquals("charset", displayNameError("a😀"))
        assertNull(displayNameError("Jose\u0301"))
        assertNull(displayNameError("𠀀𠀁"))
    }

    @Test fun `profile links allowlist and shape`() {
        assertTrue(isValidProfileLink("github", "octocat"))
        assertTrue(isValidProfileLink("x", "user_1"))
        assertFalse(isValidProfileLink("myspace", "x"))
        assertFalse(isValidProfileLink("github", ""))
        assertFalse(isValidProfileLink("github", "x".repeat(256)))
    }

    @Test fun `profile edit rules`() {
        val ok = ProfileEdit("Ana", listOf(ProfileLinkInput("github", "octocat")), ProfileVisibility.PUBLIC, true, false)
        assertNull(profileEditError(ok))
        assertEquals("display-name", profileEditError(ok.copy(displayName = "A")))
        assertEquals(
            "links",
            profileEditError(
                ok.copy(
                    links = listOf(
                        ProfileLinkInput("github", "a"),
                        ProfileLinkInput("x", "b"),
                        ProfileLinkInput("gitlab", "c"),
                    ),
                ),
            ),
        )
        assertEquals(
            "links",
            profileEditError(
                ok.copy(links = listOf(ProfileLinkInput("x", "a"), ProfileLinkInput("x", "b"))),
            ),
        )
        assertEquals("consent", profileEditError(ok.copy(publicConsent = false)))
        assertNull(profileEditError(ok.copy(visibility = ProfileVisibility.HIDDEN, publicConsent = false)))
    }

    @Test fun `claim states parse with unknown fallback`() {
        assertEquals(ClaimState.PENDING, ClaimState.parse("PENDING"))
        assertEquals(ClaimState.UNKNOWN, ClaimState.parse("FUTURE"))
        assertEquals(ClaimProof.VERIFIED, ClaimProof.parse("VERIFIED"))
        assertEquals(ClaimProof.UNKNOWN, ClaimProof.parse("BOGUS"))
    }
}
