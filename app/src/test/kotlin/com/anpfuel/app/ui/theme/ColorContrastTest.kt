package com.anpfuel.app.ui.theme

import androidx.compose.ui.graphics.Color
import com.anpfuel.app.ui.home.HomeHeroColors
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ColorContrastTest {

    private val lightScheme = LightColorScheme

    private val darkScheme = DarkColorScheme

    @Test
    fun lightThemeTextPairsMeetWcagAa() {
        assertAaNormalText(lightScheme.onPrimary, lightScheme.primary)
        assertAaNormalText(lightScheme.onPrimaryContainer, lightScheme.primaryContainer)
        assertAaNormalText(lightScheme.onSecondary, lightScheme.secondary)
        assertAaNormalText(lightScheme.onSecondaryContainer, lightScheme.secondaryContainer)
        assertAaNormalText(lightScheme.onBackground, lightScheme.background)
        assertAaNormalText(lightScheme.onSurface, lightScheme.surface)
        assertAaNormalText(lightScheme.onSurface, lightScheme.surfaceContainer)
        assertAaNormalText(lightScheme.onSurfaceVariant, lightScheme.surfaceContainer)
        assertAaNormalText(lightScheme.onError, lightScheme.error)
    }

    @Test
    fun darkThemeTextPairsMeetWcagAa() {
        assertAaNormalText(darkScheme.onPrimary, darkScheme.primary)
        assertAaNormalText(darkScheme.onPrimaryContainer, darkScheme.primaryContainer)
        assertAaNormalText(darkScheme.onSecondary, darkScheme.secondary)
        assertAaNormalText(darkScheme.onSecondaryContainer, darkScheme.secondaryContainer)
        assertAaNormalText(darkScheme.onBackground, darkScheme.background)
        assertAaNormalText(darkScheme.onSurface, darkScheme.surface)
        assertAaNormalText(darkScheme.onSurface, darkScheme.surfaceContainer)
        assertAaNormalText(darkScheme.onSurfaceVariant, darkScheme.surfaceContainer)
        assertAaNormalText(darkScheme.onError, darkScheme.error)
    }

    private fun assertAaNormalText(foreground: androidx.compose.ui.graphics.Color, background: androidx.compose.ui.graphics.Color) {
        val ratio = ColorContrast.contrastRatio(foreground, background)
        assertTrue(
            ratio >= ColorContrast.AA_NORMAL_TEXT_MIN_RATIO,
            "Expected WCAG AA contrast >= ${ColorContrast.AA_NORMAL_TEXT_MIN_RATIO} but was $ratio",
        )
    }

    @Test
    fun homeHeroTextAndActionMeetWcagAa() {
        assertAaNormalText(Color.White, HomeHeroColors.Navy)
        assertAaNormalText(HomeHeroColors.Mint, HomeHeroColors.Navy)
        assertAaNormalText(HomeHeroColors.OnMint, HomeHeroColors.Mint)
    }
}
