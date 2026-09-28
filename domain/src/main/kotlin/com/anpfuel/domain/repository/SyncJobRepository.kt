package com.anpfuel.domain.repository

import com.anpfuel.domain.state.SyncJobState

/**
 * Port for persisting the active [SyncJobState] (UC-001, BR-015).
 */
interface SyncJobRepository {

    suspend fun getCurrentState(): SyncJobState

    suspend fun saveState(state: SyncJobState)
}
