package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.port.AnonymousContributionFlagProvider
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P10-T03 anonymous-contribution flag (default OFF).
 *
 * Synchronous SharedPreferences read so the flow never blocks; enabling
 * only unlocks the device-key proof path and never forces login nor
 * migrates existing local data.
 */
@Singleton
class AnonymousContributionFlagStore @Inject constructor(
    @ApplicationContext context: Context,
) : AnonymousContributionFlagProvider {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun isEnabled(): Boolean =
        prefs.getBoolean(KEY_ENABLED, false)

    fun setEnabled(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_ENABLED, enabled).apply()
    }

    companion object {
        const val PREFS_NAME = "anonymous_contribution"
        const val KEY_ENABLED = "anonymous_contribution_enabled"
    }
}
