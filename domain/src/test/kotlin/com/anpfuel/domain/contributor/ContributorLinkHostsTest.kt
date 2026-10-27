package com.anpfuel.domain.contributor

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ContributorLinkHostsTest {

    @Test fun `server-built urls pass per platform`() {
        val cases = mapOf(
            "github" to "https://github.com/octocat",
            "gitlab" to "https://gitlab.com/user.name",
            "linkedin" to "https://www.linkedin.com/in/user-name",
            "x" to "https://x.com/user_1",
            "instagram" to "https://www.instagram.com/user.name",
            "youtube" to "https://www.youtube.com/@user",
            "bluesky" to "https://bsky.app/profile/user.bsky.social",
        )
        for ((platform, url) in cases) {
            assertTrue(ContributorLinkHosts.isAllowedUrl(url), "$platform $url")
        }
        assertEquals("github.com", ContributorLinkHosts.hostFor("github"))
    }

    @Test fun `non-https urls are dropped`() {
        assertFalse(ContributorLinkHosts.isAllowedUrl("http://github.com/octocat"))
        assertFalse(ContributorLinkHosts.isAllowedUrl("github.com/octocat"))
        assertFalse(ContributorLinkHosts.isAllowedUrl("javascript:alert(1)"))
        assertFalse(ContributorLinkHosts.isAllowedUrl(""))
    }

    @Test fun `wrong hosts are dropped`() {
        assertFalse(ContributorLinkHosts.isAllowedUrl("https://evil.com/octocat"))
        assertFalse(ContributorLinkHosts.isAllowedUrl("https://github.com.evil.com/octocat"))
        assertFalse(ContributorLinkHosts.isAllowedUrl("https://m.youtube.com/@user"))
        assertFalse(ContributorLinkHosts.isAllowedUrl("https://youtu.be/abc"))
    }

    @Test fun `unknown platforms have no host`() {
        assertEquals(null, ContributorLinkHosts.hostFor("myspace"))
        assertEquals(null, ContributorLinkHosts.hostFor(""))
    }
}
