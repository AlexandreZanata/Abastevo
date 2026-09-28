package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode
import java.time.Instant

data class SurveyWeekSelected(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val surveyWeek: SurveyWeek,
        val selectionMode: SurveyWeekSelectionMode,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): SurveyWeekSelected = SurveyWeekSelected(id, timestamp, payload)
    }
}
