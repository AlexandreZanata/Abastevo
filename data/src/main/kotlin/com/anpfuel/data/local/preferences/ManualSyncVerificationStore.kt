package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * B-BR-008 freshness: remembers the last manual sync that confirmed ANP
 * has no newer week yet (SyncJobOutcome.NO_NEW_DATA).
 *
 * Global per SurveyWeek, not per city: one UC-001 sync updates every
 * municipality at once, so switching SORRISO <> FLORIANOPOLIS must show
 * the same "verified at" state while the displayed week is unchanged.
 * Cleared when the displayed week changes (new data) or a fresh sync
 * succeeds.
 */
@Singleton
class ManualSyncVerificationStore @Inject constructor(
    @ApplicationContext context: Context,
) {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    data class Verification(
        val weekStartIso: String,
        val weekEndIso: String,
        val verifiedAtMillis: Long,
    )

    fun get(): Verification? {
        val start = prefs.getString(KEY_WEEK_START, null) ?: return null
        val end = prefs.getString(KEY_WEEK_END, null) ?: return null
        val at = prefs.getLong(KEY_VERIFIED_AT, 0L)
        if (at <= 0L) return null
        return Verification(start, end, at)
    }

    fun saveVerified(weekStartIso: String, weekEndIso: String, verifiedAtMillis: Long) {
        prefs.edit()
            .putString(KEY_WEEK_START, weekStartIso)
            .putString(KEY_WEEK_END, weekEndIso)
            .putLong(KEY_VERIFIED_AT, verifiedAtMillis)
            .apply()
    }

    fun clear() {
        prefs.edit()
            .remove(KEY_WEEK_START)
            .remove(KEY_WEEK_END)
            .remove(KEY_VERIFIED_AT)
            .apply()
    }

    companion object {
        const val PREFS_NAME = "manual_sync_verification"
        const val KEY_WEEK_START = "verified_week_start_iso"
        const val KEY_WEEK_END = "verified_week_end_iso"
        const val KEY_VERIFIED_AT = "verified_at_millis"
    }
}
