package com.anpfuel.app.capture

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.ByteArrayOutputStream

/** Debug-only gallery ingress; URI grants and original bytes never enter saved state. */
@Composable
internal fun DeveloperCaptureControls(viewModel: CaptureOcrViewModel, onGallery: () -> Unit) {
    val enabled by viewModel.previewEnabled.collectAsStateWithLifecycle()
    val city by viewModel.previewCity.collectAsStateWithLifecycle()
    val query by viewModel.previewQuery.collectAsStateWithLifecycle()
    val stations by viewModel.previewStations.collectAsStateWithLifecycle()
    val picked by viewModel.pickedStationId.collectAsStateWithLifecycle()
    val review by viewModel.reviewUri.collectAsStateWithLifecycle()
    val busy by viewModel.gateBusy.collectAsStateWithLifecycle()
    val processing by viewModel.processing.collectAsStateWithLifecycle()
    val loading by viewModel.previewSearchBusy.collectAsStateWithLifecycle()
    val failed by viewModel.previewSearchFailed.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val files = remember(context) { PrivateCaptureFiles(context) }
    var importing by remember { mutableStateOf(false) }
    var importFailed by remember { mutableStateOf(false) }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        if (uri == null) viewModel.cameraCancelled() else {
            importing = true; importFailed = false
            scope.launch {
                try {
                    val copied = withContext(Dispatchers.IO) {
                        val mime = context.contentResolver.getType(uri)
                        require(mime in setOf("image/jpeg", "image/png"))
                        val bytes = context.contentResolver.openInputStream(uri)?.use { input ->
                            val output = ByteArrayOutputStream(); val buffer = ByteArray(8192)
                            while (true) {
                                val read = input.read(buffer); if (read < 0) break
                                require(output.size() + read <= PrivateCaptureFiles.MAX_BYTES)
                                output.write(buffer, 0, read)
                            }
                            output.toByteArray()
                        } ?: error("photo.read-failed")
                        Triple(bytes, mime!!, files.write(bytes, System.currentTimeMillis()))
                    }
                    val old = viewModel.reviewUri.value?.let(android.net.Uri::parse)
                    if (!viewModel.acceptPreviewPhoto(copied.first, copied.second, copied.third.toString())) {
                        withContext(Dispatchers.IO) { files.delete(copied.third) }; importFailed = true
                        viewModel.cameraCancelled()
                    } else if (old != null) withContext(Dispatchers.IO) { files.delete(old) }
                } catch (cancelled: kotlinx.coroutines.CancellationException) { throw cancelled }
                catch (_: Exception) { importFailed = true; viewModel.cameraCancelled() }
                finally { importing = false }
            }
        }
    }
    Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow)) {
        Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Row {
                Text(stringResource(R.string.developer_capture_title), Modifier.weight(1f), style = MaterialTheme.typography.titleMedium)
                Switch(checked = enabled, enabled = !busy && !processing && !importing, onCheckedChange = viewModel::setPreviewEnabled)
            }
            if (enabled) {
                Text(stringResource(R.string.developer_capture_notice), style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
                Text(city?.let { stringResource(R.string.developer_capture_city, it.name, it.state.abbreviation) }
                    ?: stringResource(R.string.developer_capture_city_missing))
                OutlinedTextField(value = query, onValueChange = viewModel::searchPreviewStation,
                    label = { Text(stringResource(R.string.developer_capture_station)) }, singleLine = true,
                    enabled = city != null && review == null && !busy, modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(16.dp))
                if (loading) LinearProgressIndicator(Modifier.fillMaxWidth())
                if (failed) Text(stringResource(R.string.developer_capture_search_failed), color = MaterialTheme.colorScheme.error)
                if (!loading && !failed && query.trim().length >= 2 && stations.isEmpty()) Text(stringResource(R.string.developer_capture_no_station))
                stations.forEach { station ->
                    FilterChip(selected = picked == station.stationId, enabled = review == null && !busy,
                        onClick = { viewModel.pickPreviewStation(station.stationId) }, label = { Text(station.displayName) })
                }
                OutlinedButton(onClick = {
                    viewModel.authorizeCamera { if (viewModel.beginCamera()) picker.launch("image/*") }
                }, enabled = picked != null && !busy && !processing && !importing, modifier = Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.developer_capture_gallery))
                }
                if (importing) LinearProgressIndicator(Modifier.fillMaxWidth())
                if (importFailed) Text(stringResource(R.string.capture_recognition_unavailable), color = MaterialTheme.colorScheme.error)
            }
        }
    }
}
