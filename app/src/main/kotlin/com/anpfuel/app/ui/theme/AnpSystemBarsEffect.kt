package com.anpfuel.app.ui.theme

import android.app.Activity
import android.graphics.Color
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.platform.LocalView
import androidx.core.view.WindowCompat

/**
 * Keeps status/navigation bar icon contrast aligned with the active theme.
 * Dark mode: light icons on transparent system bars.
 * Light mode: dark status icons on transparent system bars.
 * Transparent bars let the branded bottom bar extend under the gesture area;
 * [lightNavigationBar] follows that bar's fill, so the system buttons always
 * contrast with it instead of relying on a system contrast scrim.
 */
@Composable
fun AnpSystemBarsEffect(darkTheme: Boolean, lightNavigationBar: Boolean = !darkTheme) {
    val view = LocalView.current
    if (view.isInEditMode) {
        return
    }

    SideEffect {
        val window = (view.context as Activity).window
        WindowCompat.getInsetsController(window, view).apply {
            isAppearanceLightStatusBars = !darkTheme
            isAppearanceLightNavigationBars = lightNavigationBar
        }
        @Suppress("DEPRECATION")
        window.statusBarColor = Color.TRANSPARENT
        @Suppress("DEPRECATION")
        window.navigationBarColor = Color.TRANSPARENT
    }
}
