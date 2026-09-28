package com.anpfuel.app.ui.theme

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance

/**
 * WCAG 2.1 contrast helpers for theme validation (Phase 9.3.3).
 */
object ColorContrast {

    const val AA_NORMAL_TEXT_MIN_RATIO = 4.5
    const val AA_LARGE_TEXT_MIN_RATIO = 3.0

    fun contrastRatio(foreground: Color, background: Color): Double {
        val foregroundLuminance = foreground.luminance()
        val backgroundLuminance = background.luminance()
        val lighter = maxOf(foregroundLuminance, backgroundLuminance)
        val darker = minOf(foregroundLuminance, backgroundLuminance)
        return (lighter + 0.05) / (darker + 0.05)
    }

    fun meetsAaNormalText(foreground: Color, background: Color): Boolean =
        contrastRatio(foreground, background) >= AA_NORMAL_TEXT_MIN_RATIO

    fun meetsAaLargeText(foreground: Color, background: Color): Boolean =
        contrastRatio(foreground, background) >= AA_LARGE_TEXT_MIN_RATIO
}
