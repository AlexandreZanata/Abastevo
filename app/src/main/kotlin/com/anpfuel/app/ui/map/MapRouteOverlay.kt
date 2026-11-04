package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.automirrored.filled.VolumeOff
import androidx.compose.material.icons.automirrored.filled.VolumeUp
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Explore
import androidx.compose.material.icons.filled.MyLocation
import androidx.compose.material.icons.filled.Remove
import androidx.compose.material.icons.filled.Route
import androidx.compose.material.icons.filled.Flag
import androidx.compose.material.icons.filled.RoundaboutRight
import androidx.compose.material.icons.filled.Straight
import androidx.compose.material.icons.filled.TurnLeft
import androidx.compose.material.icons.filled.TurnRight
import androidx.compose.material.icons.filled.UTurnLeft
import androidx.compose.material.icons.filled.UTurnRight
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntRect
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.domain.routing.NativeRoutePreview
import com.anpfuel.domain.routing.RouteManeuver
import com.anpfuel.maps.engine.GuidanceCameraAction
import java.text.NumberFormat
import kotlin.math.ceil

@OptIn(ExperimentalLayoutApi::class, ExperimentalMaterial3Api::class)
@Composable
internal fun MapRouteOverlay(
    route: NativeRouteUi,
    guidance: GuidanceUiState,
    onCurrentOrigin: () -> Unit,
    onEdit: () -> Unit,
    onDownloads: () -> Unit,
    onStart: () -> Unit,
    onClose: () -> Unit,
    onInsetChanged: (Dp) -> Unit,
    onGuidanceViewport: (IntRect, IntSize) -> Unit,
    onCameraAction: (GuidanceCameraAction) -> Unit,
    onVoiceMuted: (Boolean) -> Unit,
    loadManeuvers: () -> List<RouteManeuver>,
) {
    val density = LocalDensity.current
    val safeInsets = WindowInsets.safeDrawing
    val direction = LocalLayoutDirection.current
    var surfaceSize by remember { mutableStateOf(IntSize.Zero) }
    var headerHeight by remember { mutableIntStateOf(0) }
    var cameraHeight by remember { mutableIntStateOf(0) }
    var footerHeight by remember { mutableIntStateOf(0) }
    val verticalGap = with(density) { 8.dp.roundToPx() }
    val visible = IntRect(
        left = safeInsets.getLeft(density, direction),
        top = safeInsets.getTop(density) + verticalGap + headerHeight,
        right = surfaceSize.width - safeInsets.getRight(density, direction),
        bottom = surfaceSize.height - safeInsets.getBottom(density) - verticalGap - cameraHeight - footerHeight,
    )
    LaunchedEffect(guidance.active, visible, surfaceSize) {
        if (guidance.active && headerHeight > 0 && cameraHeight > 0 && footerHeight > 0) {
            onGuidanceViewport(visible, surfaceSize)
        }
    }
    val preview = route.preview
    val progress = guidance.progress
    val waiting = route.findingOrigin || route.building || guidance.starting
    var showSteps by remember(preview) { mutableStateOf(false) }
    var guidanceSteps by remember(guidance.active) { mutableStateOf<List<RouteManeuver>?>(null) }
    val destination = guidance.destinationLabel.ifBlank {
        route.destination?.label.orEmpty().ifBlank { stringResource(R.string.map22_destination) }
    }

    Column(
        Modifier.fillMaxSize().onSizeChanged { surfaceSize = it }.safeDrawingPadding().padding(horizontal = 12.dp, vertical = 8.dp)
            .testTag("map-route-overlay"),
    ) {
        Surface(
            modifier = Modifier.fillMaxWidth().onSizeChanged { headerHeight = it.height },
            shape = MaterialTheme.shapes.large,
            color = if (guidance.active) MaterialTheme.colorScheme.primaryContainer
                else MaterialTheme.colorScheme.surfaceContainerHigh,
            shadowElevation = 4.dp,
        ) {
            Row(
                Modifier.padding(start = 16.dp, end = 8.dp, top = 8.dp, bottom = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (guidance.active) {
                    Icon(turnIcon(progress?.direction), contentDescription = null, modifier = Modifier.size(42.dp))
                }
                Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    if (guidance.active) {
                        val turnDistance = progress?.turnMeters?.takeIf { it.isFinite() && it > 0 }
                        Text(
                            turnDistance?.let { distanceLabel(it) } ?: stringResource(
                                if (progress == null) R.string.map_host_waiting_fix else R.string.map23_next,
                            ),
                            style = MaterialTheme.typography.titleLarge,
                            modifier = Modifier.testTag("guidance-turn-distance"),
                        )
                        val instruction = guidance.notifications.firstOrNull()
                            ?: progress?.nextStreet?.takeIf { it.isNotBlank() }
                        instruction?.let {
                            Text(it, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.testTag("guidance-maneuver"))
                        }
                    } else {
                        Text(
                            if (guidance.arrived) stringResource(R.string.map_route_arrived) else destination,
                            style = MaterialTheme.typography.titleLarge,
                            maxLines = 2,
                            overflow = TextOverflow.Ellipsis,
                        )
                        if (guidance.arrived) {
                            Text(destination, maxLines = 2, overflow = TextOverflow.Ellipsis)
                        } else {
                            route.origin?.label?.takeIf { it.isNotBlank() }?.let {
                                Text(it, style = MaterialTheme.typography.bodySmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                        }
                    }
                }
                if (!guidance.active) {
                    IconButton(onClick = onClose, modifier = Modifier.testTag("map-route-close")) {
                        Icon(Icons.Default.Close, contentDescription = stringResource(R.string.action_dismiss))
                    }
                }
            }
        }

        Spacer(Modifier.weight(1f))

        if (guidance.active) {
            FlowRow(
                modifier = Modifier.align(Alignment.End).onSizeChanged { cameraHeight = it.height }.padding(bottom = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp, Alignment.End),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                GuidanceIcon(Icons.Default.Remove, R.string.map_guidance_zoom_out, "guidance-zoom-out") {
                    onCameraAction(GuidanceCameraAction.ZOOM_OUT)
                }
                GuidanceIcon(Icons.Default.Add, R.string.map_guidance_zoom_in, "guidance-zoom-in") {
                    onCameraAction(GuidanceCameraAction.ZOOM_IN)
                }
                GuidanceIcon(Icons.Default.Explore, R.string.map_guidance_orientation, "guidance-orientation", enabled = progress != null) {
                    onCameraAction(GuidanceCameraAction.ORIENTATION)
                }
                GuidanceIcon(Icons.Default.Route, R.string.map_guidance_overview, "guidance-overview") {
                    onCameraAction(GuidanceCameraAction.OVERVIEW)
                }
                GuidanceIcon(Icons.Default.MyLocation, R.string.map_guidance_recenter, "guidance-recenter") {
                    onCameraAction(GuidanceCameraAction.RECENTER)
                }
            }
        }

        Surface(
            modifier = Modifier.fillMaxWidth().onSizeChanged { size ->
                footerHeight = size.height
                onInsetChanged(with(density) { size.height.toDp() } + 16.dp)
            },
            shape = MaterialTheme.shapes.large,
            color = MaterialTheme.colorScheme.surfaceContainerHigh,
            shadowElevation = 6.dp,
        ) {
            if (guidance.active) {
                ActiveGuidancePanel(guidance, preview, onClose, onVoiceMuted) { guidanceSteps = loadManeuvers() }
            } else Column(
                Modifier.heightIn(max = 320.dp).verticalScroll(rememberScrollState()).padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                val estimateMeters = preview?.distanceMeters
                val estimateSeconds = preview?.seconds
                if (!guidance.arrived && estimateMeters != null && estimateSeconds != null) {
                    Text(
                        stringResource(R.string.map_route_estimate, timeLabel(estimateSeconds), distanceLabel(estimateMeters)),
                        style = MaterialTheme.typography.titleLarge,
                        modifier = Modifier.testTag("map-route-estimate"),
                    )
                }
                if (waiting || guidance.recalculating) {
                    Text(stringResource(when {
                        guidance.recalculating -> R.string.map23_recalculating
                        route.findingOrigin -> R.string.map_route_locating
                        route.building -> R.string.map_route_calculating
                        else -> R.string.map23_start
                    }))
                    LinearProgressIndicator(Modifier.fillMaxWidth())
                }
                when {
                    guidance.arrived -> Button(onClick = onClose, modifier = Modifier.fillMaxWidth()) {
                        Text(stringResource(R.string.action_dismiss))
                    }
                    else -> {
                        val failure = guidance.error ?: route.error
                        failure?.let {
                            Text(
                                stringResource(when (it) {
                                    "no-fix", "no-permission" -> if (guidance.error != null)
                                        R.string.map23_no_permission else R.string.map22_fix_error
                                    "gps-off" -> R.string.map23_gps_off
                                    "notification-blocked" -> R.string.map23_notification_blocked
                                    else -> R.string.map22_error
                                }),
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier.testTag("map-route-error"),
                            )
                        }
                        if (route.missingMaps.isNotEmpty()) Text(route.missingMaps.joinToString("\n"))
                        if (preview?.warnings == true) Text(stringResource(R.string.map22_warnings))
                        if (preview != null && !guidance.starting) {
                            Button(onClick = onStart, modifier = Modifier.fillMaxWidth().testTag("guidance-start")) {
                                Text(stringResource(R.string.map23_start))
                            }
                        }
                        if (!guidance.starting) {
                            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                OutlinedButton(onClick = onEdit, modifier = Modifier.testTag("map-route-edit")) {
                                    Text(stringResource(R.string.map_route_edit))
                                }
                                if (preview == null && !waiting) {
                                    TextButton(onClick = onCurrentOrigin, modifier = Modifier.testTag("map-route-current")) {
                                        Text(stringResource(R.string.map22_current))
                                    }
                                }
                                if (route.error != null && route.error !in setOf("no-fix", "no-permission")) {
                                    TextButton(onClick = onDownloads) { Text(stringResource(R.string.map_downloads)) }
                                }
                                if (!preview?.maneuvers.isNullOrEmpty()) {
                                    TextButton(onClick = { showSteps = !showSteps }) {
                                        Text(stringResource(R.string.map_route_steps))
                                    }
                                }
                            }
                        }
                        if (showSteps) preview?.maneuvers?.forEach { step -> Text(step.instruction) }
                    }
                }
            }
        }
    }
    guidanceSteps?.let { steps ->
        ModalBottomSheet(onDismissRequest = { guidanceSteps = null }) {
            LazyColumn(
                modifier = Modifier.fillMaxWidth().heightIn(max = 480.dp).padding(horizontal = 20.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                item {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(stringResource(R.string.map_route_steps), style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f))
                        IconButton(onClick = { guidanceSteps = null }) {
                            Icon(Icons.Default.Close, stringResource(R.string.feed_close))
                        }
                    }
                }
                if (steps.isEmpty()) item { Text(stringResource(R.string.map_no_results)) }
                itemsIndexed(steps) { index, step ->
                    Text("${index + 1}. ${step.instruction}", style = MaterialTheme.typography.bodyLarge)
                }
            }
        }
    }
}

