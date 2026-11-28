package com.anpfuel.app.ui.theme

import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.graphics.Color

/**
 * Brand navy of the Abastevo logo and the landing ink (`#0B1849`). It fills
 * the station-business card ("Do you manage this station?"), where the app
 * addresses station owners: white text, a muted white body and a light filled
 * action, so it reads as Abastevo rather than as a community (green) notice.
 */
data class AbastevoBusinessColors(
    val container: Color,
    val content: Color,
    val contentMuted: Color,
    val action: Color,
    val onAction: Color,
)

internal object AbastevoBusiness {
    private val BrandNavy = Color(0xFF0B1849)

    val Light = AbastevoBusinessColors(
        container = BrandNavy,
        content = Paper,
        contentMuted = Color(0xFFD3DAF0),
        action = Paper,
        onAction = BrandNavy,
    )

    /** Lifted so the card still stands apart from the near-black dark screens. */
    val Dark = AbastevoBusinessColors(
        container = Color(0xFF213380),
        content = Paper,
        contentMuted = Color(0xFFD3DAF0),
        action = Mist,
        onAction = BrandNavy,
    )
}

/** Business card colors for the current theme. */
val abastevoBusinessColors: AbastevoBusinessColors
    @Composable
    @ReadOnlyComposable
    get() = if (LocalAnpDarkTheme.current) AbastevoBusiness.Dark else AbastevoBusiness.Light
