package com.anpfuel.application.usecase.notification

import com.anpfuel.domain.notification.AppNotification
import com.anpfuel.domain.notification.AppNotificationContent
import com.anpfuel.domain.notification.AppNotificationSource
import com.anpfuel.domain.notification.LocalNotificationInbox
import com.anpfuel.domain.notification.NotificationFeedRule
import com.anpfuel.domain.notification.SystemNotificationPoster
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.PreviousSurveyWeekRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.GeographicScope
import com.anpfuel.domain.valueobject.SurveyWeek
import java.math.BigDecimal
import java.math.RoundingMode

/**
 * B-BR-NOTIF-05 — local alerts after a successful ANP weekly import, from
 * data already on the device: `local.new_week` once per newer week (never
 * for the very first import) and `local.city_average_drop` once per city and
 * week when a fuel's average fell. Each alert is kept in the device inbox and
 * posted as an Android notification only when the OS allows it.
 */
class EvaluateWeeklyLocalAlertsUseCase(
    private val averagePrices: AveragePriceRepository,
    private val priceTables: PriceTableRepository,
    private val preferences: UserPreferencesRepository,
    private val inbox: LocalNotificationInbox,
    private val poster: SystemNotificationPoster,
    private val nowMillis: () -> Long = System::currentTimeMillis,
) {
    /** Number of new local alerts recorded. */
    suspend operator fun invoke(): Int {
        val imported = priceTables.getImportedPriceSurveys().filter { it.hasSummaryData }
        val newest = imported.maxByOrNull { it.surveyWeek.endDate }?.surveyWeek ?: return 0
        val known = inbox.list().map { it.id }.toSet()
        var created = 0

        val weekId = "local.new_week:${newest.endDate}"
        if (imported.map { it.surveyWeek }.distinct().size > 1 && weekId !in known) {
            record(weekId, AppNotificationContent.NewSurveyWeek(newest.startDate.toString(), newest.endDate.toString()))
            created++
        }

        val prefs = preferences.getPreferences()
        val state = prefs.preferredState ?: return created
        val municipality = prefs.preferredMunicipality?.trim()?.takeIf { it.isNotEmpty() } ?: return created
        val previous = PreviousSurveyWeekRule.resolvePreviousWeek(newest, imported) ?: return created
        val dropId = "local.city_average_drop:${state.abbreviation}:${municipality.uppercase()}:${newest.endDate}"
        if (dropId in known) return created
        val drops = NotificationFeedRule.cityAverageDrops(
            previous = cityAverages(state, municipality, previous),
            current = cityAverages(state, municipality, newest),
        )
        if (drops.isNotEmpty()) {
            record(dropId, AppNotificationContent.CityAverageDrop(municipality, state.abbreviation, drops))
            created++
        }
        return created
    }

    private suspend fun record(id: String, content: AppNotificationContent) {
        val notification = AppNotification(id, AppNotificationSource.LOCAL, content, nowMillis(), read = false)
        inbox.add(notification)
        if (poster.canPost()) poster.postLocal(notification)
    }

    private suspend fun cityAverages(state: BrazilianState, municipality: String, week: SurveyWeek): Map<FuelProduct, Long> =
        averagePrices.getPricesByMunicipality(state = state, municipality = municipality, surveyWeek = week)
            .sortedBy { if (it.geographicScope == GeographicScope.MUNICIPALITY) 0 else 1 }
            .mapNotNull { price -> price.average?.let { price.fuelProduct to it.value.toMilli() } }
            .distinctBy { it.first }
            .toMap()

    private fun BigDecimal.toMilli(): Long = multiply(BigDecimal(1000)).setScale(0, RoundingMode.HALF_UP).toLong()
}
