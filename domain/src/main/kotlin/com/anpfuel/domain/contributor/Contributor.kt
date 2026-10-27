package com.anpfuel.domain.contributor

/**
 * B-BR-CTB public project contributor (CTB-09/10).
 *
 * Server-built display fields only: never account ids, aliases, e-mail or
 * grant reasons. Unknown wire values stay explicit instead of granting
 * anything or crashing on forward-compatible payloads.
 */
data class ContributorLink(
    val platform: String,
    val handle: String,
    val url: String,
)

data class Contributor(
    val displayName: String,
    val links: List<ContributorLink>,
    val githubLogin: String?,
    val contributorSince: String,
    val roles: List<String>,
)

data class ContributorPage(
    val items: List<Contributor>,
    val nextCursor: String,
)

sealed class ContributorException(message: String) : Exception(message) {
    class Invalid(code: String) : ContributorException(code)
    class Denied(code: String) : ContributorException(code)
    class Unavailable(code: String) : ContributorException(code)
    class Offline(code: String) : ContributorException(code)
}

/**
 * CTB-08 server link hosts. The UI re-checks every url before opening it;
 * the mapper already drops anything outside this table.
 */
object ContributorLinkHosts {
    private val hosts = mapOf(
        "github" to "github.com",
        "gitlab" to "gitlab.com",
        "linkedin" to "www.linkedin.com",
        "x" to "x.com",
        "instagram" to "www.instagram.com",
        "youtube" to "www.youtube.com",
        "bluesky" to "bsky.app",
    )

    fun hostFor(platform: String): String? = hosts[platform]

    fun isAllowedUrl(raw: String): Boolean {
        val uri = try {
            java.net.URI(raw)
        } catch (_: Exception) {
            return false
        }
        if (uri.scheme != "https") return false
        val host = uri.host ?: return false
        return hosts.values.any { it == host }
    }
}
