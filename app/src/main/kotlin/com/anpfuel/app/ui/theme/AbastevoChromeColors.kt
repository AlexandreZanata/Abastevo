package com.anpfuel.app.ui.theme

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsTopHeight
import androidx.compose.material3.ColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color

/**
 * Colors of the solid brand chrome: header, top app bars, the status bar band
 * and the bottom bar. [control] fills round buttons and fields drawn on the
 * chrome; [selected] and [onSelected] are the pill of the selected tab and
 * the light action on chrome. [hero] fills the one brand hero card of a
 * screen, which uses the same content colors.
 */
@Immutable
data class AbastevoChromeColors(
    val container: Color,
    val content: Color,
    val contentMuted: Color,
    val control: Color,
    val accent: Color,
    val selected: Color,
    val onSelected: Color,
    val hero: Color,
)

object AbastevoChromeDefaults {

    internal val Light = AbastevoChrome.Light

    internal val Dark = AbastevoChrome.Dark

    /** Scheme-driven chrome used when dynamic colors are explicitly enabled. */
    fun fromScheme(scheme: ColorScheme) = AbastevoChromeColors(
        container = scheme.inverseSurface,
        content = scheme.inverseOnSurface,
        contentMuted = scheme.inverseOnSurface.copy(alpha = 0.74f),
        control = scheme.inverseOnSurface.copy(alpha = 0.14f),
        accent = scheme.inversePrimary,
        selected = scheme.inverseOnSurface,
        onSelected = scheme.inverseSurface,
        hero = scheme.inverseSurface,
    )
}

val LocalAbastevoChromeColors = staticCompositionLocalOf { AbastevoChromeDefaults.Light }

/**
 * Chrome band behind the status bar, so screens without a header (the map,
 * full-screen flows) keep the same solid top edge and light status icons.
 */
@Composable
fun AbastevoStatusBarBand(modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .fillMaxWidth()
            .windowInsetsTopHeight(WindowInsets.statusBars)
            .background(LocalAbastevoChromeColors.current.container),
    )
}
