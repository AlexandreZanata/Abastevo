package com.anpfuel.app.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.em
import androidx.compose.ui.unit.sp
import com.anpfuel.app.R

/**
 * Landing display family (`--font-display`): titles, prices, buttons and labels.
 * Bundled OFL files; provenance in `docs/mobile/DESIGN_SYSTEM.md`.
 */
val ManropeFontFamily = FontFamily(
    Font(R.font.manrope_medium, FontWeight.Medium),
    Font(R.font.manrope_semibold, FontWeight.SemiBold),
    Font(R.font.manrope_bold, FontWeight.Bold),
    Font(R.font.manrope_extrabold, FontWeight.ExtraBold),
)

/** Landing text family (`--font-sans`): body copy and supporting text. */
val SourceSans3FontFamily = FontFamily(
    Font(R.font.source_sans_3_regular, FontWeight.Normal),
    Font(R.font.source_sans_3_semibold, FontWeight.SemiBold),
    Font(R.font.source_sans_3_bold, FontWeight.Bold),
)

private fun display(weight: FontWeight, size: Int, lineHeight: Int, tracking: Float) = TextStyle(
    fontFamily = ManropeFontFamily,
    fontWeight = weight,
    fontSize = size.sp,
    lineHeight = lineHeight.sp,
    letterSpacing = tracking.em,
)

private fun body(size: Int, lineHeight: Int) = TextStyle(
    fontFamily = SourceSans3FontFamily,
    fontWeight = FontWeight.Normal,
    fontSize = size.sp,
    lineHeight = lineHeight.sp,
    letterSpacing = 0.em,
)

/**
 * Landing type scale on the Material roles: Manrope for display, headline,
 * title and label; Source Sans 3 for body. Body sizes sit 1sp above the
 * Material defaults because Source Sans 3 has a smaller x-height than Roboto.
 */
val Typography = Typography(
    displayLarge = display(FontWeight.ExtraBold, size = 52, lineHeight = 60, tracking = -0.02f),
    displayMedium = display(FontWeight.ExtraBold, size = 44, lineHeight = 52, tracking = -0.02f),
    displaySmall = display(FontWeight.ExtraBold, size = 34, lineHeight = 42, tracking = -0.02f),
    headlineLarge = display(FontWeight.Bold, size = 30, lineHeight = 38, tracking = -0.02f),
    headlineMedium = display(FontWeight.Bold, size = 26, lineHeight = 34, tracking = -0.015f),
    headlineSmall = display(FontWeight.Bold, size = 22, lineHeight = 30, tracking = -0.015f),
    titleLarge = display(FontWeight.Bold, size = 20, lineHeight = 28, tracking = -0.01f),
    titleMedium = display(FontWeight.SemiBold, size = 16, lineHeight = 24, tracking = 0f),
    titleSmall = display(FontWeight.SemiBold, size = 14, lineHeight = 20, tracking = 0f),
    bodyLarge = body(size = 17, lineHeight = 26),
    bodyMedium = body(size = 15, lineHeight = 22),
    bodySmall = body(size = 13, lineHeight = 18),
    labelLarge = display(FontWeight.Bold, size = 15, lineHeight = 20, tracking = 0f),
    labelMedium = display(FontWeight.SemiBold, size = 12, lineHeight = 16, tracking = 0.01f),
    labelSmall = display(FontWeight.SemiBold, size = 11, lineHeight = 16, tracking = 0.02f),
)
