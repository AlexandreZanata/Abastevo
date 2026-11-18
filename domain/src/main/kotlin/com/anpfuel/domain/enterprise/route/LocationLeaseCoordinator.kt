package com.anpfuel.domain.enterprise.route

import java.time.Instant

/*
 * Single location owner lease coordinator between tracking and turn-by-turn
 * guidance (EMP-GPS-006, BR-GPS-011, BR-ROUTE-014..015). Guarantees at most one
 * hardware location subscription is requested regardless of consumer overlap.
 */

enum class LocationConsumer {
    TRACKING, GUIDANCE
}

/** Validated location fix distributed over active consumer leases (redacted in logs). */
class SharedLocationFix(
    val collectedAt: Instant,
    val point: GeoPointE7,
    val accuracyMetres: Int,
    val speedCmPerSecond: Int? = null,
    val bearingDegrees: Int? = null,
    val coarse: Boolean = false,
) {
    init {
        if (accuracyMetres !in 0..10_000) throw RouteDomainException()
        if (speedCmPerSecond != null && speedCmPerSecond !in 0..100_000) throw RouteDomainException()
        if (bearingDegrees != null && bearingDegrees !in 0..359) throw RouteDomainException()
    }

    override fun equals(other: Any?): Boolean =
        other is SharedLocationFix &&
            collectedAt == other.collectedAt &&
            point == other.point &&
            accuracyMetres == other.accuracyMetres &&
            speedCmPerSecond == other.speedCmPerSecond &&
            bearingDegrees == other.bearingDegrees &&
            coarse == other.coarse

    override fun hashCode(): Int {
        var result = collectedAt.hashCode()
        result = 31 * result + point.hashCode()
        result = 31 * result + accuracyMetres
        return result
    }

    override fun toString(): String = "SharedLocationFix(acc=${accuracyMetres}m,coarse=$coarse,redacted)"
}

/** Transition result describing whether the underlying hardware subscription must start or stop. */
data class LeaseTransition(
    val startHardwareSubscription: Boolean,
    val stopHardwareSubscription: Boolean,
    val activeConsumers: Set<LocationConsumer>,
)

/**
 * Coordinates TRACKING and GUIDANCE leases without initiating duplicate
 * hardware location subscriptions. Guidance fixes are never forwarded to the
 * server when only GUIDANCE is leased (BR-ROUTE-015).
 */
class LocationLeaseCoordinator {
    private val active = LinkedHashSet<LocationConsumer>()
    private var subscriptionStartCount = 0

    val activeConsumers: Set<LocationConsumer> get() = active.toSet()

    val hasActiveHardwareSubscription: Boolean get() = active.isNotEmpty()

    /** Number of times a hardware subscription was started (0->1 transitions only). */
    val hardwareStartCount: Int get() = subscriptionStartCount

    fun acquire(consumer: LocationConsumer): LeaseTransition {
        val wasEmpty = active.isEmpty()
        active.add(consumer)
        val shouldStart = wasEmpty && active.isNotEmpty()
        if (shouldStart) subscriptionStartCount++
        return LeaseTransition(
            startHardwareSubscription = shouldStart,
            stopHardwareSubscription = false,
            activeConsumers = active.toSet(),
        )
    }

    fun release(consumer: LocationConsumer): LeaseTransition {
        val wasActive = active.isNotEmpty()
        active.remove(consumer)
        val shouldStop = wasActive && active.isEmpty()
        return LeaseTransition(
            startHardwareSubscription = false,
            stopHardwareSubscription = shouldStop,
            activeConsumers = active.toSet(),
        )
    }

    /** Fans out an observed hardware fix only to currently holding consumers. */
    fun fanOut(fix: SharedLocationFix): Map<LocationConsumer, SharedLocationFix> {
        if (active.isEmpty()) return emptyMap()
        return active.associateWith { fix }
    }
}
