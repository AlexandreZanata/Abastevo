package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant

data class SyncJobCompleted(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: SyncJobCompletedPayload,
) : DomainEvent {

    companion object {
        fun create(
            payload: SyncJobCompletedPayload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): SyncJobCompleted = SyncJobCompleted(id, timestamp, payload)
    }
}
