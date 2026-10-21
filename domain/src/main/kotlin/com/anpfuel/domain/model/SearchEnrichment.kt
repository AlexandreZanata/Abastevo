package com.anpfuel.domain.model

import com.anpfuel.domain.valueobject.AvailabilitySnapshot
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry

/**
 * MEX-04 — Single-pass catalog/availability enrichment for search results.
 * [entries] is keyed by the requested result and holds the canonical catalog
 * entry, or null when the result has no catalog counterpart.
 */
data class EnrichedSearchCandidate(
    val result: MunicipalitySearchResult,
    val entry: MunicipalityCatalogEntry?,
)

data class SearchEnrichment(
    val candidates: List<EnrichedSearchCandidate>,
    val snapshot: AvailabilitySnapshot,
)
