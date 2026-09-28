package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import java.time.Instant

data class PriceTableImportFailed(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val surveyWeekId: DomainId?,
        val tableType: PriceTableType,
        val detail: String,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): PriceTableImportFailed = PriceTableImportFailed(id, timestamp, payload)
    }
}
