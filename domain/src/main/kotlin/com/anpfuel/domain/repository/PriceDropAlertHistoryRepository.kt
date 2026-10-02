package com.anpfuel.domain.repository

import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Local-only record of shown price-drop alerts (P23-T01, UC-014).
 *
 * One entry per vehicle: the survey week of its last shown alert.
 * Backs duplicate suppression across repeated evaluations — the same
 * drop must notify once per week, never once per run. Device-local
 * only: no account, no backend, no PII beyond the vehicle id already
 * owned on device. Clearing app data resets history (alerts may
 * repeat once), which is safe and explicit.
 */
interface PriceDropAlertHistoryRepository {

    suspend fun lastNotifiedWeek(vehicleId: DomainId): SurveyWeek?

    suspend fun recordNotified(vehicleId: DomainId, week: SurveyWeek)
}
