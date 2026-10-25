package com.anpfuel.app.ui.theme

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class AnpContainerDefaultsTest {

    private val lightFill = ColorTokens.SurfaceContainerLowLight
    private val lightOutlineVariant = Color(0xFFCAC4D0)
    private val lightOutline = Color(0xFF79747E)
    private val darkFill = ColorTokens.SurfaceContainerLowDark
    private val darkOutlineVariant = Color(0xFF49454F)
    private val darkOutline = Color(0xFF938F99)

    @Test
    fun `light card border is darker than the fill and meets the minimum`() {
        val border = AnpContainerDefaults.borderColor(
            containerColor = lightFill,
            outlineVariant = lightOutlineVariant,
            outline = lightOutline,
        )

        assertMinimumContrast(lightFill, border)
        assertTrue(border.luminance() < lightFill.luminance(), "border must darken the light card")
        assertEquals(1f, border.alpha)
    }

    @Test
    fun `dark card border is lighter than the fill and meets the minimum`() {
        val border = AnpContainerDefaults.borderColor(
            containerColor = darkFill,
            outlineVariant = darkOutlineVariant,
            outline = darkOutline,
        )

        assertMinimumContrast(darkFill, border)
        assertTrue(border.luminance() > darkFill.luminance(), "border must lighten the dark card")
        assertEquals(1f, border.alpha)
    }

    @Test
    fun `outline variant is kept when it already meets the minimum`() {
        val border = AnpContainerDefaults.borderColor(
            containerColor = Color.Black,
            outlineVariant = lightOutlineVariant,
            outline = lightOutline,
        )

        assertEquals(lightOutlineVariant, border)
    }

    @Test
    fun `highlighted light containers get a compatible outline`() {
        val fill = Color(0xFFBBDEFB)

        val border = AnpContainerDefaults.borderColor(
            containerColor = fill,
            outlineVariant = lightOutlineVariant,
            outline = lightOutline,
        )

        assertMinimumContrast(fill, border)
        assertTrue(border.luminance() < fill.luminance())
    }

    @Test
    fun `highlighted dark containers get a lighter compatible outline`() {
        val fill = Color(0xFF0D47A1)

        val border = AnpContainerDefaults.borderColor(
            containerColor = fill,
            outlineVariant = darkOutlineVariant,
            outline = darkOutline,
        )

        assertMinimumContrast(fill, border)
        assertTrue(border.luminance() > fill.luminance())
    }

    private fun assertMinimumContrast(fill: Color, border: Color) {
        val ratio = ColorContrast.contrastRatio(border, fill)
        assertTrue(
            ratio >= AnpContainerDefaults.MIN_BORDER_CONTRAST_RATIO,
            "expected border contrast >= ${AnpContainerDefaults.MIN_BORDER_CONTRAST_RATIO} but was $ratio",
        )
    }
}
