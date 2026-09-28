package com.anpfuel.app.ui.theme

import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class FuelProductTintContrastTest {

    @Test
    fun lightThemeFuelTintsMeetWcagAaLargeTextOnCardSurface() {
        val background = ColorTokens.SurfaceContainerLowLight
        FuelProduct.entries.forEach { product ->
            val tint = FuelProductTint.colorFor(product, darkTheme = false)
            assertAaLargeText(
                foreground = tint,
                background = background,
                label = "${product.name} light tint",
            )
        }
    }

    @Test
    fun darkThemeFuelTintsMeetWcagAaLargeTextOnCardSurface() {
        val background = ColorTokens.SurfaceContainerLowDark
        FuelProduct.entries.forEach { product ->
            val tint = FuelProductTint.colorFor(product, darkTheme = true)
            assertAaLargeText(
                foreground = tint,
                background = background,
                label = "${product.name} dark tint",
            )
        }
    }

    private fun assertAaLargeText(
        foreground: androidx.compose.ui.graphics.Color,
        background: androidx.compose.ui.graphics.Color,
        label: String,
    ) {
        val ratio = ColorContrast.contrastRatio(foreground, background)
        assertTrue(
            ratio >= ColorContrast.AA_LARGE_TEXT_MIN_RATIO,
            "$label: expected WCAG AA large-text contrast >= " +
                "${ColorContrast.AA_LARGE_TEXT_MIN_RATIO} but was $ratio",
        )
    }
}
