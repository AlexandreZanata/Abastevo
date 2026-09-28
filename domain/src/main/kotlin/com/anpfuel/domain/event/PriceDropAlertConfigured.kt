package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.VehiclePriceSource
import java.time.Instant

data class PriceDropAlertConfigured(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val vehicleId: DomainId,
        val enabled: Boolean,
        val alertPriceSource: VehiclePriceSource,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): PriceDropAlertConfigured = PriceDropAlertConfigured(id, timestamp, payload)
    }
}
