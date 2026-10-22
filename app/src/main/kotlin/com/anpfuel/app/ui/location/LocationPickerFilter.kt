package com.anpfuel.app.ui.location

import com.anpfuel.application.usecase.location.CatalogMunicipalityItem
import com.anpfuel.domain.rule.MunicipalitySearchTextNormalizer
import com.anpfuel.domain.valueobject.BrazilianState

internal object LocationPickerFilter {

    fun filterStates(
        states: List<BrazilianState>,
        query: String,
        stateLabel: (BrazilianState) -> String,
    ): List<BrazilianState> {
        val normalizedQuery = normalizeQuery(query) ?: return states
        return states.filter { state ->
            val abbreviation = state.abbreviation.uppercase()
            if (normalizedQuery.length == 2 && abbreviation == normalizedQuery) {
                true
            } else {
                matches(
                    normalizedQuery,
                    abbreviation,
                    stateLabel(state),
                    state.enumLabel(),
                )
            }
        }
    }

    fun filterMunicipalities(
        municipalities: List<CatalogMunicipalityItem>,
        query: String,
    ): List<CatalogMunicipalityItem> {
        val normalizedQuery = normalizeQuery(query) ?: return municipalities
        return municipalities.filter { item ->
            matches(normalizedQuery, item.municipality)
        }
    }

    /**
     * MEX-06 precomputed filter index: normalized names are computed once
     * per loaded list instead of once per item per keystroke. The filtered
     * result is identical to [filterMunicipalities]; only the repeated
     * normalization work is removed from composition.
     */
    fun indexMunicipalities(
        municipalities: List<CatalogMunicipalityItem>,
    ): Map<CatalogMunicipalityItem, String> =
        municipalities.associateWith { item ->
            MunicipalitySearchTextNormalizer.normalize(item.municipality)
        }

    fun filterIndexed(
        index: Map<CatalogMunicipalityItem, String>,
        query: String,
    ): List<CatalogMunicipalityItem> {
        val normalizedQuery = normalizeQuery(query)
            ?: return index.keys.toList()
        return index.filterValues { normalizedName ->
            normalizedName.contains(normalizedQuery)
        }.keys.toList()
    }

    private fun normalizeQuery(query: String): String? {
        val normalized = MunicipalitySearchTextNormalizer.normalize(query)
        return normalized.takeIf { it.isNotEmpty() }
    }

    private fun matches(normalizedQuery: String, vararg candidates: String): Boolean =
        candidates.any { candidate ->
            MunicipalitySearchTextNormalizer.normalize(candidate).contains(normalizedQuery)
        }

    private fun BrazilianState.enumLabel(): String =
        name.replace('_', ' ')
}
