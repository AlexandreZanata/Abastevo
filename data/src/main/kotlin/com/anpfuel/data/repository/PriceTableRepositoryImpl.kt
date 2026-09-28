package com.anpfuel.data.repository

import androidx.room.withTransaction
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.ImportAuditLogDao
import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.local.fts.MunicipalityFtsIndexer
import com.anpfuel.data.local.preferences.PriceTableMetadataStore
import com.anpfuel.data.mapper.EntityDomainMapper
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class PriceTableRepositoryImpl @Inject constructor(
    private val database: AnpFuelDatabase,
    private val surveyWeekDao: SurveyWeekDao,
    private val averagePriceDao: AveragePriceDao,
    private val stationPriceDao: StationPriceDao,
    private val priceTableMetadataStore: PriceTableMetadataStore,
    private val ftsIndexer: MunicipalityFtsIndexer,
) : PriceTableRepository {

    override suspend fun getImportedPriceSurveys(): List<PriceSurvey> =
        surveyWeekDao.findAllOrderedByEndDateDesc()
            .map(EntityDomainMapper::toPriceSurvey)

    override suspend fun findPriceTableByUrl(sourceUrl: String): PriceTable? =
        priceTableMetadataStore.findByUrl(sourceUrl)

    override suspend fun savePriceSurvey(priceSurvey: PriceSurvey) {
        surveyWeekDao.insert(EntityDomainMapper.toSurveyWeekEntity(priceSurvey))
    }

    override suspend fun saveDiscoveredPriceTable(priceTable: PriceTable) {
        priceTableMetadataStore.save(priceTable)
    }

    override suspend fun importAveragePrices(prices: List<AveragePrice>) {
        if (prices.isEmpty()) {
            return
        }

        val entities = prices.map(EntityDomainMapper::toAveragePriceEntity)
        database.withTransaction {
            averagePriceDao.insertAll(entities)
        }
        ftsIndexer.syncAfterBatchInsert()
    }

    override suspend fun importStationPrices(prices: List<StationPrice>) {
        if (prices.isEmpty()) {
            return
        }

        val entities = prices.map(EntityDomainMapper::toStationPriceEntity)
        database.withTransaction {
            stationPriceDao.insertAll(entities)
        }
    }

    override suspend fun countImportedSurveyWeeks(): Int =
        surveyWeekDao.count()

    override suspend fun findPriceSurveyById(id: DomainId): PriceSurvey? =
        surveyWeekDao.findById(id.value)?.let(EntityDomainMapper::toPriceSurvey)

    override suspend fun findPriceSurveyByWeek(surveyWeek: SurveyWeek): PriceSurvey? =
        surveyWeekDao.findByDates(
            startDate = surveyWeek.startDate.toString(),
            endDate = surveyWeek.endDate.toString(),
        )?.let(EntityDomainMapper::toPriceSurvey)
}
