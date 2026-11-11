package com.anpfuel.app.ui.theme

import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/**
 * Colors of the fixed bottom navigation bar. The bar is solid brand chrome;
 * the selected tab sits in a light pill ([indicator]) with black content.
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

    internal val Light = fromChrome(AbastevoChrome.Light)

    internal val Dark = fromChrome(AbastevoChrome.Dark)

    /** Bar drawn on [chrome]; also used for dynamic colors via [AbastevoChromeDefaults.fromScheme]. */
    fun fromChrome(chrome: AbastevoChromeColors) = AbastevoNavColors(
        container = chrome.container,
        indicator = chrome.selected,
        selectedIcon = chrome.onSelected,
        selectedLabel = chrome.onSelected,
        unselected = chrome.contentMuted,
    )
}

val LocalAbastevoNavColors = staticCompositionLocalOf { AbastevoNavDefaults.Light }
