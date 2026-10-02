package com.anpfuel.data.repository

import com.anpfuel.data.local.preferences.PriceDropAlertHistoryDataStore
import com.anpfuel.domain.repository.PriceDropAlertHistoryRepository
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P23-T01 — device-local alert history behind the domain port.
 * No account, no backend, no export: entries live and die with the
 * app data they describe.
 */
@Singleton
class PriceDropAlertHistoryRepositoryImpl @Inject constructor(
    private val historyDataStore: PriceDropAlertHistoryDataStore,
) : PriceDropAlertHistoryRepository {

    override suspend fun lastNotifiedWeek(vehicleId: DomainId): SurveyWeek? =
        historyDataStore.lastNotifiedWeek(vehicleId)

    override suspend fun recordNotified(vehicleId: DomainId, week: SurveyWeek) {
        historyDataStore.recordNotified(vehicleId, week)
    }
}
