package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant

data class VehicleRemoved(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val vehicleId: DomainId,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): VehicleRemoved = VehicleRemoved(id, timestamp, payload)
    }
}
