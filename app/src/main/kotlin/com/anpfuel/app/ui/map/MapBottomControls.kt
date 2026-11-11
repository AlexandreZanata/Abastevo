package com.anpfuel.app.ui.map

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
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
import androidx.compose.foundation.shape.CircleShape
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
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.OutlinedButton

private val PanelShape = RoundedCornerShape(topStart = 28.dp, topEnd = 28.dp)
private val ActionHeight = 52.dp

/** Spring shared by the panel height and the handle, soft but without bounce. */
private val PanelSpring = spring<androidx.compose.ui.unit.IntSize>(
    dampingRatio = Spring.DampingRatioNoBouncy,
    stiffness = Spring.StiffnessMediumLow,
)

/**
 * Stable map actions: an explicit drag-handle toggle and Filters/Download.
 * Discovery results never participate in this panel's layout. The ODbL
 * attribution stays visible over the map above it.
 *
 * Opening and closing animate the panel height with a soft spring while the
 * actions fade, so the map controls above follow the panel smoothly. Filters
 * is the main action (filled); Download is secondary (outlined).
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
    // The handle widens and darkens while collapsed to invite pulling it up.
    val handleWidth by animateDpAsState(if (expanded) 36.dp else 48.dp, label = "map-handle-width")
    val handleAlpha by animateFloatAsState(if (expanded) 0.22f else 0.4f, label = "map-handle-alpha")
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
            shape = PanelShape,
            color = MaterialTheme.colorScheme.surface,
            shadowElevation = 16.dp,
        ) {
            Column(
                modifier = Modifier.heightIn(max = maxPanelHeight)
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp),
            ) {
                val toggleLabel = stringResource(if (expanded) R.string.map_actions_hide else R.string.map_actions_show)
                // Drag the handle down to minimize to just the handle, up to restore;
                // a tap toggles too (accessible equivalent of the gesture).
                Box(
                    modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)
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
                        Modifier.size(width = handleWidth, height = 5.dp)
                            .background(MaterialTheme.colorScheme.onSurface.copy(alpha = handleAlpha), CircleShape),
                    )
                }
                AnimatedVisibility(
                    visible = expanded,
                    enter = expandVertically(animationSpec = PanelSpring, expandFrom = Alignment.Top) +
                        fadeIn(tween(durationMillis = 220, delayMillis = 60)),
                    exit = fadeOut(tween(durationMillis = 120)) +
                        shrinkVertically(animationSpec = PanelSpring, shrinkTowards = Alignment.Top),
                ) {
                    Column(
                        modifier = Modifier.padding(bottom = 16.dp),
                        verticalArrangement = Arrangement.spacedBy(10.dp),
                    ) {
                        val actions = listOf(
                            MapPanelAction(R.string.map_filters, Icons.Default.FilterAlt, "map-open-filters", primary = true, onFilters),
                            MapPanelAction(R.string.map_download, Icons.Default.Download, "map-open-downloads", primary = false, onDownloads),
                        )
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
}

@Composable
private fun MapPanelButton(action: MapPanelAction, modifier: Modifier = Modifier) {
    val buttonModifier = modifier.heightIn(min = ActionHeight).testTag(action.tag)
    val content: @Composable () -> Unit = {
        Icon(action.icon, contentDescription = null, modifier = Modifier.size(20.dp))
        Text(
            stringResource(action.label),
            style = MaterialTheme.typography.titleSmall,
            modifier = Modifier.padding(start = 10.dp),
        )
    }
    if (action.primary) {
        Button(onClick = action.onClick, modifier = buttonModifier) { content() }
    } else {
        OutlinedButton(onClick = action.onClick, modifier = buttonModifier) { content() }
    }
}

private data class MapPanelAction(
    @androidx.annotation.StringRes val label: Int,
    val icon: ImageVector,
    val tag: String,
    val primary: Boolean,
    val onClick: () -> Unit,
)
