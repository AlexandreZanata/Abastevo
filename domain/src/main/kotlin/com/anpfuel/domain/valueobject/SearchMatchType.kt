package com.anpfuel.domain.valueobject

/**
 * Match quality tier for intelligent municipality search ranking (BR-017).
 * Lower [rank] values sort higher in result lists.
 */
enum class SearchMatchType(val rank: Int) {
    EXACT_PREFIX(rank = 0),
    ACCENT_NORMALIZED(rank = 1),
    TYPO_TOLERANT(rank = 2),
    SUBSTRING(rank = 3),
}
