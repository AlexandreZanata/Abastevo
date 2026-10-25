package com.anpfuel.app.ui.theme

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PremiumColorsTest {

    @Test
    fun `light premium text and accents stay legible on the soft yellow fill`() {
        assertPalette(PremiumColors.Light)
    }

    @Test
    fun `dark premium text and accents stay legible on the dark gold fill`() {
        assertPalette(PremiumColors.Dark)
    }

    @Test
    fun `fixed premium accent does not depend on dynamic color tokens`() {
        assertTrue(PremiumColors.Light.container != PremiumColors.Dark.container)
        assertTrue(PremiumColors.Light.accent != PremiumColors.Dark.accent)
    }

    private fun assertPalette(palette: PremiumPalette) {
        val textRatio = ColorContrast.contrastRatio(palette.content, palette.container)
        assertTrue(
            textRatio >= ColorContrast.AA_NORMAL_TEXT_MIN_RATIO,
            "premium content contrast was $textRatio",
        )
        val accentRatio = ColorContrast.contrastRatio(palette.accent, palette.container)
        assertTrue(
            accentRatio >= PremiumColors.MIN_ACCENT_CONTRAST_RATIO,
            "premium accent contrast was $accentRatio",
        )
        val borderRatio = ColorContrast.contrastRatio(palette.border, palette.container)
        assertTrue(
            borderRatio >= PremiumColors.MIN_ACCENT_CONTRAST_RATIO,
            "premium border contrast was $borderRatio",
        )
    }
}
