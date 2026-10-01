package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.port.FeedbackFlagProvider
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P17-T02 feedback flag (default OFF).
 *
 * Synchronous SharedPreferences read so the use cases never block;
 * flag OFF hides rating/comment/vote/report actions while queued
 * outbox ops stay preserved for retry (rollback is flag OFF).
 */
@Singleton
class FeedbackFlagStore @Inject constructor(
    @ApplicationContext context: Context,
) : FeedbackFlagProvider {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun isEnabled(): Boolean =
        prefs.getBoolean(KEY_ENABLED, false)

    fun setEnabled(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_ENABLED, enabled).apply()
    }

    companion object {
        const val PREFS_NAME = "feedback"
        const val KEY_ENABLED = "feedback_enabled"
    }
}
