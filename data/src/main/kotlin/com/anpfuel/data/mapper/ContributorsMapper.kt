package com.anpfuel.data.mapper

import com.anpfuel.domain.contributor.Contributor
import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.ContributorLink
import com.anpfuel.domain.contributor.ContributorLinkHosts
import com.anpfuel.domain.contributor.ContributorPage
import org.json.JSONArray
import org.json.JSONObject

private const val MAX_ITEMS = 64
private const val MAX_ROLES = 8
private const val MAX_TEXT_CHARS = 100
private val SINCE_PATTERN = Regex("^\\d{4}-\\d{2}$")

/**
 * B-BR-CTB public contributors mapping (CTB-09/10).
 *
 * Drops unknown platforms, non-https urls and wrong hosts instead of
 * rendering them; drops blank display names. Malformed optional fields
 * degrade to absent, never to fabricated identity.
 */
object ContributorsMapper {

    fun page(root: JSONObject): ContributorPage {
        try {
            if (!root.has("items")) throw invalid("items")
            val itemsRaw = root.optJSONArray("items") ?: JSONArray()
            if (itemsRaw.length() > MAX_ITEMS) throw invalid("items")
            val nextCursor = root.optString("next_cursor", "")
            if (nextCursor.length > 4096) throw invalid("cursor")
            val items = mutableListOf<Contributor>()
            for (index in 0 until itemsRaw.length()) {
                contributor(itemsRaw.optJSONObject(index) ?: continue)?.let(items::add)
            }
            return ContributorPage(items, nextCursor)
        } catch (e: ContributorException) {
            throw e
        } catch (e: Exception) {
            throw invalid("body")
        }
    }

    fun contributor(raw: JSONObject?): Contributor? {
        if (raw == null) return null
        try {
            val displayName = raw.optString("display_name", "")
            if (displayName.isBlank() || displayName.length > MAX_TEXT_CHARS) return null
            val linksRaw = raw.optJSONArray("links") ?: JSONArray()
            val links = mutableListOf<ContributorLink>()
            for (index in 0 until linksRaw.length()) {
                link(linksRaw.optJSONObject(index))?.let(links::add)
            }
            val githubLogin = raw.optString("github_login", "").takeIf { it.isNotEmpty() }
            if (githubLogin != null && (githubLogin.length > 39 || githubLogin.isBlank())) return null
            val since = raw.optString("contributor_since", "")
                .takeIf { SINCE_PATTERN.matches(it) } ?: ""
            val rolesRaw = raw.optJSONArray("roles") ?: JSONArray()
            if (rolesRaw.length() > MAX_ROLES) return null
            val roles = mutableListOf<String>()
            for (index in 0 until rolesRaw.length()) {
                val role = rolesRaw.opt(index)
                if (role !is String || role.isBlank() || role.length > 64) return null
                roles.add(role)
            }
            return Contributor(displayName, links, githubLogin, since, roles)
        } catch (_: Exception) {
            return null
        }
    }

    fun link(raw: JSONObject?): ContributorLink? {
        if (raw == null) return null
        val platform = raw.optString("platform", "")
        val handle = raw.optString("handle", "")
        val url = raw.optString("url", "")
        val expected = ContributorLinkHosts.hostFor(platform) ?: return null
        if (handle.isBlank() || handle.length > 255) return null
        if (!isHttpsHost(url, expected)) return null
        return ContributorLink(platform, handle, url)
    }

    private fun isHttpsHost(raw: String, expected: String): Boolean {
        if (!ContributorLinkHosts.isAllowedUrl(raw)) return false
        return try {
            java.net.URI(raw).host == expected
        } catch (_: Exception) {
            false
        }
    }

    private fun invalid(part: String) =
        ContributorException.Invalid("contributors.invalid-$part")
}
