package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant

data class PreferencesUpdated(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val changedKeys: Set<String>,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): PreferencesUpdated = PreferencesUpdated(id, timestamp, payload)
    }
}
