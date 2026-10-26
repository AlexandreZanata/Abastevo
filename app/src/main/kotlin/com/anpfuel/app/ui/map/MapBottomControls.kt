package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Box
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
import androidx.compose.material.icons.filled.ChevronLeft
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.BookmarkBorder
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.FilterAlt
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.FilledTonalIconButton
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
import com.anpfuel.app.ui.components.ActionCardButton

/** Attribution stays over the map, separate from the bounded action container. */
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
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            FilledTonalIconButton(
                onClick = { expanded = !expanded },
                modifier = Modifier.size(48.dp).testTag("map-actions-toggle"),
            ) {
                Icon(
                    imageVector = if (expanded) Icons.Default.ChevronLeft else Icons.Default.ChevronRight,
                    contentDescription = stringResource(if (expanded) R.string.map_actions_hide else R.string.map_actions_show),
                )
            }
            Box(Modifier.weight(1f), contentAlignment = Alignment.CenterEnd) {
                Surface(
                    modifier = Modifier.testTag("map-attribution"),
                    shape = MaterialTheme.shapes.small,
                    color = MaterialTheme.colorScheme.surface.copy(alpha = 0.94f),
                ) {
                    Text(
                        stringResource(R.string.map_attribution),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(horizontal = 6.dp, vertical = 3.dp),
                    )
                }
            }
        }
        if (expanded) {
            Surface(
                modifier = Modifier.fillMaxWidth().testTag("map-action-panel"),
                shape = RoundedCornerShape(24.dp),
                color = MaterialTheme.colorScheme.surface.copy(alpha = 0.97f),
                shadowElevation = 3.dp,
            ) {
                Column(
                    modifier = Modifier.heightIn(max = maxPanelHeight)
                        .verticalScroll(rememberScrollState()).padding(12.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    content()
                    val actions = buildList {
                        add(MapPanelAction(R.string.map_filters, Icons.Default.FilterAlt, "map-open-filters", onFilters))
                        add(MapPanelAction(R.string.map_search, Icons.Default.Search, "map-open-search", onSearch))
                        add(MapPanelAction(R.string.map_download, Icons.Default.Download, "map-open-downloads", onDownloads))
                        if (onSaved != null) add(MapPanelAction(R.string.map25_open_saved, Icons.Default.BookmarkBorder, "map-open-saved", onSaved))
                    }
                    actions.chunked(columns).forEach { row ->
                        Row(
                            modifier = Modifier.fillMaxWidth().height(IntrinsicSize.Min),
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            row.forEach { action ->
                                ActionCardButton(
                                    label = stringResource(action.label),
                                    icon = action.icon,
                                    onClick = action.onClick,
                                    showChevron = false,
                                    modifier = Modifier.weight(1f).fillMaxHeight()
                                        .heightIn(min = 48.dp).testTag(action.tag),
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

private data class MapPanelAction(
    @androidx.annotation.StringRes val label: Int,
    val icon: ImageVector,
    val tag: String,
    val onClick: () -> Unit,
)
