package com.anpfuel.data.repository

import com.anpfuel.domain.event.DomainEvent
import com.anpfuel.domain.repository.DomainEventPublisher
import javax.inject.Inject
import javax.inject.Singleton

/**
 * No-op publisher for v1 — use cases already collect events for callers that need them.
 */
@Singleton
class NoOpDomainEventPublisher @Inject constructor() : DomainEventPublisher {

    override suspend fun publish(event: DomainEvent) = Unit
}
