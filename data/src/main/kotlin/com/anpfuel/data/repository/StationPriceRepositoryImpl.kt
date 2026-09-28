package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.mapper.EntityDomainMapper
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.rule.MunicipalitySearchTextNormalizer
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class StationPriceRepositoryImpl @Inject constructor(
    private val stationPriceDao: StationPriceDao,
    private val surveyWeekDao: SurveyWeekDao,
) : StationPriceRepository {

    override suspend fun getStationPrices(
        state: BrazilianState,
        municipality: String,
        fuelProduct: FuelProduct,
        surveyWeek: SurveyWeek,
    ): List<StationPrice> {
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value
        return stationPriceDao.findByLocationAndProduct(
            surveyWeekId = surveyWeekId,
            state = state.abbreviation,
            municipality = normalizeMunicipality(municipality),
            fuelProduct = fuelProduct.name,
        ).map { entity ->
            EntityDomainMapper.toStationPrice(entity, surveyWeek)
        }
    }

    override suspend fun hasStationData(
        surveyWeek: SurveyWeek,
        state: BrazilianState,
        municipality: String,
    ): Boolean {
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value
        return stationPriceDao.countByLocation(
            surveyWeekId = surveyWeekId,
            state = state.abbreviation,
            municipality = normalizeMunicipality(municipality),
        ) > 0
    }

    override suspend fun deleteStationPricesOlderThanRetention(retentionWeeks: Int) {
        val weeksWithStationData = surveyWeekDao.findAllWithStationDataOrderedByEndDateDesc()
        if (weeksWithStationData.size <= retentionWeeks) {
            return
        }

        val expiredWeekIds = weeksWithStationData
            .drop(retentionWeeks)
            .map { it.id }

        if (expiredWeekIds.isEmpty()) {
            return
        }

        stationPriceDao.deleteBySurveyWeekIds(expiredWeekIds)
        expiredWeekIds.forEach { weekId ->
            surveyWeekDao.clearStationImportedAt(weekId)
        }
    }

    private fun normalizeMunicipality(municipality: String): String =
        MunicipalitySearchTextNormalizer.normalize(municipality)
}
