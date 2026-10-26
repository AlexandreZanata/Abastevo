package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.domain.routing.NativeRoutePreview
import com.anpfuel.domain.routing.NativeRouteRequest

/** Real guidance controls bound to the foreground service and native core. */
@Composable
fun GuidanceControls(
    preview: NativeRoutePreview,
    request: NativeRouteRequest,
    viewModel: GuidanceViewModel,
    modifier: Modifier = Modifier,
) {
    val state by viewModel.serviceState.collectAsStateWithLifecycle()
    val resume by viewModel.resume.collectAsStateWithLifecycle()
    Column(modifier.fillMaxWidth().padding(vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(stringResource(R.string.map23_title), style = MaterialTheme.typography.titleMedium)
        val progress = state.progress
        if (progress != null) {
            Text(
                "%.0f m · %d s · %.0f%%".format(progress.remainingMeters, progress.seconds, progress.completionPercent),
                modifier = Modifier.testTag("guidance-progress"),
            )
            LinearProgressIndicator(
                progress = { (progress.completionPercent / 100.0).toFloat().coerceIn(0f, 1f) },
                modifier = Modifier.fillMaxWidth().testTag("guidance-progress-bar"),
            )
            if (progress.nextStreet.isNotEmpty()) {
                Text("${stringResource(R.string.map23_next)}: ${progress.nextStreet}", modifier = Modifier.testTag("guidance-next"))
            }
        }
        state.notifications.firstOrNull()?.let {
            Text(it, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.testTag("guidance-maneuver"))
        }
        if (state.recalculating) {
            Text(stringResource(R.string.map23_recalculating), modifier = Modifier.testTag("guidance-recalculating"))
            LinearProgressIndicator(Modifier.fillMaxWidth())
        }
        if (state.rerouteFailed) {
            Text(stringResource(R.string.map23_reroute_failed), color = MaterialTheme.colorScheme.error, modifier = Modifier.testTag("guidance-reroute-failed"))
        }
        if (state.gpsPoor || state.gpsLost) {
            Text(
                stringResource(if (state.gpsLost) R.string.map23_gps_lost else R.string.map23_gps_poor),
                modifier = Modifier.testTag("guidance-gps"),
            )
        }
        if (state.voiceUnavailable) {
            Text(stringResource(R.string.map23_voice_unavailable), modifier = Modifier.testTag("guidance-voice"))
        }
        state.error?.let {
            Text(
                stringResource(
                    when (it) {
                        "no-permission" -> R.string.map23_no_permission
                        "gps-off" -> R.string.map23_gps_off
                        "notification-blocked" -> R.string.map23_notification_blocked
                        else -> R.string.map22_error
                    },
                ),
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.testTag("guidance-error"),
            )
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            if (state.active) {
                Button(onClick = { viewModel.stop() }, modifier = Modifier.heightIn(min = 48.dp).testTag("guidance-stop")) {
                    Text(stringResource(R.string.map23_stop))
                }
            } else {
                Button(
                    onClick = { viewModel.start(request) },
                    modifier = Modifier.heightIn(min = 48.dp).testTag("guidance-start"),
                ) {
                    Text(stringResource(R.string.map23_start))
                }
                resume?.let {
                    OutlinedButton(onClick = { viewModel.resumeSaved() }, modifier = Modifier.heightIn(min = 48.dp).testTag("guidance-resume")) {
                        Text(stringResource(R.string.map23_resume))
                    }
                }
            }
        }
    }
}
