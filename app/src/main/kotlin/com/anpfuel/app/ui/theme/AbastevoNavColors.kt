package com.anpfuel.app.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/** Colors of the fixed bottom navigation bar, independent of card/sheet surfaces. */
@Immutable
data class AbastevoNavColors(
    val container: Color,
    val indicator: Color,
    val selectedIcon: Color,
    val selectedLabel: Color,
    val unselected: Color,
)

object AbastevoNavDefaults {

    /** Light brand bar: the header's navy with a blue selection pill. */
    internal val Light = AbastevoNavColors(
        container = EnergiaLightTokens.NavContainer,
        indicator = EnergiaLightTokens.NavIndicator,
        selectedIcon = EnergiaLightTokens.NavSelectedIcon,
        selectedLabel = EnergiaLightTokens.NavSelectedLabel,
        unselected = EnergiaLightTokens.NavUnselected,
    )

    /** Dark brand bar: the same navy container and blue selection pill as light. */
    internal val Dark = AbastevoNavColors(
        container = EnergiaDarkTokens.NavContainer,
        indicator = EnergiaDarkTokens.NavIndicator,
        selectedIcon = EnergiaDarkTokens.NavSelectedIcon,
        selectedLabel = EnergiaDarkTokens.NavSelectedLabel,
        unselected = EnergiaDarkTokens.NavUnselected,
    )

    /** Scheme-driven bar used when dynamic colors are explicitly enabled. */
    fun fromScheme(scheme: ColorScheme) = AbastevoNavColors(
        container = scheme.surfaceContainer,
        indicator = scheme.secondaryContainer,
        selectedIcon = scheme.onSecondaryContainer,
        selectedLabel = scheme.onSurface,
        unselected = scheme.onSurfaceVariant,
    )
}

val LocalAbastevoNavColors = staticCompositionLocalOf { AbastevoNavDefaults.Light }
