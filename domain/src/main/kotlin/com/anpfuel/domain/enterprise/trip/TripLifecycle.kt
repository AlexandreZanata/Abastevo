package com.anpfuel.domain.enterprise.trip

/** Closed failure vocabulary shared with the Go package through trip-v2 (EMP-TRIP-011 adds CONFLICT and BEFORE_START). */
enum class TripFailure { INVALID, SCOPE, INCOMPLETE, FORBIDDEN, INVALID_TRANSITION, UNSUPPORTED_COMMAND, FINISH_BEFORE_START, CONFLICT, BEFORE_START }

/** An absent part of the eligible context or of a lifecycle command, reported in this order. */
enum class TripMissing { ASSET, DRIVER, DEPARTURE_EVIDENCE, DECLARATION, FINAL_EVIDENCE, REASON }

class TripDomainException(val failure: TripFailure = TripFailure.INVALID, missing: List<TripMissing> = emptyList()) :
    IllegalArgumentException("trip: ${failure.name.lowercase()}") {
    /** Every absent part, in order; empty unless INCOMPLETE. */
    val missing: List<TripMissing> = missing.toList()
}

internal fun invalid(): TripDomainException = TripDomainException()

/** Trip operational states of the canonical catalog (08-state-model §3). */
enum class TripState {
    PLANNED, ASSIGNED, ACCEPTED, IN_PROGRESS, PAUSED, COMPLETED, CANCELLED;

    val terminal: Boolean get() = this == COMPLETED || this == CANCELLED

    companion object {
        fun parse(raw: String): TripState = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/** Operational trip commands. */
enum class TripCommand {
    START, FINISH, PAUSE, RESUME, CANCEL;

    companion object {
        fun parse(raw: String): TripCommand = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/**
 * The operational table (EDR-178, EMP-TRIP-011 EDR-252), first failure first: terminal states (a new attempt is a new
 * trip), the mission-owned states PLANNED, ASSIGNED and ACCEPTED, then the moves of an open trip: PAUSE IN_PROGRESS →
 * PAUSED, RESUME PAUSED → IN_PROGRESS, CANCEL → CANCELLED and FINISH → COMPLETED from either; any other pair is
 * INVALID_TRANSITION. START itself creates a trip only through [Trip.start].
 */
object TripLifecycle {
    fun transition(from: TripState, command: TripCommand): TripState {
        if (from.terminal) throw TripDomainException(TripFailure.INVALID_TRANSITION)
        if (from != TripState.IN_PROGRESS && from != TripState.PAUSED) throw TripDomainException(TripFailure.UNSUPPORTED_COMMAND)
        return when {
            command == TripCommand.FINISH -> TripState.COMPLETED
            command == TripCommand.CANCEL -> TripState.CANCELLED
            command == TripCommand.PAUSE && from == TripState.IN_PROGRESS -> TripState.PAUSED
            command == TripCommand.RESUME && from == TripState.PAUSED -> TripState.IN_PROGRESS
            else -> throw TripDomainException(TripFailure.INVALID_TRANSITION)
        }
    }
}

/** Closed trip review vocabulary: BR-REV states plus VOIDED, an input never derived from operation. */
enum class TripReviewState {
    DRAFT, SUBMITTED, IN_REVIEW, APPROVED, RETURNED, REJECTED, CANCELLED, VOIDED;

    companion object {
        fun parse(raw: String): TripReviewState = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/** Independent operational and review states (BR-REV-001); any combination is representable. */
data class TripStanding(val operational: TripState, val review: TripReviewState) {
    /** Operational completion only. */
    val completed: Boolean get() = operational == TripState.COMPLETED

    /** Administrative approval only. */
    val approved: Boolean get() = review == TripReviewState.APPROVED
}
