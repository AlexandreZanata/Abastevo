package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Box
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.runtime.remember
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.Role
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.clickable
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
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
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.FilterAlt
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
 * Stable map actions: an explicit drag-handle toggle and Filters/Download.
 * Discovery results never participate in this panel's layout. The ODbL
 * attribution stays visible over the map above it.
 */
@Composable
internal fun MapBottomControls(
    onFilters: () -> Unit,
    onDownloads: () -> Unit,
    modifier: Modifier = Modifier,
    /** Height of the sheet itself (without the attribution), for map controls above it. */
    onSheetHeightChanged: (Dp) -> Unit = {},
) {
    var expanded by rememberSaveable { mutableStateOf(true) }
    var dragDistance by remember { mutableFloatStateOf(0f) }
    val density = LocalDensity.current
    val dragThreshold = with(density) { 24.dp.toPx() }
    val columns = if (LocalDensity.current.fontScale > 1.3f) 1 else 2
    val maxPanelHeight = LocalConfiguration.current.screenHeightDp.dp * 0.5f
    Column(
        modifier = modifier.fillMaxWidth().testTag("map-bottom-overlay"),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Surface(
            modifier = Modifier.padding(start = 12.dp).testTag("map-attribution"),
            shape = CircleShape,
            color = MaterialTheme.colorScheme.surface.copy(alpha = 0.9f),
        ) {
            Text(
                stringResource(R.string.map_attribution),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
            )
        }
        Surface(
            modifier = Modifier.fillMaxWidth().testTag("map-action-panel")
                .onSizeChanged { size -> onSheetHeightChanged(with(density) { size.height.toDp() }) },
            shape = RoundedCornerShape(topStart = 24.dp, topEnd = 24.dp),
            color = MaterialTheme.colorScheme.surface,
            shadowElevation = 12.dp,
        ) {
            Column(
                modifier = Modifier.heightIn(max = maxPanelHeight)
                    .verticalScroll(rememberScrollState())
                    .padding(start = 16.dp, end = 16.dp, bottom = if (expanded) 12.dp else 0.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                val toggleLabel = stringResource(if (expanded) R.string.map_actions_hide else R.string.map_actions_show)
                // Drag the handle down to minimize to just the handle, up to restore;
                // a tap toggles too (accessible equivalent of the gesture).
                Box(
                    modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).padding(bottom = if (expanded) 0.dp else 4.dp)
                        .draggable(
                            state = rememberDraggableState { delta -> dragDistance += delta },
                            orientation = Orientation.Vertical,
                            onDragStarted = { dragDistance = 0f },
                            onDragStopped = {
                                if (dragDistance > dragThreshold) expanded = false
                                if (dragDistance < -dragThreshold) expanded = true
                            },
                        )
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
                if (expanded) {
                    val actions = buildList {
                        add(MapPanelAction(R.string.map_filters, Icons.Default.FilterAlt, "map-open-filters", onFilters))
                        add(MapPanelAction(R.string.map_download, Icons.Default.Download, "map-open-downloads", onDownloads))
                    }
                    actions.chunked(columns).forEach { row ->
                        Row(
                            modifier = Modifier.fillMaxWidth().height(IntrinsicSize.Min),
                            horizontalArrangement = Arrangement.spacedBy(10.dp),
                        ) {
                            row.forEach { action ->
                                MapPanelButton(action, Modifier.weight(1f).fillMaxHeight())
                            }
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
        shape = CircleShape,
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        contentColor = MaterialTheme.colorScheme.onSurface,
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 20.dp, vertical = 14.dp),
            horizontalArrangement = Arrangement.spacedBy(10.dp, Alignment.CenterHorizontally),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(action.icon, contentDescription = null, modifier = Modifier.size(20.dp))
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
