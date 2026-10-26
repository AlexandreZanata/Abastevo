package com.anpfuel.app.ui.map

import android.Manifest
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.application.usecase.mapsaved.ExchangeFormat
import com.anpfuel.domain.map.saved.SavedBounds
import com.anpfuel.domain.map.saved.SavedTripKind
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

@Composable
private fun messageText(code: String?): String? = when (code) {
    null -> null
    "favorite-saved" -> stringResource(R.string.map25_msg_favorite_saved)
    "favorite-removed" -> stringResource(R.string.map25_msg_favorite_removed)
    "trip-saved" -> stringResource(R.string.map25_msg_trip_saved)
    "import-done" -> stringResource(R.string.map25_msg_import_done)
    "import-already" -> stringResource(R.string.map25_msg_import_already)
    "import-nothing-new" -> stringResource(R.string.map25_msg_import_nothing_new)
    "export-done" -> stringResource(R.string.map25_msg_export_done)
    "recording-saved" -> stringResource(R.string.map25_msg_recording_saved)
    "recording-discarded" -> stringResource(R.string.map25_msg_recording_discarded)
    "place-quota", "trip-quota" -> stringResource(R.string.map25_msg_quota, SavedBounds.MAX_PLACES, SavedBounds.MAX_TRIPS)
    "nothing-to-export" -> stringResource(R.string.map25_msg_nothing_to_export)
    "deleted-records-present" -> stringResource(R.string.map25_msg_deleted_present)
    "no-permission", "gps-off", "notification-blocked" -> stringResource(R.string.map25_msg_recording_blocked)
    "guidance-active" -> stringResource(R.string.map25_msg_guidance_active)
    "recording-exists" -> stringResource(R.string.map25_msg_recording_exists)
    else -> if (code.startsWith("xml-") || code.startsWith("archive-") || code.startsWith("file-") || code.startsWith("format-") ||
        code.startsWith("coordinate-") || code.startsWith("track-") || code.startsWith("too-many") || code.startsWith("duplicate")
    ) stringResource(R.string.map25_msg_file_refused) else stringResource(R.string.map25_msg_error)
}

/**
 * MAP-25 saved places/trips: private, local-only list with explicit import/export through the system
 * file chooser and an explicit, owner-scoped track recorder. No file is read until the user picks it.
 */
