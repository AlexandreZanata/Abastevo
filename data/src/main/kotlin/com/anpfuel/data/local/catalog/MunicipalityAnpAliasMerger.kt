package com.anpfuel.data.local.catalog

import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.local.fts.MunicipalityFtsIndexer
import com.anpfuel.domain.rule.MunicipalitySearchTextNormalizer
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class MunicipalityAnpAliasMerger @Inject constructor(
    private val averagePriceDao: AveragePriceDao,
    private val municipalityCatalogDao: MunicipalityCatalogDao,
    private val catalogResolver: MunicipalityCatalogResolver,
    private val ftsIndexer: MunicipalityFtsIndexer,
) {

    suspend fun mergeAliasesFromSurveyWeek(surveyWeekId: String) {
        val locations = averagePriceDao.findDistinctLocationsBySurveyWeek(surveyWeekId)
        var aliasesUpdated = false

        for (location in locations) {
            val catalogEntry = catalogResolver.resolveByStateAbbrev(
                stateAbbrev = location.state,
                anpMunicipality = location.municipality,
            ) ?: continue

            val normalizedAnpName = MunicipalitySearchTextNormalizer.normalize(location.municipality)
            val alreadyIndexed = normalizedAnpName == catalogEntry.normalizedName ||
                location.municipality == catalogEntry.anpAlias

            if (!alreadyIndexed) {
                municipalityCatalogDao.updateAnpAlias(catalogEntry.id, location.municipality)
                aliasesUpdated = true
            }
        }

        if (aliasesUpdated) {
            ftsIndexer.syncAfterCatalogChange()
        }
    }
}
