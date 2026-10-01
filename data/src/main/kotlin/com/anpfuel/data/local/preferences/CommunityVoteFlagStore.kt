package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.port.CommunityVoteFlagProvider
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P10-T07 community-vote flag (default OFF).
 *
 * Synchronous SharedPreferences read so the use case never blocks on
 * DataStore; flag OFF hides confirm/dispute actions while submitted
 * records stay preserved for review (rollback is flag OFF).
 */
@Singleton
class CommunityVoteFlagStore @Inject constructor(
    @ApplicationContext context: Context,
) : CommunityVoteFlagProvider {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun isEnabled(): Boolean =
        prefs.getBoolean(KEY_ENABLED, false)

    fun setEnabled(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_ENABLED, enabled).apply()
    }

    companion object {
        const val PREFS_NAME = "community_vote"
        const val KEY_ENABLED = "community_vote_enabled"
    }
}
