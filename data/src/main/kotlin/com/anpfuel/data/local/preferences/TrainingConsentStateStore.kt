package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.port.TrainingConsentLocalStore
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * CS11A training-consent local state (default OFF).
 *
 * Synchronous SharedPreferences reads so the use case never blocks;
 * the switch preference, the per-device monotonic revision and the
 * queued offline revocation persist across restarts. The server stays
 * authoritative: every foreground reloads the owner status and the
 * pending revocation uploads on the next connection.
 */
@Singleton
class TrainingConsentStateStore @Inject constructor(
    @ApplicationContext context: Context,
) : TrainingConsentLocalStore {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun isToggleOn(): Boolean =
        prefs.getBoolean(KEY_TOGGLE, false)

    override fun setToggleOn(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_TOGGLE, enabled).apply()
    }

    @Synchronized
    override fun nextRevision(): Long {
        val next = prefs.getLong(KEY_REVISION, 0) + 1
        prefs.edit().putLong(KEY_REVISION, next).apply()
        return next
    }

    override fun hasPendingRevocation(): Boolean =
        prefs.getBoolean(KEY_PENDING, false)

    override fun setPendingRevocation(pending: Boolean) {
        prefs.edit().putBoolean(KEY_PENDING, pending).apply()
    }

    companion object {
        const val PREFS_NAME = "training_consent"
        const val KEY_TOGGLE = "training_consent_toggle"
        const val KEY_REVISION = "training_consent_revision"
        const val KEY_PENDING = "training_consent_pending_revocation"
    }
}
