package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.port.CaptureOcrFlagProvider
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P10-T04 capture/OCR flag (default ON since the Community contribute
 * entry opens the camera module: enabling only unlocks local capture +
 * OCR candidates and never uploads, auto-picks a price or migrates
 * existing local data).
 *
 * Synchronous SharedPreferences read so the flow never blocks; enabling
 * only unlocks local capture + OCR candidates and never uploads,
 * auto-picks a price or migrates existing local data.
 */
@Singleton
class CaptureOcrFlagStore @Inject constructor(
    @ApplicationContext context: Context,
) : CaptureOcrFlagProvider {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun isEnabled(): Boolean =
        prefs.getBoolean(KEY_ENABLED, true)

    fun setEnabled(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_ENABLED, enabled).apply()
    }

    companion object {
        const val PREFS_NAME = "capture_ocr"
        const val KEY_ENABLED = "capture_ocr_enabled"
    }
}
