package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant

data class VehicleUpdated(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val vehicleId: DomainId,
        val displayName: String,
        val fuelProduct: FuelProduct,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): VehicleUpdated = VehicleUpdated(id, timestamp, payload)
    }
}
