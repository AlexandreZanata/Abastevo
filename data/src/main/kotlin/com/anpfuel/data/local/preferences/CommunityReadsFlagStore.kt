package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.port.CommunityReadsFlagProvider
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P10-T02 community-reads flag (default ON since the Community tab and
 * the contribute station picker need backend reads with Room fallback;
 * enabling never removes ANP offline paths).
 *
 * Synchronous SharedPreferences read so the use case never blocks on
 * DataStore; enabling only adds backend reads with Room fallback and
 * never removes ANP offline paths.
 */
@Singleton
class CommunityReadsFlagStore @Inject constructor(
    @ApplicationContext context: Context,
) : CommunityReadsFlagProvider {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun isEnabled(): Boolean =
        prefs.getBoolean(KEY_ENABLED, true)

    fun setEnabled(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_ENABLED, enabled).apply()
    }

    companion object {
        const val PREFS_NAME = "community_reads"
        const val KEY_ENABLED = "community_reads_enabled"
    }
}
