package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant

data class StationNavigationRequested(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val cnpj: Cnpj,
        val navigationQuery: String,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): StationNavigationRequested = StationNavigationRequested(id, timestamp, payload)
    }
}
