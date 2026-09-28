package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant

/**
 * Base type for immutable domain events (past tense names, agent core §5).
 */
sealed interface DomainEvent {
    val id: DomainId
    val timestamp: Instant
}
