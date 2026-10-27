package com.anpfuel.domain.contributor

/**
 * B-BR-CTB contributor profile self-service (CTB-08/12).
 *
 * Client validation mirrors the server bounds so mistakes fail fast; the
 * server stays authoritative and its error codes are shown verbatim.
 */
enum class ProfileVisibility { PUBLIC, HIDDEN }

data class ProfileLinkInput(
    val platform: String,
    val handle: String,
)

data class ProfileEdit(
    val displayName: String,
    val links: List<ProfileLinkInput>,
    val visibility: ProfileVisibility,
    val publicConsent: Boolean,
    val showRoles: Boolean,
)

data class StoredProfile(
    val displayName: String,
    val links: List<ContributorLink>,
    val visibility: ProfileVisibility,
    val publicConsent: Boolean,
    val showRoles: Boolean,
)

data class ProfileRead(
    val profile: StoredProfile?,
    val roles: List<String>,
)

private val DISPLAY_ALLOWED = setOf(' '.code, '.'.code, '\''.code, '-'.code)

private fun isMark(codePoint: Int): Boolean = when (Character.getType(codePoint).toByte()) {
    Character.NON_SPACING_MARK, Character.COMBINING_SPACING_MARK, Character.ENCLOSING_MARK -> true
    else -> false
}

/**
 * NFC is server-side; the client mirrors the rune count (2–40) and the
 * allowlist (letters, marks, digits, space and `.'-`), by code point.
 */
fun displayNameError(raw: String): String? {
    val name = raw.trim()
    val runes = name.codePointCount(0, name.length)
    if (runes < 2 || runes > 40) return "length"
    for (codePoint in name.codePoints()) {
        if (!(Character.isLetterOrDigit(codePoint) || isMark(codePoint) || codePoint in DISPLAY_ALLOWED)) {
            return "charset"
        }
    }
    return null
}

fun isValidProfileLink(platform: String, handle: String): Boolean {
    if (ContributorLinkHosts.hostFor(platform) == null) return false
    if (handle.isBlank() || handle.length > 255) return false
    return true
}

fun profileEditError(edit: ProfileEdit): String? {
    if (displayNameError(edit.displayName) != null) return "display-name"
    if (edit.links.size > 2) return "links"
    if (edit.links.map { it.platform }.toSet().size != edit.links.size) return "links"
    if (!edit.links.all { isValidProfileLink(it.platform, it.handle) }) return "links"
    if (edit.visibility == ProfileVisibility.PUBLIC && !edit.publicConsent) return "consent"
    return null
}
