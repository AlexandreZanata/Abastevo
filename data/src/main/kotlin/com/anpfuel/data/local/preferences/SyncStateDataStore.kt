package com.anpfuel.data.local.preferences

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.anpfuel.domain.state.SyncJobState
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

private val Context.syncStateDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "sync_state",
)

@Singleton
class SyncStateDataStore @Inject constructor(
    @ApplicationContext context: Context,
) {
    private val dataStore = context.syncStateDataStore

    suspend fun readState(): SyncJobState =
        dataStore.data.map { preferences ->
            preferences[Keys.SYNC_JOB_STATE]
                ?.let { runCatching { SyncJobState.valueOf(it) }.getOrNull() }
                ?: SyncJobState.IDLE
        }.first()

    suspend fun writeState(state: SyncJobState) {
        dataStore.edit { preferences ->
            preferences[Keys.SYNC_JOB_STATE] = state.name
        }
    }

    suspend fun resetToIdle() {
        writeState(SyncJobState.IDLE)
    }

    private object Keys {
        val SYNC_JOB_STATE = stringPreferencesKey("sync_job_state")
    }
}
