package com.anpfuel.data.local.preferences

import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * P23-T01 — key/row codec for the device-local alert history.
 *
 * Pure mapping only (unit-tested): one string key per vehicle plus
 * the ISO week bounds shared with [ActiveSurveyWeekCodec]. The
 * DataStore wrapper stays a thin pass-through like the other
 * preference stores.
 */
internal object PriceDropAlertHistoryCodec {

    fun key(vehicleId: DomainId): String =
        "price_drop_notified_" + vehicleId.value

    fun encode(week: SurveyWeek): Pair<String, String> =
        week.startDate.toString() to week.endDate.toString()

    fun decode(startDate: String?, endDate: String?): SurveyWeek? =
        ActiveSurveyWeekCodec.decode(startDate, endDate)
}
