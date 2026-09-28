package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import java.time.Instant

data class PriceTableImported(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val surveyWeekId: DomainId,
        val tableType: PriceTableType,
        val rowCount: Int,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): PriceTableImported = PriceTableImported(id, timestamp, payload)
    }
}
