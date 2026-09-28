package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException

/**
 * BR-016 — Municipality catalog completeness.
 *
 * GIVEN IBGE catalog loaded
 * WHEN user searches
 * THEN every federative unit municipality appears in the index regardless of current week data.
 */
object MunicipalityCatalogCompletenessRule {

    const val MIN_IBGE_MUNICIPALITIES = 5_570

    fun isCatalogComplete(catalogEntryCount: Int): Boolean =
        catalogEntryCount >= MIN_IBGE_MUNICIPALITIES

    fun validate(catalogEntryCount: Int) {
        if (!isCatalogComplete(catalogEntryCount)) {
            throw DomainException(
                "BR-016: Municipality catalog must contain at least " +
                    "$MIN_IBGE_MUNICIPALITIES entries (found $catalogEntryCount)",
            )
        }
    }
}
