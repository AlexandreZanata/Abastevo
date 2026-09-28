package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant

data class StationDetailRequested(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val surveyWeekId: DomainId,
        val municipality: String,
        val state: BrazilianState,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): StationDetailRequested = StationDetailRequested(id, timestamp, payload)
    }
}
