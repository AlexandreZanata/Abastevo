package com.anpfuel.domain.event

import com.anpfuel.domain.state.SyncJobState

enum class SyncJobOutcome {
    SUCCESS,
    PARTIAL,
    NO_NEW_DATA,
    FAILED,
}

data class SyncJobCompletedPayload(
    val finalState: SyncJobState,
    val outcome: SyncJobOutcome,
    val detail: String? = null,
)
