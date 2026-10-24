package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.domain.repository.LedgerSyncScope
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * MEX-15 sync scope controls (device + per-dataset participation).
 * Local-only, app-private; revoking a dataset stops its commands from
 * dispatching and from restore pulls without deleting held work.
 */
@Singleton
class LedgerSyncScopeStore internal constructor(
    private val readBoolean: (String, Boolean) -> Boolean,
    private val writeBoolean: (String, Boolean) -> Unit,
) : LedgerSyncScope {

    @Inject
    constructor(@ApplicationContext context: Context) : this(
        readBoolean = { key, def ->
            context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE).getBoolean(key, def)
        },
        writeBoolean = { key, value ->
            val prefs: SharedPreferences =
                context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            prefs.edit().putBoolean(key, value).apply()
        },
    )

    override fun isDatasetEnabled(dataset: String): Boolean = readBoolean(keyFor(dataset), true)

    override fun setDatasetEnabled(dataset: String, enabled: Boolean) {
        writeBoolean(keyFor(dataset), enabled)
    }

    override fun isDeviceEnabled(): Boolean = readBoolean(KEY_DEVICE, true)

    override fun setDeviceEnabled(enabled: Boolean) {
        writeBoolean(KEY_DEVICE, enabled)
    }

    companion object {
        const val PREFS_NAME = "ledger_sync_scope"
        const val KEY_DEVICE = "device_enabled"

        fun keyFor(dataset: String): String = "dataset|$dataset"
    }
}
