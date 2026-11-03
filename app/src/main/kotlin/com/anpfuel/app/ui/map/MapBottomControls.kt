package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Box
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.Role
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.clickable
import androidx.compose.foundation.background
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.BookmarkBorder
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.FilterAlt
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R

/**
 * Bottom sheet over the map: drag-handle toggle, list header (slot) and a 2x2
 * action grid. The ODbL attribution stays visible over the map above it.
 */
@Composable
internal fun MapBottomControls(
    onFilters: () -> Unit,
    onSearch: () -> Unit,
    onDownloads: () -> Unit,
    onSaved: (() -> Unit)?,
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit = {},
) {
    var expanded by rememberSaveable { mutableStateOf(true) }
    val columns = if (LocalDensity.current.fontScale > 1.3f) 1 else 2
    val maxPanelHeight = LocalConfiguration.current.screenHeightDp.dp * 0.5f
    Column(
        modifier = modifier.fillMaxWidth().testTag("map-bottom-overlay"),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Surface(
            modifier = Modifier.padding(start = 12.dp).testTag("map-attribution"),
            shape = CircleShape,
            color = MaterialTheme.colorScheme.surfaceContainer.copy(alpha = 0.9f),
        ) {
            Text(
                stringResource(R.string.map_attribution),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
            )
        }
        Surface(
            modifier = Modifier.fillMaxWidth().testTag("map-action-panel"),
            shape = RoundedCornerShape(topStart = 28.dp, topEnd = 28.dp),
            color = MaterialTheme.colorScheme.surfaceContainer,
            shadowElevation = 12.dp,
        ) {
            Column(
                modifier = Modifier.heightIn(max = maxPanelHeight)
                    .verticalScroll(rememberScrollState())
                    .padding(start = 16.dp, end = 16.dp, bottom = 12.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                val toggleLabel = stringResource(if (expanded) R.string.map_actions_hide else R.string.map_actions_show)
                Box(
                    modifier = Modifier.fillMaxWidth().heightIn(min = 28.dp)
                        .clickable(role = Role.Button, onClickLabel = toggleLabel) { expanded = !expanded }
                        .semantics { contentDescription = toggleLabel }
                        .testTag("map-actions-toggle"),
                    contentAlignment = Alignment.Center,
                ) {
                    Box(
                        Modifier.padding(top = 10.dp).size(width = 36.dp, height = 4.dp)
                            .background(MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.4f), CircleShape),
                    )
                }
                content()
                if (expanded) {
                    val actions = buildList {
                        add(MapPanelAction(R.string.map_filters, Icons.Default.FilterAlt, "map-open-filters", onFilters))
                        add(MapPanelAction(R.string.map_search, Icons.Default.Search, "map-open-search", onSearch))
                        add(MapPanelAction(R.string.map_download, Icons.Default.Download, "map-open-downloads", onDownloads))
                        if (onSaved != null) add(MapPanelAction(R.string.map25_open_saved, Icons.Default.BookmarkBorder, "map-open-saved", onSaved))
                    }
                    actions.chunked(columns).forEach { row ->
                        Row(
                            modifier = Modifier.fillMaxWidth().height(IntrinsicSize.Min),
                            horizontalArrangement = Arrangement.spacedBy(10.dp),
                        ) {
                            row.forEach { action ->
                                MapPanelButton(action, Modifier.weight(1f).fillMaxHeight())
                            }
                            if (row.size < columns) Spacer(Modifier.weight(1f))
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun MapPanelButton(action: MapPanelAction, modifier: Modifier = Modifier) {
    Surface(
        onClick = action.onClick,
        modifier = modifier.heightIn(min = 56.dp).testTag(action.tag),
        shape = RoundedCornerShape(16.dp),
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        contentColor = MaterialTheme.colorScheme.onSurface,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 14.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(action.icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
            Text(stringResource(action.label), style = MaterialTheme.typography.titleSmall)
        }
    }
}

private data class MapPanelAction(
    @androidx.annotation.StringRes val label: Int,
    val icon: ImageVector,
    val tag: String,
    val onClick: () -> Unit,
)
