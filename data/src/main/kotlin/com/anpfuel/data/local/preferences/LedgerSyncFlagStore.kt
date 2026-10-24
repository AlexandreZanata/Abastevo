package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.port.LedgerSyncFlagProvider
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * MEX-14 explicit ledger-sync opt-in (default OFF). Private by
 * construction: a single boolean in app-private preferences, no
 * account data, no server round-trip to read.
 */
@Singleton
class LedgerSyncFlagStore @Inject constructor(
    @ApplicationContext context: Context,
) : LedgerSyncFlagProvider {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun isEnabled(): Boolean = prefs.getBoolean(KEY_ENABLED, false)

    fun setEnabled(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_ENABLED, enabled).apply()
    }

    companion object {
        const val PREFS_NAME = "ledger_sync"
        const val KEY_ENABLED = "ledger_sync_enabled"
    }
}
