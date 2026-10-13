package com.anpfuel.app.ui.components

import android.view.View
import androidx.compose.runtime.Composable
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat

/**
 * Bottom system padding captured where Compose inset reads are still
 * intact (above the consuming scaffolds) and shared down the tree.
 *
 * Dialog windows on some devices report zero bottom insets while the
 * main window reports the real navigation height; surfaces anchored to
 * the screen bottom inside dialogs back the framework value with this
 * one instead of guessing.
 */
val LocalBottomSafePadding = compositionLocalOf { 0.dp }

/**
 * Raw platform system paddings, unaffected by Compose inset consumption.
 *
 * The nav shell and every screen scaffold consume the system insets for
 * layout, so Compose inset reads below them resolve to zero and surfaces
 * anchored to the screen edge (modal sheets, fullscreen readers) would
 * slide under the status or navigation bars. The platform root insets are
 * unaffected by Compose consumption. IME is deliberately excluded (no
 * text input on these surfaces); values refresh on configuration change.
 */
@Composable
fun rememberRawSystemTopPadding(): Dp {
    val view = LocalView.current
    val density = LocalDensity.current
    val configuration = LocalConfiguration.current
    return remember(view, density, configuration) {
        with(density) { rawSystemTop(view).toDp() }
    }
}

@Composable
fun rememberRawSystemBottomPadding(): Dp {
    val view = LocalView.current
    val density = LocalDensity.current
    val configuration = LocalConfiguration.current
    return remember(view, density, configuration) {
        with(density) { rawSystemBottom(view).toDp() }
    }
}

private fun rawSystemTop(view: View): Int {
    val root = ViewCompat.getRootWindowInsets(view) ?: return 0
    val bars = root.getInsets(WindowInsetsCompat.Type.systemBars()).top
    val cutout = root.getInsets(WindowInsetsCompat.Type.displayCutout()).top
    return maxOf(bars, cutout)
}

internal fun rawSystemBottom(view: View): Int {
    val root = ViewCompat.getRootWindowInsets(view) ?: return 0
    val bars = root.getInsets(WindowInsetsCompat.Type.systemBars()).bottom
    val cutout = root.getInsets(WindowInsetsCompat.Type.displayCutout()).bottom
    return maxOf(bars, cutout)
}
