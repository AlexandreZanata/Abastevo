package com.anpfuel.app.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/**
 * Colors of the fixed bottom navigation bar. The bar is flat on the screen
 * color; the selected tab is ink with an accent marker above its icon.
 */
@Immutable
data class AbastevoNavColors(
    val container: Color,
    val indicator: Color,
    val selectedIcon: Color,
    val selectedLabel: Color,
    val unselected: Color,
)

object AbastevoNavDefaults {

    internal val Light = AbastevoNavColors(
        container = EnergiaLightTokens.NavContainer,
        indicator = AbastevoAccent.Light.accent,
        selectedIcon = EnergiaLightTokens.NavSelected,
        selectedLabel = EnergiaLightTokens.NavSelected,
        unselected = EnergiaLightTokens.NavUnselected,
    )

    internal val Dark = AbastevoNavColors(
        container = EnergiaDarkTokens.NavContainer,
        indicator = AbastevoAccent.Dark.accent,
        selectedIcon = EnergiaDarkTokens.NavSelected,
        selectedLabel = EnergiaDarkTokens.NavSelected,
        unselected = EnergiaDarkTokens.NavUnselected,
    )

    /** Scheme-driven bar used when dynamic colors are explicitly enabled. */
    fun fromScheme(scheme: ColorScheme) = AbastevoNavColors(
        container = scheme.surface,
        indicator = scheme.secondary,
        selectedIcon = scheme.onSurface,
        selectedLabel = scheme.onSurface,
        unselected = scheme.onSurfaceVariant,
    )
}

val LocalAbastevoNavColors = staticCompositionLocalOf { AbastevoNavDefaults.Light }
