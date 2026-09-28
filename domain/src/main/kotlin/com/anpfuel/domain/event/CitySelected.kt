package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant

data class CitySelected(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val municipality: String,
        val state: BrazilianState,
        val surveyWeekId: DomainId,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): CitySelected = CitySelected(id, timestamp, payload)
    }
}
