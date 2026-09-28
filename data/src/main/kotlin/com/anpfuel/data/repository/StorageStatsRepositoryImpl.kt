package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.domain.model.StorageUsage
import com.anpfuel.domain.repository.StorageStatsRepository
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class StorageStatsRepositoryImpl @Inject constructor(
    private val averagePriceDao: AveragePriceDao,
    private val stationPriceDao: StationPriceDao,
    private val surveyWeekDao: SurveyWeekDao,
) : StorageStatsRepository {

    override suspend fun getStorageUsage(): StorageUsage = StorageUsage(
        summaryRowCount = averagePriceDao.count(),
        stationRowCount = stationPriceDao.count(),
        importedWeekCount = surveyWeekDao.count(),
    )
}
