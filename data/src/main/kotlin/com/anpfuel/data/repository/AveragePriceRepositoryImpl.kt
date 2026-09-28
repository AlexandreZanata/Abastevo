package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.mapper.EntityDomainMapper
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.rule.MunicipalitySearchTextNormalizer
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class AveragePriceRepositoryImpl @Inject constructor(
    private val averagePriceDao: AveragePriceDao,
    private val surveyWeekDao: SurveyWeekDao,
) : AveragePriceRepository {

    override suspend fun getLatestImportedSurveyWeek(): SurveyWeek? =
        surveyWeekDao.findAllOrderedByEndDateDesc()
            .firstOrNull()
            ?.let { entity ->
                SurveyWeek.fromIsoDates(entity.startDate, entity.endDate)
            }

    override suspend fun getPricesByMunicipality(
        state: BrazilianState,
        municipality: String,
        surveyWeek: SurveyWeek,
    ): List<AveragePrice> {
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value
        return averagePriceDao.findByLocation(
            surveyWeekId = surveyWeekId,
            state = state.abbreviation,
            municipality = normalizeMunicipality(municipality),
        ).map { entity ->
            EntityDomainMapper.toAveragePrice(entity, surveyWeek)
        }
    }

    override suspend fun getPriceHistory(
        state: BrazilianState,
        municipality: String,
        fuelProduct: FuelProduct,
    ): List<AveragePrice> =
        averagePriceDao.findPriceHistory(
            state = state.abbreviation,
            municipality = normalizeMunicipality(municipality),
            fuelProduct = fuelProduct.name,
        ).map { entity ->
            val surveyWeekEntity = requireNotNull(
                surveyWeekDao.findById(entity.surveyWeekId),
            ) { "Missing survey week ${entity.surveyWeekId}" }
            val surveyWeek = SurveyWeek.fromIsoDates(
                surveyWeekEntity.startDate,
                surveyWeekEntity.endDate,
            )
            EntityDomainMapper.toAveragePrice(entity, surveyWeek)
        }

    override suspend fun getStatesWithData(surveyWeek: SurveyWeek): List<BrazilianState> {
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value
        return averagePriceDao.findDistinctStates(surveyWeekId)
            .mapNotNull(BrazilianState::fromAbbreviation)
    }

    override suspend fun getMunicipalitiesWithData(
        state: BrazilianState,
        surveyWeek: SurveyWeek,
    ): List<String> {
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value
        return averagePriceDao.findDistinctMunicipalities(
            surveyWeekId = surveyWeekId,
            state = state.abbreviation,
        )
    }

    private fun normalizeMunicipality(municipality: String): String =
        MunicipalitySearchTextNormalizer.normalize(municipality)
}
