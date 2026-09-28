package com.anpfuel.data.local.catalog

import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.local.entity.MunicipalityCatalogEntity
import com.anpfuel.data.mapper.AnpStateMapper
import com.anpfuel.domain.rule.MunicipalitySearchTextNormalizer
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class MunicipalityCatalogResolver @Inject constructor(
    private val municipalityCatalogDao: MunicipalityCatalogDao,
) {

    suspend fun resolve(
        stateLabel: String,
        anpMunicipality: String,
    ): MunicipalityCatalogEntity? {
        val stateAbbrev = runCatching { AnpStateMapper.toAbbreviation(stateLabel) }.getOrNull()
            ?: return null
        return resolveByStateAbbrev(stateAbbrev, anpMunicipality)
    }

    suspend fun resolveByStateAbbrev(
        stateAbbrev: String,
        anpMunicipality: String,
    ): MunicipalityCatalogEntity? {
        val normalized = MunicipalitySearchTextNormalizer.normalize(anpMunicipality)
        return municipalityCatalogDao.findByStateAndNormalizedName(stateAbbrev, normalized)
    }
}
