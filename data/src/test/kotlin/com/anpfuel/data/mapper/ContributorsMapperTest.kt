package com.anpfuel.data.mapper

import com.anpfuel.domain.contributor.ContributorException
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ContributorsMapperTest {

    private fun item(
        name: String = "Ana",
        links: String = """[{"platform":"github","handle":"octocat","url":"https://github.com/octocat"}]""",
        github: String = """"github_login":"octocat"""",
        since: String = """"contributor_since":"2026-03"""",
    ) = """{"display_name":"$name","links":$links,$github,$since,"roles":["Suporte"]}"""

    @Test fun `valid page maps with server links`() {
        val page = ContributorsMapper.page(
            JSONObject("""{"items":[${item()}],"next_cursor":"abc"}"""),
        )
        assertEquals("abc", page.nextCursor)
        assertEquals(1, page.items.size)
        val one = page.items[0]
        assertEquals("Ana", one.displayName)
        assertEquals("octocat", one.githubLogin)
        assertEquals("2026-03", one.contributorSince)
        assertEquals(listOf("Suporte"), one.roles)
        assertEquals(1, one.links.size)
        assertEquals("https://github.com/octocat", one.links[0].url)
    }

    @Test fun `unknown platform is dropped and contributor kept`() {
        val page = ContributorsMapper.page(
            JSONObject(
                """{"items":[${item(links = """[{"platform":"myspace","handle":"x","url":"https://myspace.com/x"}]""")}],"next_cursor":""}""",
            ),
        )
        assertTrue(page.items[0].links.isEmpty())
    }

    @Test fun `non-https link is dropped`() {
        val page = ContributorsMapper.page(
            JSONObject(
                """{"items":[${item(links = """[{"platform":"x","handle":"u","url":"http://x.com/u"}]""")}],"next_cursor":""}""",
            ),
        )
        assertTrue(page.items[0].links.isEmpty())
    }

    @Test fun `wrong host is dropped`() {
        val page = ContributorsMapper.page(
            JSONObject(
                """{"items":[${item(links = """[{"platform":"github","handle":"o","url":"https://evil.com/o"}]""")}],"next_cursor":""}""",
            ),
        )
        assertTrue(page.items[0].links.isEmpty())
    }

    @Test fun `blank display name drops the item`() {
        val page = ContributorsMapper.page(
            JSONObject("""{"items":[${item(name = "  ")}],"next_cursor":""}"""),
        )
        assertTrue(page.items.isEmpty())
    }

    @Test fun `malformed since degrades to blank`() {
        val page = ContributorsMapper.page(
            JSONObject("""{"items":[${item(since = """"contributor_since":"march"""")}],"next_cursor":""}"""),
        )
        assertEquals("", page.items[0].contributorSince)
    }

    @Test fun `missing items array is invalid`() {
        assertThrows(ContributorException.Invalid::class.java) {
            ContributorsMapper.page(JSONObject("""{"next_cursor":""}"""))
        }
    }

    @Test fun `absent github login stays null`() {
        val page = ContributorsMapper.page(
            JSONObject("""{"items":[${item(github = """"github_login":null""")}],"next_cursor":""}"""),
        )
        assertNull(page.items[0].githubLogin)
    }
}
