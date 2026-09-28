package com.anpfuel.domain.repository

import com.anpfuel.domain.event.DomainEvent

/**
 * Port for publishing immutable domain events from use cases.
 */
interface DomainEventPublisher {

    suspend fun publish(event: DomainEvent)
}
