package com.anpfuel.data.local.preferences

import com.anpfuel.domain.valueobject.SurveyWeek

internal object ActiveSurveyWeekCodec {

    fun encode(week: SurveyWeek?): Pair<String, String>? =
        week?.let { it.startDate.toString() to it.endDate.toString() }

    fun decode(startDate: String?, endDate: String?): SurveyWeek? {
        if (startDate.isNullOrBlank() || endDate.isNullOrBlank()) {
            return null
        }
        return SurveyWeek.fromIsoDates(startDate, endDate)
    }
}
