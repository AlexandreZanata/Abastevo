package com.anpfuel.app.ui.theme

import androidx.compose.ui.unit.TextUnitType
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class TypographyTest {

    private val displayRoles = with(Typography) {
        listOf(
            displayLarge, displayMedium, displaySmall,
            headlineLarge, headlineMedium, headlineSmall,
            titleLarge, titleMedium, titleSmall,
            labelLarge, labelMedium, labelSmall,
        )
    }

    private val bodyRoles = with(Typography) { listOf(bodyLarge, bodyMedium, bodySmall) }

    @Test
    fun `titles prices and labels use the landing display family`() {
        displayRoles.forEach { assertEquals(ManropeFontFamily, it.fontFamily) }
    }

    @Test
    fun `body copy uses the landing text family`() {
        bodyRoles.forEach { assertEquals(SourceSans3FontFamily, it.fontFamily) }
    }

    @Test
    fun `every role scales with the user font size and leaves room for accents`() {
        (displayRoles + bodyRoles).forEach { style ->
            assertEquals(TextUnitType.Sp, style.fontSize.type)
            assertEquals(TextUnitType.Sp, style.lineHeight.type)
            assertTrue(style.lineHeight.value >= style.fontSize.value * MIN_LINE_HEIGHT_RATIO)
        }
    }

    @Test
    fun `smallest text stays readable`() {
        (displayRoles + bodyRoles).forEach { assertTrue(it.fontSize.value >= MIN_FONT_SIZE_SP) }
    }

    private companion object {
        const val MIN_LINE_HEIGHT_RATIO = 1.15f
        const val MIN_FONT_SIZE_SP = 11f
    }
}
