package com.anpfuel.app.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/**
 * Fill and label of filled buttons: black with a white label on light
 * screens, off-white with a black label on dark ones. Kept apart from
 * `primary` so the button fill can differ from it under dynamic colors.
 */
@Immutable
data class AbastevoActionColors(
    val container: Color,
    val content: Color,
)

object AbastevoActionDefaults {

    internal val Light = AbastevoActionColors(
        container = EnergiaLightTokens.Action,
        content = EnergiaLightTokens.OnAction,
    )

    internal val Dark = AbastevoActionColors(
        container = EnergiaDarkTokens.Action,
        content = EnergiaDarkTokens.OnAction,
    )

    /** Scheme-driven actions used when dynamic colors are explicitly enabled. */
    fun fromScheme(scheme: ColorScheme) = AbastevoActionColors(
        container = scheme.primary,
        content = scheme.onPrimary,
    )
}

val LocalAbastevoActionColors = staticCompositionLocalOf { AbastevoActionDefaults.Light }
