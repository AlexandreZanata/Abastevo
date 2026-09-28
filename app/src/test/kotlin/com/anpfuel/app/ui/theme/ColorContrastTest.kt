package com.anpfuel.app.ui.theme

import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ColorContrastTest {

    private val lightScheme = lightColorScheme(
        primary = AnpBlue,
        onPrimary = ColorTokens.White,
        primaryContainer = ColorTokens.BlueContainerLight,
        onPrimaryContainer = AnpBlueDark,
        secondary = AnpGreen,
        onSecondary = ColorTokens.White,
        secondaryContainer = ColorTokens.GreenContainerLight,
        onSecondaryContainer = AnpGreenDark,
    )

    private val darkScheme = darkColorScheme(
        primary = ColorTokens.BlueLight,
        onPrimary = AnpBlueDark,
        primaryContainer = AnpBlueDark,
        onPrimaryContainer = ColorTokens.BlueContainerLight,
        secondary = ColorTokens.GreenLight,
        onSecondary = AnpGreenDark,
        secondaryContainer = AnpGreenDark,
        onSecondaryContainer = ColorTokens.GreenContainerLight,
    )

    @Test
    fun lightThemeTextPairsMeetWcagAa() {
        assertAaNormalText(lightScheme.onPrimary, lightScheme.primary)
        assertAaNormalText(lightScheme.onPrimaryContainer, lightScheme.primaryContainer)
        assertAaNormalText(lightScheme.onSecondary, lightScheme.secondary)
        assertAaNormalText(lightScheme.onSecondaryContainer, lightScheme.secondaryContainer)
        assertAaNormalText(lightScheme.onBackground, lightScheme.background)
        assertAaNormalText(lightScheme.onSurface, lightScheme.surface)
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
        assertAaNormalText(darkScheme.onError, darkScheme.error)
    }

    private fun assertAaNormalText(foreground: androidx.compose.ui.graphics.Color, background: androidx.compose.ui.graphics.Color) {
        val ratio = ColorContrast.contrastRatio(foreground, background)
        assertTrue(
            ratio >= ColorContrast.AA_NORMAL_TEXT_MIN_RATIO,
            "Expected WCAG AA contrast >= ${ColorContrast.AA_NORMAL_TEXT_MIN_RATIO} but was $ratio",
        )
    }
}
