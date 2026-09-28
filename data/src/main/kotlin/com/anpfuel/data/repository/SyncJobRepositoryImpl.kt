package com.anpfuel.data.repository

import com.anpfuel.data.local.preferences.SyncStateDataStore
import com.anpfuel.domain.repository.SyncJobRepository
import com.anpfuel.domain.state.SyncJobState
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class SyncJobRepositoryImpl @Inject constructor(
    private val syncStateDataStore: SyncStateDataStore,
) : SyncJobRepository {

    override suspend fun getCurrentState(): SyncJobState =
        syncStateDataStore.readState()

    override suspend fun saveState(state: SyncJobState) {
        syncStateDataStore.writeState(state)
    }
}
