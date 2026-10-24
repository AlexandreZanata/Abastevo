package com.anpfuel.app.ui.theme

import androidx.compose.foundation.BorderStroke
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.compositeOver
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.unit.dp

/**
 * Shared neutral outline for content containers.
 *
 * The active Material scheme decides the direction: `outline` is darker than
 * light containers and lighter than dark ones, so dynamic colors keep working
 * without reading the system dark-mode flag. The outline starts at
 * `outlineVariant` and moves toward `outline` only until it reaches
 * [MIN_BORDER_CONTRAST_RATIO] against the effective fill.
 */
object AnpContainerDefaults {

    val BorderWidth = 1.dp

    /**
     * Decorative card boundaries are intentionally below the 3:1 non-text
     * contrast threshold reserved for controls and state boundaries. This
     * lower value keeps the line discrete while remaining visible on the
     * audited light and dark surfaces.
     */
    const val MIN_BORDER_CONTRAST_RATIO = 2.0

    @Composable
    fun borderStroke(containerColor: Color): BorderStroke {
        val scheme = MaterialTheme.colorScheme
        return BorderStroke(
            width = BorderWidth,
            color = borderColor(
                containerColor = containerColor.compositeOver(scheme.surface),
                outlineVariant = scheme.outlineVariant,
                outline = scheme.outline,
            ),
        )
    }

    internal fun borderColor(
        containerColor: Color,
        outlineVariant: Color,
        outline: Color,
    ): Color {
        val variantRatio = ColorContrast.contrastRatio(containerColor, outlineVariant)
        if (variantRatio >= MIN_BORDER_CONTRAST_RATIO) {
            return outlineVariant
        }

        val outlineRatio = ColorContrast.contrastRatio(containerColor, outline)
        if (outlineRatio <= variantRatio) {
            return outline
        }

        var lower = 0f
        var upper = 1f
        repeat(12) {
            val midpoint = (lower + upper) / 2f
            val candidate = lerp(containerColor, outline, midpoint)
            if (ColorContrast.contrastRatio(containerColor, candidate) >= MIN_BORDER_CONTRAST_RATIO) {
                upper = midpoint
            } else {
                lower = midpoint
            }
        }
        return lerp(containerColor, outline, upper)
    }
}
