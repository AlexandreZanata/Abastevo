package com.anpfuel.domain.enterprise.tracking

/** Closed failure vocabulary shared with the Go package through session-v1. */
enum class TrackingFailure {
    INVALID, SCOPE, FORBIDDEN, TRACKING_OFF, PERSONAL_TRIP, IMPLICIT_START, SESSION_EXISTS, INVALID_TRANSITION, TRIP_STATE, OUT_OF_ORDER
}

class TrackingDomainException(val failure: TrackingFailure = TrackingFailure.INVALID) :
    IllegalArgumentException("tracking: ${failure.name.lowercase()}")

internal fun invalid(): TrackingDomainException = TrackingDomainException()

/** Tracking session states of the canonical catalog (08-state-model §3). */
enum class TrackingSessionState {
    NOT_STARTED, RECORDING, SUSPENDED, INTERRUPTED, ENDED;

    val terminal: Boolean get() = this == ENDED

    companion object {
        fun parse(raw: String): TrackingSessionState = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/** The device location permission observed at a start, resume or reduction. */
enum class LocationPermission {
    PRECISE, APPROXIMATE, NONE;

    companion object {
        fun parse(raw: String): LocationPermission = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/** The organization's tracking policy resolved at opening. */
enum class TrackingPolicy {
    OFF, OPTIONAL, EXPECTED;

    companion object {
        fun parse(raw: String): TrackingPolicy = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/** Where an opening, start or resume request came from. */
enum class TrackingOrigin {
    TRIP_SCREEN, NOTIFICATION, PERSONAL, BOOT, WORKER, PUSH, GEOFENCE, SERVER;

    companion object {
        fun parse(raw: String): TrackingOrigin = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/** The typed reason recorded with a history entry. */
enum class TrackingCause {
    NOT_STARTED, PERMISSION_DENIED, NOTIFICATION_BLOCKED, TRIP_PAUSED, LONG_STATIONARY, PERMISSION_REVOKED, PROCESS_STOPPED,
    DEVICE_RESTARTED, STOPPED_BY_DRIVER, OWNER_CHANGED, PRECISION_REDUCED, TRIP_COMPLETED, TRIP_CANCELLED, MEMBERSHIP_ENDED,
    DEVICE_REVOKED;

    companion object {
        fun parse(raw: String): TrackingCause = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/** The kind of actor of a history entry, fixed by command and cause. */
enum class TrackingActor {
    DRIVER, DEVICE, TRIP, SERVER;

    companion object {
        fun parse(raw: String): TrackingActor = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/** Track quality of the canonical catalog (08-state-model §3). */
enum class TrackQuality {
    PRECISE, APPROXIMATE, INSUFFICIENT;

    companion object {
        fun parse(raw: String): TrackQuality = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}
