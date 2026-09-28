package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant

data class SyncRequested(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val source: SyncRequestSource,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): SyncRequested = SyncRequested(id, timestamp, payload)
    }
}
