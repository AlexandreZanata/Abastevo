package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant

data class DeviceLocationResolved(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val state: BrazilianState,
        val municipality: String,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): DeviceLocationResolved = DeviceLocationResolved(id, timestamp, payload)
    }
}
