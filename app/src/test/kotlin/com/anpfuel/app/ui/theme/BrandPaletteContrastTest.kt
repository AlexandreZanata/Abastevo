package com.anpfuel.app.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class BrandPaletteContrastTest {

    @Test
    fun `every light container is white on off-white screens`() {
        listOf(
            LightColorScheme.surface,
            LightColorScheme.surfaceContainerLowest,
            LightColorScheme.surfaceContainerLow,
            LightColorScheme.surfaceContainer,
            LightColorScheme.surfaceContainerHigh,
            LightColorScheme.surfaceContainerHighest,
            LightColorScheme.primaryContainer,
        ).forEach { assertEquals(Color.White, it) }
        assertTrue(LightColorScheme.background.luminance() < Color.White.luminance())
        assertTrue(LightColorScheme.background.luminance() > LIGHT_SCREEN_MIN_LUMINANCE)
    }

    @Test
    fun `dark screens are near black with lighter cards`() {
        assertEquals(DarkColorScheme.surfaceContainerLow, DarkColorScheme.surface)
        assertTrue(DarkColorScheme.background.luminance() < DARK_SCREEN_MAX_LUMINANCE)
        assertTrue(DarkColorScheme.surfaceContainerLow.luminance() > DarkColorScheme.background.luminance())
    }

    @Test
    fun `foundation is near-neutral in both themes`() {
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
        assertEquals(DarkColorScheme.onSurface, AbastevoActionDefaults.Dark.container)
        assertEquals(LightColorScheme.onSurface, AbastevoActionDefaults.Dark.content)
    }

    @Test
    fun `chrome is solid black in both themes`() {
        assertEquals(Color.Black, AbastevoChromeDefaults.Light.container)
        assertEquals(Color.Black, AbastevoChromeDefaults.Dark.container)
        assertEquals(LightColorScheme.primary, AbastevoChromeDefaults.Light.container)
        listOf(AbastevoChromeDefaults.Light, AbastevoChromeDefaults.Dark).forEach { chrome ->
            assertNearNeutral(chrome.container)
            assertNearNeutral(chrome.hero)
            assertTrue(chrome.container.luminance() < CHROME_MAX_LUMINANCE)
            assertEquals(chrome.container, AbastevoNavDefaults.fromChrome(chrome).container)
            assertEquals(chrome.selected, AbastevoNavDefaults.fromChrome(chrome).indicator)
        }
        // The dark hero card lifts off the near-black screen and its cards.
        assertTrue(AbastevoChromeDefaults.Dark.hero.luminance() > DarkColorScheme.surfaceContainerLow.luminance())
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
        // On the dark chrome the accent takes its bright tone in both themes.
        assertEquals(AbastevoAccent.Dark.accent, AbastevoChromeDefaults.Light.accent)
        assertEquals(AbastevoAccent.Dark.accent, AbastevoChromeDefaults.Dark.accent)
    }

    @Test
    fun `light palette meets contrast targets`() {
        assertPalette(LightColorScheme, AbastevoChromeDefaults.Light, AbastevoActionDefaults.Light)
    }

    @Test
    fun `dark palette meets contrast targets`() {
        assertPalette(DarkColorScheme, AbastevoChromeDefaults.Dark, AbastevoActionDefaults.Dark)
    }

    private fun assertPalette(scheme: ColorScheme, chrome: AbastevoChromeColors, action: AbastevoActionColors) {
        val nav = AbastevoNavDefaults.fromChrome(chrome)
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
        // Header and top bars: titles, field text and icons on the chrome.
        assertText(chrome.content, chrome.container, "chrome title")
        assertText(chrome.contentMuted, chrome.container, "chrome secondary text")
        assertText(chrome.content, chrome.control, "chrome field text")
        assertNonText(chrome.accent, chrome.container, "chrome location pin")
        assertNonText(chrome.accent, chrome.control, "chrome field pin")
        assertText(chrome.onSelected, chrome.selected, "chrome chip label")
        // Bottom bar: solid chrome, light pill behind the selected tab.
        assertText(nav.selectedLabel, nav.indicator, "selected label")
        assertNonText(nav.selectedIcon, nav.indicator, "selected icon")
        assertText(nav.unselected, nav.container, "unselected label")
        assertNonText(nav.indicator, nav.container, "selection pill")
        // Hero card: chrome content and the light action on the hero fill.
        assertText(chrome.content, chrome.hero, "hero title")
        assertText(chrome.contentMuted, chrome.hero, "hero secondary text")
        assertNonText(chrome.selected, chrome.hero, "hero action fill")
    }

    /** Graphite and off-white may lean a few steps warm, never into a hue. */
    private fun assertNeutral(color: Color) = assertNearNeutral(color)

    private fun assertNearNeutral(color: Color) {
        val channels = listOf(color.red, color.green, color.blue)
        assertTrue(channels.max() - channels.min() <= MAX_CHANNEL_SPREAD, "$color is not near-neutral")
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
        const val MAX_CHANNEL_SPREAD = 6f / 255f
        const val LIGHT_SCREEN_MIN_LUMINANCE = 0.85f
        const val DARK_SCREEN_MAX_LUMINANCE = 0.01f
        const val CHROME_MAX_LUMINANCE = 0.02f
    }
}