@Composable
fun SavedScreen(
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: SavedViewModel = hiltViewModel(),
) {
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    val serviceState by TripRecordingServiceState.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    LaunchedEffect(Unit) { viewModel.refresh() }
    LaunchedEffect(serviceState.active, serviceState.points) { viewModel.refresh() }

    val importer = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) {
            val bytes = try {
                context.contentResolver.openInputStream(uri)?.use { input ->
                    val out = java.io.ByteArrayOutputStream()
                    val buffer = ByteArray(16 * 1024)
                    while (true) {
                        val read = input.read(buffer)
                        if (read < 0) break
                        out.write(buffer, 0, read)
                        if (out.size() > SavedBounds.MAX_FILE_BYTES) return@use null
                    }
                    out.toByteArray()
                }
            } catch (_: Exception) { null }
            if (bytes == null) viewModel.importFile(ByteArray(SavedBounds.MAX_FILE_BYTES + 1)) else viewModel.importFile(bytes)
        }
    }
    val pending = ui.export
    val exporter = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("*/*")) { uri ->
        var result = "export-failed"
        if (uri != null && pending != null) {
            result = try {
                context.contentResolver.openOutputStream(uri)?.use { it.write(pending.bytes) }
                "export-done"
            } catch (_: Exception) { "export-failed" }
        }
        viewModel.exportConsumed(result)
    }
    LaunchedEffect(pending) { pending?.let { exporter.launch(it.fileName) } }

    val permissions = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { grants ->
        if (grants[Manifest.permission.ACCESS_FINE_LOCATION] == true) {
            val name = "Trip " + SimpleDateFormat("yyyy-MM-dd HH:mm", Locale.US).format(Date())
            ContextCompat.startForegroundService(context, TripRecordingService.startIntent(context, name))
        } else {
            viewModel.exportConsumed("no-permission")
        }
    }

    Column(
        modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(stringResource(R.string.map25_title), style = MaterialTheme.typography.titleLarge)
        Text(stringResource(R.string.map25_privacy), style = MaterialTheme.typography.bodySmall)
        messageText(ui.message ?: serviceState.error)?.let {
            Text(it, color = MaterialTheme.colorScheme.primary, modifier = Modifier.testTag("saved-message"))
        }

        val recording = ui.recording
        Text(stringResource(R.string.map25_recording_title), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.map25_recording_notice), style = MaterialTheme.typography.bodySmall)
        if (recording == null) {
            Button(
                onClick = {
                    val needed = buildList {
                        add(Manifest.permission.ACCESS_FINE_LOCATION)
                        if (Build.VERSION.SDK_INT >= 33) add(Manifest.permission.POST_NOTIFICATIONS)
                    }
                    permissions.launch(needed.toTypedArray())
                },
                modifier = Modifier.heightIn(min = 48.dp).testTag("saved-record-start"),
            ) { Text(stringResource(R.string.map25_record_start)) }
        } else {
            Text(stringResource(R.string.map25_recording_active, recording.pointCount), modifier = Modifier.testTag("saved-recording-state"))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(
                    onClick = {
                        if (serviceState.active) context.startService(TripRecordingService.stopIntent(context, save = true))
                        else viewModel.finishRecording(save = true)
                    },
                    modifier = Modifier.heightIn(min = 48.dp).testTag("saved-record-stop"),
                ) { Text(stringResource(R.string.map25_record_stop_save)) }
                OutlinedButton(
                    onClick = {
                        if (serviceState.active) context.startService(TripRecordingService.stopIntent(context, save = false))
                        else viewModel.finishRecording(save = false)
                    },
                    modifier = Modifier.heightIn(min = 48.dp).testTag("saved-record-discard"),
                ) { Text(stringResource(R.string.map25_record_discard)) }
            }
        }

        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(
                onClick = { importer.launch(arrayOf("*/*")) },
                modifier = Modifier.heightIn(min = 48.dp).testTag("saved-import"),
            ) { Text(stringResource(R.string.map25_import)) }
            OutlinedButton(
                onClick = { viewModel.prepareExport(ExchangeFormat.GPX) },
                enabled = ui.places.isNotEmpty() || ui.trips.isNotEmpty(),
                modifier = Modifier.heightIn(min = 48.dp).testTag("saved-export-gpx"),
            ) { Text(stringResource(R.string.map25_export_gpx)) }
            OutlinedButton(
                onClick = { viewModel.prepareExport(ExchangeFormat.KML) },
                enabled = ui.places.isNotEmpty() || ui.trips.isNotEmpty(),
                modifier = Modifier.heightIn(min = 48.dp).testTag("saved-export-kml"),
            ) { Text(stringResource(R.string.map25_export_kml)) }
        }

        Text(stringResource(R.string.map25_places), style = MaterialTheme.typography.titleMedium)
        if (ui.places.isEmpty()) Text(stringResource(R.string.map25_empty), style = MaterialTheme.typography.bodySmall)
        ui.places.forEach { place ->
            Row(Modifier.fillMaxWidth().testTag("saved-place-${place.id}"), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(place.name, modifier = Modifier.weight(1f).padding(top = 12.dp))
                TextButton(onClick = { viewModel.prepareExport(ExchangeFormat.GPX, placeId = place.id) }, modifier = Modifier.heightIn(min = 48.dp)) {
                    Text(stringResource(R.string.map25_export_one))
                }
                TextButton(onClick = { viewModel.deletePlace(place.id) }, modifier = Modifier.heightIn(min = 48.dp).testTag("saved-delete-place-${place.id}")) {
                    Text(stringResource(R.string.map25_delete))
                }
            }
        }

        Text(stringResource(R.string.map25_trips), style = MaterialTheme.typography.titleMedium)
        if (ui.trips.isEmpty()) Text(stringResource(R.string.map25_empty), style = MaterialTheme.typography.bodySmall)
        ui.trips.forEach { trip ->
            val kind = stringResource(
                when (trip.kind) {
                    SavedTripKind.ROUTE_PLAN -> R.string.map25_kind_plan
                    SavedTripKind.RECORDED_TRACK -> R.string.map25_kind_recorded
                    SavedTripKind.IMPORTED_TRACK -> R.string.map25_kind_imported
                },
            )
            Row(Modifier.fillMaxWidth().testTag("saved-trip-${trip.id}"), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text("${trip.name} · $kind · ${stringResource(R.string.map25_points, trip.pointCount)}", modifier = Modifier.weight(1f).padding(top = 12.dp))
                TextButton(onClick = { viewModel.prepareExport(ExchangeFormat.GPX, tripId = trip.id) }, modifier = Modifier.heightIn(min = 48.dp)) {
                    Text(stringResource(R.string.map25_export_one))
                }
                TextButton(onClick = { viewModel.deleteTrip(trip.id) }, modifier = Modifier.heightIn(min = 48.dp).testTag("saved-delete-trip-${trip.id}")) {
                    Text(stringResource(R.string.map25_delete))
                }
            }
        }
        Button(onClick = onBack, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("saved-back")) {
            Text(stringResource(R.string.map_downloads_ok))
        }
    }

    ui.pendingDeletedImport?.let { bytes ->
        AlertDialog(
            onDismissRequest = viewModel::dismissDeletedImport,
            title = { Text(stringResource(R.string.map25_reimport_title)) },
            text = { Text(stringResource(R.string.map25_reimport_body)) },
            confirmButton = {
                TextButton(onClick = { viewModel.importFile(bytes, confirmDeleted = true) }, modifier = Modifier.testTag("saved-reimport-confirm")) {
                    Text(stringResource(R.string.map25_reimport_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = viewModel::dismissDeletedImport) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }
}
