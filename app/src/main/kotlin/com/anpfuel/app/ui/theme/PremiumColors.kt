package com.anpfuel.app.ui.theme

import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

/** Fixed Premium accent that keeps its identity across dynamic color schemes. */
data class PremiumPalette(
    val container: Color,
    val content: Color,
    val accent: Color,
    val border: Color,
)

object PremiumColors {

    const val MIN_ACCENT_CONTRAST_RATIO = 3.0

    internal val Light = PremiumPalette(
        container = Color(0xFFFFF6D6),
        content = Color(0xFF4A3600),
        accent = Color(0xFF7A5A00),
        border = Color(0xFFB07D00),
    )

    internal val Dark = PremiumPalette(
        container = Color(0xFF3A2E0A),
        content = Color(0xFFFFE6A3),
        accent = Color(0xFFFFD54F),
        border = Color(0xFFB08A2E),
    )

    fun palette(darkTheme: Boolean): PremiumPalette = if (darkTheme) Dark else Light

    @Composable
    fun current(): PremiumPalette = palette(LocalAnpDarkTheme.current)
}