@Composable
private fun ActiveGuidancePanel(
    guidance: GuidanceUiState,
    preview: NativeRoutePreview?,
    onStop: () -> Unit,
    onVoiceMuted: (Boolean) -> Unit,
    onManeuvers: () -> Unit,
) {
    Column(Modifier.padding(horizontal = 12.dp, vertical = 8.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Column(
                modifier = Modifier.weight(1f).heightIn(max = 90.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                val seconds = guidance.progress?.seconds ?: preview?.seconds
                val meters = guidance.progress?.remainingMeters ?: preview?.distanceMeters
                if (seconds != null && meters != null) {
                    Text(
                        stringResource(R.string.map_route_estimate, timeLabel(seconds), distanceLabel(meters)),
                        style = MaterialTheme.typography.titleLarge,
                        modifier = Modifier.testTag("guidance-progress"),
                    )
                }
                val message = when {
                    guidance.rerouteFailed -> R.string.map23_reroute_failed
                    guidance.recalculating -> R.string.map23_recalculating
                    guidance.gpsLost -> R.string.map23_gps_lost
                    guidance.gpsPoor -> R.string.map23_gps_poor
                    guidance.voiceUnavailable -> R.string.map23_voice_unavailable
                    else -> null
                }
                message?.let { Text(stringResource(it), style = MaterialTheme.typography.bodySmall) }
            }
            GuidanceIcon(
                if (guidance.voiceMuted || guidance.voiceUnavailable) Icons.AutoMirrored.Filled.VolumeOff else Icons.AutoMirrored.Filled.VolumeUp,
                if (guidance.voiceMuted) R.string.map_guidance_unmute else R.string.map_guidance_mute,
                "guidance-voice-toggle",
                enabled = !guidance.voiceUnavailable,
            ) { onVoiceMuted(!guidance.voiceMuted) }
            GuidanceIcon(Icons.AutoMirrored.Filled.List, R.string.map_route_steps, "guidance-maneuvers", onClick = onManeuvers)
        }
        Button(onClick = onStop, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("guidance-stop")) {
            Text(stringResource(R.string.map23_stop))
        }
    }
}

@Composable
private fun GuidanceIcon(icon: ImageVector, label: Int, tag: String, enabled: Boolean = true, onClick: () -> Unit) {
    FilledTonalIconButton(onClick = onClick, enabled = enabled, modifier = Modifier.size(48.dp).testTag(tag)) {
        Icon(icon, stringResource(label))
    }
}


@Composable
private fun distanceLabel(meters: Double): String {
    val locale = LocalConfiguration.current.locales[0]
    val format = NumberFormat.getNumberInstance(locale)
    return if (meters < 1000) {
        format.maximumFractionDigits = 0
        stringResource(R.string.map_route_meters, format.format(meters.coerceAtLeast(0.0)))
    } else {
        format.maximumFractionDigits = 1
        stringResource(R.string.map_route_kilometers, format.format(meters / 1000))
    }
}

@Composable
private fun timeLabel(seconds: Int): String {
    val minutes = ceil(seconds.coerceAtLeast(0) / 60.0).toInt()
    return if (minutes < 60) stringResource(R.string.map_route_minutes, minutes)
    else stringResource(R.string.map_route_hours_minutes, minutes / 60, minutes % 60)
}

private fun turnIcon(direction: String?) = when (direction) {
    "TURN_LEFT", "TURN_SHARP_LEFT", "TURN_SLIGHT_LEFT", "EXIT_HIGHWAY_TO_LEFT" -> Icons.Default.TurnLeft
    "TURN_RIGHT", "TURN_SHARP_RIGHT", "TURN_SLIGHT_RIGHT", "EXIT_HIGHWAY_TO_RIGHT" -> Icons.Default.TurnRight
    "U_TURN_LEFT" -> Icons.Default.UTurnLeft
    "U_TURN_RIGHT" -> Icons.Default.UTurnRight
    "ENTER_ROUND_ABOUT", "LEAVE_ROUND_ABOUT", "STAY_ON_ROUND_ABOUT" -> Icons.Default.RoundaboutRight
    "REACHED_YOUR_DESTINATION" -> Icons.Default.Flag
    else -> Icons.Default.Straight
}
