package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant

data class FuelProductSelected(
    override val id: DomainId,
    override val timestamp: Instant,
    val payload: Payload,
) : DomainEvent {

    data class Payload(
        val fuelProduct: FuelProduct,
        val municipality: String,
        val state: BrazilianState,
    )

    companion object {
        fun create(
            payload: Payload,
            id: DomainId = DomainId.generate(),
            timestamp: Instant = Instant.now(),
        ): FuelProductSelected = FuelProductSelected(id, timestamp, payload)
    }
}
