package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Remembers whether the driver dismissed the home benefit shortcuts.
 *
 * Synchronous SharedPreferences flag so Home can honour the dismissal
 * before the first frame, without flashing the shortcuts on reopen.
 */
@Singleton
class HomeBenefitsVisibilityStore @Inject constructor(
    @ApplicationContext context: Context,
) {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    fun isVisible(): Boolean =
        prefs.getBoolean(KEY_BENEFITS_VISIBLE, true)

    fun setVisible(visible: Boolean) {
        prefs.edit().putBoolean(KEY_BENEFITS_VISIBLE, visible).apply()
    }

    companion object {
        const val PREFS_NAME = "home"
        const val KEY_BENEFITS_VISIBLE = "home_benefits_visible"
    }
}
