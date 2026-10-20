package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.mapper.EntityDomainMapper
import com.anpfuel.domain.discovery.DiscoveryQuery
import com.anpfuel.domain.discovery.DiscoverySort
import com.anpfuel.domain.discovery.DiscoveryStationsRule
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.rule.MinimumSearchLengthRule
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

    override suspend fun getStationPricesPage(
        state: BrazilianState,
        municipality: String,
        fuelProduct: FuelProduct,
        surveyWeek: SurveyWeek,
        search: String?,
        sort: DiscoverySort,
        page: Int,
        pageSize: Int,
    ): DiscoveryStationsRule.DiscoveryPage {
        if (page < 0) throw DomainException("discovery page must be >= 0")
        if (pageSize !in 1..DiscoveryQuery.MAX_PAGE_SIZE) {
            throw DomainException("discovery pageSize must be 1..${DiscoveryQuery.MAX_PAGE_SIZE}")
        }
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value
        val normalizedMunicipality = normalizeMunicipality(municipality)
        val effectiveSearch = escapeLike(search?.trim()?.lowercase(java.util.Locale.ROOT).orEmpty()
            .takeIf { it.length >= MinimumSearchLengthRule.MIN_LENGTH } ?: "")
        val entities = when (sort) {
            DiscoverySort.PRICE_ASC -> stationPriceDao.findPageByPrice(
                surveyWeekId = surveyWeekId,
                state = state.abbreviation,
                municipality = normalizedMunicipality,
                fuelProduct = fuelProduct.name,
                search = effectiveSearch,
                limit = pageSize,
                offset = page * pageSize,
            )
            DiscoverySort.RECENCY_DESC -> stationPriceDao.findPageByRecency(
                surveyWeekId = surveyWeekId,
                state = state.abbreviation,
                municipality = normalizedMunicipality,
                fuelProduct = fuelProduct.name,
                search = effectiveSearch,
                limit = pageSize,
                offset = page * pageSize,
            )
        }
        val totalCount = stationPriceDao.countPage(
            surveyWeekId = surveyWeekId,
            state = state.abbreviation,
            municipality = normalizedMunicipality,
            fuelProduct = fuelProduct.name,
            search = effectiveSearch,
        )
        return DiscoveryStationsRule.DiscoveryPage(
            items = entities.map { entity -> EntityDomainMapper.toStationPrice(entity, surveyWeek) },
            hasMore = (page + 1) * pageSize < totalCount,
            totalCount = totalCount,
        )
    }

    override suspend fun findStationPrice(
        state: BrazilianState,
        municipality: String,
        fuelProduct: FuelProduct,
        surveyWeek: SurveyWeek,
        cnpjDigits: String,
    ): StationPrice? {
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value
        return stationPriceDao.findByCnpj(
            surveyWeekId = surveyWeekId,
            state = state.abbreviation,
            municipality = normalizeMunicipality(municipality),
            fuelProduct = fuelProduct.name,
            cnpjDigits = cnpjDigits,
        )?.let { entity -> EntityDomainMapper.toStationPrice(entity, surveyWeek) }
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

    /** LIKE metacharacters stay literal, mirroring `contains` semantics. */
    private fun escapeLike(search: String): String =
        search.replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_")
}
