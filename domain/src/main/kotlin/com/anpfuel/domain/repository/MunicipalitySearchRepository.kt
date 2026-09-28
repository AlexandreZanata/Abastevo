package com.anpfuel.domain.repository

import com.anpfuel.domain.model.MunicipalitySearchResult

/**
 * Port for FTS-backed municipality search (UC-004).
 */
interface MunicipalitySearchRepository {

    suspend fun search(query: String, limit: Int = DEFAULT_LIMIT): List<MunicipalitySearchResult>

    companion object {
        const val DEFAULT_LIMIT = 20
    }
}
