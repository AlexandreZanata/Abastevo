package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException

/**
 * BR-007 — Municipality search requires at least two characters.
 */
object MinimumSearchLengthRule {

    const val MIN_LENGTH = 2

    fun isSearchAllowed(query: String): Boolean =
        query.trim().length >= MIN_LENGTH

    fun validate(query: String) {
        if (!isSearchAllowed(query)) {
            throw DomainException(
                "BR-007: Search query must contain at least $MIN_LENGTH characters",
            )
        }
    }
}
