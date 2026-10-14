package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.port.CommunityHelpfulnessFlagProvider
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Community-first helpfulness flag (default ON; an explicit OFF remains respected).
 *
 * Synchronous SharedPreferences read so the use case never blocks on
 * DataStore; flag OFF hides helpful/unhelpful actions while recorded
 * votes stay preserved server-side for review (rollback is flag OFF).
 */
@Singleton
class CommunityHelpfulnessFlagStore @Inject constructor(
    @ApplicationContext context: Context,
) : CommunityHelpfulnessFlagProvider {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun isEnabled(): Boolean =
        prefs.getBoolean(KEY_ENABLED, true)

    fun setEnabled(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_ENABLED, enabled).apply()
    }

    companion object {
        const val PREFS_NAME = "community_helpfulness"
        const val KEY_ENABLED = "community_helpfulness_enabled"
    }
}
