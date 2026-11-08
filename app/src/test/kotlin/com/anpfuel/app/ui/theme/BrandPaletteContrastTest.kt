package com.anpfuel.app.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.ui.graphics.Color
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class BrandPaletteContrastTest {

    @Test
    fun `light screens and surfaces are pure white`() {
        assertEquals(Color.White, LightColorScheme.background)
        assertEquals(Color.White, LightColorScheme.surface)
        assertEquals(Color.White, LightColorScheme.surfaceContainerLowest)
    }

    @Test
    fun `light palette meets contrast targets`() {
        assertPalette(LightColorScheme, AbastevoNavDefaults.Light)
    }

    @Test
    fun `dark palette meets contrast targets`() {
        assertPalette(DarkColorScheme, AbastevoNavDefaults.Dark)
    }

    @Test
    fun `dark highlights stay on the teal brand hue`() {
        assertEquals(DarkColorScheme.primary, AbastevoNavDefaults.Dark.indicator)
        assertEquals(DarkColorScheme.onPrimary, AbastevoNavDefaults.Dark.selectedIcon)
        assertTrue(DarkColorScheme.primary.green > DarkColorScheme.primary.red)
        assertTrue(DarkColorScheme.secondaryContainer.blue > DarkColorScheme.secondaryContainer.red)
    }

    private fun assertPalette(scheme: ColorScheme, nav: AbastevoNavColors) {
        // Actions and links on screens and cards.
        assertText(scheme.onPrimary, scheme.primary, "button label")
        assertText(scheme.primary, scheme.background, "link on screen")
        assertText(scheme.primary, scheme.surfaceContainerLow, "link on card")
        assertText(scheme.onSecondary, scheme.secondary, "secondary label")
        // Tonal containers.
        assertText(scheme.onPrimaryContainer, scheme.primaryContainer, "primary container")
        assertText(scheme.onSecondaryContainer, scheme.secondaryContainer, "selected chip")
        assertText(scheme.onTertiaryContainer, scheme.tertiaryContainer, "tertiary container")
        assertText(scheme.onErrorContainer, scheme.errorContainer, "error container")
        // Body and secondary text on every surface level.
        assertText(scheme.onSurface, scheme.background, "body text")
        listOf(
            scheme.surfaceContainerLow,
            scheme.surfaceContainer,
            scheme.surfaceContainerHigh,
            scheme.surfaceContainerHighest,
            scheme.surfaceVariant,
        ).forEach { assertText(scheme.onSurfaceVariant, it, "secondary text") }
        assertNonText(scheme.outline, scheme.background, "outline")
        // Bottom bar.
        assertText(nav.selectedLabel, nav.container, "selected label")
        assertText(nav.unselected, nav.container, "unselected label")
        assertNonText(nav.selectedIcon, nav.indicator, "selected icon")
        assertNonText(nav.indicator, nav.container, "selection pill")
    }

    private fun assertText(foreground: Color, background: Color, label: String) {
        val ratio = ColorContrast.contrastRatio(foreground, background)
        assertTrue(ratio >= ColorContrast.AA_NORMAL_TEXT_MIN_RATIO, "$label contrast was $ratio")
    }

    private fun assertNonText(foreground: Color, background: Color, label: String) {
        val ratio = ColorContrast.contrastRatio(foreground, background)
        assertTrue(ratio >= NON_TEXT_MIN_RATIO, "$label contrast was $ratio")
    }

    private companion object {
        const val NON_TEXT_MIN_RATIO = 3.0
    }
}
