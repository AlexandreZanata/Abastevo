package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import java.time.Instant

data class PriceTableDiscovered(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val url: String,
        val tableType: PriceTableType,
        val surveyWeek: SurveyWeek,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): PriceTableDiscovered = PriceTableDiscovered(id, timestamp, payload)
    }
}
