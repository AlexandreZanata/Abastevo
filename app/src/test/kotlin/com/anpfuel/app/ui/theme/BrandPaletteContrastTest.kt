package com.anpfuel.app.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.ui.graphics.Color
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class BrandPaletteContrastTest {

    @Test
    fun `light cards and surfaces are pure white`() {
        assertEquals(Color.White, LightColorScheme.surface)
        assertEquals(Color.White, LightColorScheme.surfaceContainerLowest)
    }

    @Test
    fun `dark screens are true black`() {
        assertEquals(Color.Black, DarkColorScheme.background)
        assertEquals(Color.Black, DarkColorScheme.surface)
    }

    @Test
    fun `foundation is monochrome in both themes`() {
        listOf(LightColorScheme, DarkColorScheme).forEach { scheme ->
            listOf(
                scheme.background,
                scheme.onBackground,
                scheme.surface,
                scheme.onSurface,
                scheme.surfaceVariant,
                scheme.onSurfaceVariant,
                scheme.outline,
                scheme.outlineVariant,
                scheme.surfaceContainerLow,
                scheme.surfaceContainer,
                scheme.surfaceContainerHigh,
                scheme.surfaceContainerHighest,
                scheme.primary,
                scheme.onPrimary,
                scheme.primaryContainer,
                scheme.onPrimaryContainer,
            ).forEach { assertNeutral(it) }
        }
    }

    @Test
    fun `filled actions invert with the theme`() {
        assertEquals(LightColorScheme.onSurface, AbastevoActionDefaults.Light.container)
        assertEquals(Color.White, AbastevoActionDefaults.Light.content)
        assertEquals(Color.White, AbastevoActionDefaults.Dark.container)
        assertEquals(LightColorScheme.onSurface, AbastevoActionDefaults.Dark.content)
    }

    @Test
    fun `the accent is the only brand color and never the primary`() {
        listOf(LightColorScheme to AbastevoAccent.Light, DarkColorScheme to AbastevoAccent.Dark).forEach { (scheme, tones) ->
            assertEquals(tones.accent, scheme.secondary)
            assertEquals(tones.container, scheme.secondaryContainer)
            assertNeutral(scheme.primary)
            // Green: the accent keeps the community meaning it had before.
            assertTrue(tones.accent.green > tones.accent.red)
            assertTrue(tones.accent.green > tones.accent.blue)
        }
        assertEquals(AbastevoAccent.Light.accent, AbastevoNavDefaults.Light.indicator)
        assertEquals(AbastevoAccent.Dark.accent, AbastevoNavDefaults.Dark.indicator)
    }

    @Test
    fun `light palette meets contrast targets`() {
        assertPalette(LightColorScheme, AbastevoNavDefaults.Light, AbastevoActionDefaults.Light)
    }

    @Test
    fun `dark palette meets contrast targets`() {
        assertPalette(DarkColorScheme, AbastevoNavDefaults.Dark, AbastevoActionDefaults.Dark)
    }

    private fun assertPalette(scheme: ColorScheme, nav: AbastevoNavColors, action: AbastevoActionColors) {
        val screensAndCards = listOf(
            scheme.background,
            scheme.surfaceContainerLow,
            scheme.surfaceContainer,
            scheme.surfaceContainerHigh,
        )
        // Filled buttons on screens and on every card level.
        assertText(action.content, action.container, "filled button label")
        screensAndCards.forEach { assertNonText(action.container, it, "filled button fill") }
        // Selected chips and segmented buttons, outlined button borders.
        assertText(scheme.onPrimary, scheme.primary, "selected option label")
        screensAndCards.forEach { assertNonText(scheme.primary, it, "outlined button border") }
        // Text links and accent icons on screens and cards.
        screensAndCards.forEach { assertText(scheme.secondary, it, "accent link") }
        assertText(scheme.onSecondary, scheme.secondary, "accent label")
        // Tonal containers.
        assertText(scheme.onPrimaryContainer, scheme.primaryContainer, "primary container")
        assertText(scheme.onSecondaryContainer, scheme.secondaryContainer, "accent container")
        assertText(scheme.onTertiary, scheme.tertiary, "tertiary label")
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
        // Bottom bar: flat on the screen color, accent marker over the selected tab.
        assertEquals(scheme.background, nav.container)
        assertText(nav.selectedLabel, nav.container, "selected label")
        assertText(nav.unselected, nav.container, "unselected label")
        assertNonText(nav.selectedIcon, nav.container, "selected icon")
        assertNonText(nav.indicator, nav.container, "selection marker")
    }

    private fun assertNeutral(color: Color) {
        assertEquals(color.red, color.green, "$color is not neutral")
        assertEquals(color.green, color.blue, "$color is not neutral")
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
