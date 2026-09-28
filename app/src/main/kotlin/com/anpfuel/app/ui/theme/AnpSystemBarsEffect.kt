package com.anpfuel.app.ui.theme

import android.app.Activity
import android.graphics.Color
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.platform.LocalView
import androidx.core.view.WindowCompat

/**
 * Keeps status/navigation bar icon contrast aligned with the active theme.
 * Dark mode: light icons on transparent status bar and black navigation bar.
 * Light mode: dark icons on transparent system bars.
 */
@Composable
fun AnpSystemBarsEffect(darkTheme: Boolean) {
    val view = LocalView.current
    if (view.isInEditMode) {
        return
    }

    SideEffect {
        val window = (view.context as Activity).window
        WindowCompat.getInsetsController(window, view).apply {
            isAppearanceLightStatusBars = !darkTheme
            isAppearanceLightNavigationBars = !darkTheme
        }
        @Suppress("DEPRECATION")
        window.statusBarColor = Color.TRANSPARENT
        @Suppress("DEPRECATION")
        window.navigationBarColor = if (darkTheme) Color.BLACK else Color.TRANSPARENT
    }
}
