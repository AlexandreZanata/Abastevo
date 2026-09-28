package com.anpfuel.app.ui.components

import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.statusBars
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowBack
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarColors
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.TopAppBarScrollBehavior
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import com.anpfuel.app.R

/**
 * Top app bar that respects the status bar inset under edge-to-edge.
 * Set [applyStatusBarInsets] to false for full-bleed hero layouts (Phase 13.2).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AnpTopAppBar(
    title: @Composable () -> Unit,
    modifier: Modifier = Modifier,
    onNavigateUp: (() -> Unit)? = null,
    navigationIcon: @Composable () -> Unit = {
        if (onNavigateUp != null) {
            IconButton(onClick = onNavigateUp) {
                Icon(
                    imageVector = Icons.Default.ArrowBack,
                    contentDescription = stringResource(R.string.action_back),
                )
            }
        }
    },
    actions: @Composable RowScope.() -> Unit = {},
    applyStatusBarInsets: Boolean = true,
    colors: TopAppBarColors = TopAppBarDefaults.topAppBarColors(),
    scrollBehavior: TopAppBarScrollBehavior? = null,
) {
    TopAppBar(
        title = title,
        modifier = modifier,
        navigationIcon = navigationIcon,
        actions = actions,
        colors = colors,
        scrollBehavior = scrollBehavior,
        windowInsets = if (applyStatusBarInsets) {
            WindowInsets.statusBars
        } else {
            WindowInsets(0, 0, 0, 0)
        },
    )
}
