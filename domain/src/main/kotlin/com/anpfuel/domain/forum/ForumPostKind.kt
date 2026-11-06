package com.anpfuel.domain.forum

/**
 * Post kinds in Forum Abastevo.
 *
 * Phase 1 exclusively reads EDITORIAL posts.
 * Forward compatible with NEWS, COMMUNITY, and TOPIC.
 */
enum class ForumPostKind(val wireValue: String) {
    EDITORIAL("EDITORIAL"),
    NEWS("NEWS"),
    COMMUNITY("COMMUNITY"),
    TOPIC("TOPIC"),
    BULLETIN("BULLETIN"),
    UNKNOWN("UNKNOWN");

    companion object {
        fun fromWire(value: String?): ForumPostKind {
            if (value.isNullOrBlank()) return UNKNOWN
            return entries.firstOrNull { it.wireValue.equals(value.trim(), ignoreCase = true) } ?: UNKNOWN
        }
    }
}
