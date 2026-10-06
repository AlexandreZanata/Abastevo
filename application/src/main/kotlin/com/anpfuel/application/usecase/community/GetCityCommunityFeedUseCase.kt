package com.anpfuel.application.usecase.community

import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.domain.community.CityCommunityFeedReader
import com.anpfuel.domain.community.FeedCity
import com.anpfuel.domain.community.FeedQuery
import com.anpfuel.domain.repository.MunicipalityCatalogRepository

/** Reuses manual city preference and canonical IBGE catalog; no GPS or account. */
class GetCityCommunityFeedUseCase(
    private val reader: CityCommunityFeedReader,
    private val locations: SelectLocationUseCase,
    private val catalog: MunicipalityCatalogRepository,
) {
    suspend fun city(): FeedCity? {
        val preferred = locations.getPreferredLocation() ?: return null
        val entry = catalog.findCatalogEntry(preferred.state, preferred.municipality) ?: return null
        val code = entry.ibgeCode ?: return null
        return FeedCity(code, preferred.state, preferred.municipality)
    }
    suspend operator fun invoke(query: FeedQuery, cursor: String? = null) = reader.read(query, cursor)
}
