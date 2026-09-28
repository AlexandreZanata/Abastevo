package com.anpfuel.domain.state

import com.anpfuel.domain.exception.DomainException

enum class SyncJobState {
    IDLE,
    DISCOVERING,
    DOWNLOADING,
    PARSING,
    IMPORTING,
    COMPLETED,
    FAILED,
    ;

    val isTerminal: Boolean
        get() = this == COMPLETED || this == FAILED

    val isActive: Boolean
        get() = this in ACTIVE_STATES

    fun allowedTransitions(): Set<SyncJobState> = ALLOWED_TRANSITIONS[this].orEmpty()

    fun transitionTo(target: SyncJobState): SyncJobState {
        if (target !in allowedTransitions()) {
            throw DomainException("Invalid SyncJob transition from $name to ${target.name}")
        }
        return target
    }

    companion object {
        private val ACTIVE_STATES = setOf(DISCOVERING, DOWNLOADING, PARSING, IMPORTING)

        private val ALLOWED_TRANSITIONS: Map<SyncJobState, Set<SyncJobState>> = mapOf(
            IDLE to setOf(DISCOVERING),
            DISCOVERING to setOf(DOWNLOADING, COMPLETED, FAILED),
            DOWNLOADING to setOf(PARSING, FAILED),
            PARSING to setOf(IMPORTING, FAILED),
            IMPORTING to setOf(COMPLETED, FAILED),
            COMPLETED to setOf(IDLE),
            FAILED to setOf(IDLE),
        )
    }
}
