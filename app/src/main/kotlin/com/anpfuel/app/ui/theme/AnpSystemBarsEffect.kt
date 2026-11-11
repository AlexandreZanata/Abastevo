package com.anpfuel.app.ui.theme

import android.app.Activity
import android.graphics.Color
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.platform.LocalView
import androidx.core.view.WindowCompat

/**
 * Keeps status/navigation bar icon contrast aligned with the brand chrome.
 * Both system bars are transparent: the header (or [AbastevoStatusBarBand])
 * fills the status bar and the bottom bar extends under the gesture area.
 * [lightStatusBar] and [lightNavigationBar] follow those fills, so system
 * icons always contrast with them instead of relying on a contrast scrim.
 */
@Composable
fun AnpSystemBarsEffect(lightStatusBar: Boolean, lightNavigationBar: Boolean) {
    val view = LocalView.current
    if (view.isInEditMode) {
        return
    }

    SideEffect {
        val window = (view.context as Activity).window
        WindowCompat.getInsetsController(window, view).apply {
            isAppearanceLightStatusBars = lightStatusBar
            isAppearanceLightNavigationBars = lightNavigationBar
        }
        @Suppress("DEPRECATION")
        window.statusBarColor = Color.TRANSPARENT
        @Suppress("DEPRECATION")
        window.navigationBarColor = Color.TRANSPARENT
    }
}
