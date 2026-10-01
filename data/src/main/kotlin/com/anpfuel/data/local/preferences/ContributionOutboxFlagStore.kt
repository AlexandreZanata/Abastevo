package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.port.ContributionOutboxFlagProvider
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P10-T05 contribution-outbox flag (default OFF).
 *
 * Synchronous SharedPreferences read so enqueue never blocks; enabling
 * only unlocks durable outbox dispatch and never removes ANP offline
 * paths or deletes accepted history.
 */
@Singleton
class ContributionOutboxFlagStore @Inject constructor(
    @ApplicationContext context: Context,
) : ContributionOutboxFlagProvider {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun isEnabled(): Boolean =
        prefs.getBoolean(KEY_ENABLED, false)

    fun setEnabled(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_ENABLED, enabled).apply()
    }

    companion object {
        const val PREFS_NAME = "contribution_outbox"
        const val KEY_ENABLED = "contribution_outbox_enabled"
    }
}
