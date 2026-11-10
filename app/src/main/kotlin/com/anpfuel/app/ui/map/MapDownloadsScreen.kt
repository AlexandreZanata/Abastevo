package com.anpfuel.app.ui.map

import android.text.format.Formatter
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.CloudDownload
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.Map
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.filled.Schedule
import androidx.compose.material.icons.filled.SdStorage
import androidx.compose.material.icons.filled.SignalCellularAlt
import androidx.compose.material.icons.filled.Wifi
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.OutlinedButton
import com.anpfuel.app.ui.designsystem.TextButton

/**
 * Usable map-download screen: configured origin -> real envelope -> pinned-key
 * verification -> catalog with sizes -> explicit download (unique work) with
 * progress/cancel/retry -> verified activation -> observable state. Missing
 * configuration shows the specific pendency; network failure is retryable and
 * never presented as "no package" nor success.
 */
@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun MapDownloadsScreen(
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: MapDownloadsViewModel = hiltViewModel(),
) {
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.map_downloads)) },
                onNavigateUp = onBack,
            )
        },
    ) { padding ->
        Column(
            Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            RegionHeader(ui)
            when {
                ui.loading -> DownloadCard {
                    Text(stringResource(R.string.map_loading), style = MaterialTheme.typography.bodyMedium)
                    LinearProgressIndicator(Modifier.fillMaxWidth())
                }
                ui.unconfigured -> DownloadCard {
                    Text(stringResource(R.string.map_dist_unconfigured), style = MaterialTheme.typography.bodyMedium)
                    if (ui.missingUrl) Text("map_dist_base_url: missing", style = MaterialTheme.typography.bodySmall)
                    if (ui.missingKeys) Text("map_dist_keys: missing", style = MaterialTheme.typography.bodySmall)
                    if (ui.keysInvalid) Text("map_dist_keys: invalid", style = MaterialTheme.typography.bodySmall)
                    OutlinedButton(onClick = { viewModel.refresh() }, modifier = Modifier.heightIn(min = 48.dp)) {
                        Text(stringResource(R.string.map_retry))
                    }
                }
                ui.error != null -> DownloadCard {
                    val retryDesc = stringResource(R.string.map_retry)
                    Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Default.ErrorOutline, null, tint = MaterialTheme.colorScheme.error)
                        Text(
                            if (ui.error == "in-use") stringResource(R.string.map_downloads_in_use) else ui.error.orEmpty(),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                    }
                    OutlinedButton(
                        onClick = { viewModel.retry() },
                        modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = retryDesc },
                        enabled = ui.retryable || ui.candidateGeneration != null,
                    ) {
                        Text(retryDesc)
                    }
                }
                else -> DownloadContent(ui, viewModel)
            }
            Button(
                onClick = onBack,
                modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp).testTag("map-download-back"),
            ) {
                Text(stringResource(R.string.map_downloads_ok))
            }
        }
    }
}

@Composable
private fun RegionHeader(ui: MapDownloadsUi) {
    Row(horizontalArrangement = Arrangement.spacedBy(14.dp), verticalAlignment = Alignment.CenterVertically) {
        Surface(shape = CircleShape, color = MaterialTheme.colorScheme.primaryContainer, modifier = Modifier.size(52.dp)) {
            Box(contentAlignment = Alignment.Center) {
                Icon(Icons.Default.Map, null, tint = MaterialTheme.colorScheme.onPrimaryContainer)
            }
        }
        Column(Modifier.weight(1f)) {
            ui.region?.let { region ->
                Text(
                    region.name,
                    style = MaterialTheme.typography.titleLarge,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.testTag("map-download-region"),
                )
            }
            Text(
                stringResource(R.string.map_downloads_subtitle),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun DownloadContent(ui: MapDownloadsUi, viewModel: MapDownloadsViewModel) {
    val context = LocalContext.current
    fun size(bytes: Long): String = Formatter.formatShortFileSize(context, bytes)
    DownloadCard {
        // Installed state first: what works offline right now.
        val active = ui.activeGeneration
        StatusRow(
            icon = if (active != null && ui.published) Icons.Default.CheckCircle else Icons.Default.SdStorage,
            tint = if (active != null && ui.published) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
            title = if (active != null) stringResource(R.string.map_downloads_active_generation, active) else stringResource(R.string.map_no_package),
            subtitle = when {
                active == null -> null
                ui.published -> stringResource(R.string.map_published)
                else -> stringResource(R.string.map_needs_activation)
            },
        )
        if (active != null && !ui.published) {
            val activateLabel = stringResource(R.string.map_activate)
            Button(
                onClick = { viewModel.activateActive() },
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).semantics { contentDescription = activateLabel },
            ) {
                Text(activateLabel)
            }
        }
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            InfoChip(
                icon = if (ui.wifiOnly) Icons.Default.Wifi else Icons.Default.SignalCellularAlt,
                label = stringResource(if (ui.wifiOnly) R.string.map_downloads_wifi_only else R.string.map_downloads_wifi_any),
            )
            if (ui.candidateGeneration != null) {
                InfoChip(
                    icon = Icons.Default.CloudDownload,
                    label = stringResource(R.string.map_downloads_size_total, size(ui.candidateBytes), ui.candidatePackages.size),
                )
            }
        }
    }

    val candidate = ui.candidateGeneration
    if (candidate != null) {
        DownloadCard {
            Text(
                stringResource(R.string.map_downloads_update_generation, candidate),
                style = MaterialTheme.typography.titleMedium,
            )
            if (ui.candidateDependencies.isNotEmpty()) {
                Text(
                    stringResource(R.string.map_downloads_depends_on, ui.candidateDependencies.joinToString()),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (ui.downloading) {
                val total = ui.candidateBytes.coerceAtLeast(1)
                val fraction = (ui.downloadedBytes.toFloat() / total).coerceIn(0f, 1f)
                Row(verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text(
                        "${(fraction * 100).toInt()}%",
                        style = MaterialTheme.typography.headlineMedium,
                        fontWeight = FontWeight.SemiBold,
                        color = MaterialTheme.colorScheme.primary,
                    )
                    Text(
                        stringResource(R.string.map_downloads_progress, size(ui.downloadedBytes), size(ui.candidateBytes)),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(bottom = 4.dp),
                    )
                }
                LinearProgressIndicator(
                    progress = { fraction },
                    modifier = Modifier.fillMaxWidth().heightIn(min = 6.dp),
                    strokeCap = androidx.compose.ui.graphics.StrokeCap.Round,
                )
            }
            ui.candidatePackages.forEach { pkg -> PackageRow(pkg, ::size) }
            if (ui.downloading) {
                val cancelLabel = stringResource(R.string.map_cancel)
                OutlinedButton(
                    onClick = { viewModel.cancel() },
                    modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).semantics { contentDescription = cancelLabel },
                ) {
                    Text(cancelLabel)
                }
            } else {
                val downloadLabel = stringResource(R.string.map_download_with_size, size(ui.candidateBytes))
                Button(
                    onClick = { viewModel.download() },
                    modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("map-download-start")
                        .semantics { contentDescription = downloadLabel },
                ) {
                    Icon(Icons.Default.CloudDownload, null, Modifier.padding(end = 8.dp))
                    Text(downloadLabel)
                }
            }
        }
    } else if (ui.regionUnknown) {
        DownloadCard {
            Text(
                stringResource(R.string.map_region_unknown),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("map-download-region-unknown"),
            )
        }
    } else {
        val checkUpdateLabel = stringResource(R.string.map_check_update)
        OutlinedButton(
            onClick = { viewModel.checkForUpdate() },
            modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("map-download-update")
                .semantics { contentDescription = checkUpdateLabel },
        ) {
            Text(checkUpdateLabel)
        }
    }

    if (ui.inactiveGenerations.isNotEmpty()) {
        DownloadCard {
            val deleteLabel = stringResource(R.string.map_downloads_delete)
            ui.inactiveGenerations.forEach { generation ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        stringResource(R.string.map_downloads_stored, generation),
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.weight(1f),
                    )
                    TextButton(
                        onClick = { viewModel.deleteInactive(generation) },
                        modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = deleteLabel },
                    ) {
                        Icon(Icons.Default.DeleteOutline, null, Modifier.padding(end = 4.dp))
                        Text(deleteLabel)
                    }
                }
            }
        }
    }
}

@Composable
private fun PackageRow(pkg: MapPackageUi, size: (Long) -> String) {
    val (icon, label) = when {
        pkg.packageId == "world" -> Icons.Default.Public to stringResource(R.string.map_pkg_world)
        pkg.packageId == "world-coasts" -> Icons.Default.Public to stringResource(R.string.map_pkg_world_coasts)
        else -> Icons.Default.Map to stringResource(R.string.map_pkg_region)
    }
    val (statusLabel, statusColor) = when (pkg.status) {
        "DOWNLOADING" -> stringResource(R.string.map_pkg_status_downloading) to MaterialTheme.colorScheme.primary
        "COMPLETE", "VERIFIED" -> stringResource(R.string.map_pkg_status_complete) to MaterialTheme.colorScheme.primary
        "FAILED" -> stringResource(R.string.map_pkg_status_failed) to MaterialTheme.colorScheme.error
        else -> stringResource(R.string.map_pkg_status_pending) to MaterialTheme.colorScheme.onSurfaceVariant
    }
    Surface(
        shape = RoundedCornerShape(16.dp),
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                Icon(
                    when (pkg.status) {
                        "COMPLETE", "VERIFIED" -> Icons.Default.CheckCircle
                        "FAILED" -> Icons.Default.ErrorOutline
                        "DOWNLOADING" -> icon
                        else -> Icons.Default.Schedule
                    },
                    null,
                    tint = statusColor,
                )
                Column(Modifier.weight(1f)) {
                    Text(label, style = MaterialTheme.typography.titleSmall)
                    Text(
                        stringResource(R.string.map_downloads_progress, size(pkg.downloadedBytes), size(pkg.bytes)),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Text(statusLabel, style = MaterialTheme.typography.labelLarge, color = statusColor)
            }
            if (pkg.status == "DOWNLOADING") {
                LinearProgressIndicator(
                    progress = { (pkg.downloadedBytes.toFloat() / pkg.bytes.coerceAtLeast(1)).coerceIn(0f, 1f) },
                    modifier = Modifier.fillMaxWidth(),
                    strokeCap = androidx.compose.ui.graphics.StrokeCap.Round,
                )
            }
            pkg.error?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            }
        }
    }
}

@Composable
private fun StatusRow(icon: ImageVector, tint: androidx.compose.ui.graphics.Color, title: String, subtitle: String?) {
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
        Icon(icon, null, tint = tint)
        Column {
            Text(title, style = MaterialTheme.typography.titleMedium)
            subtitle?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

@Composable
private fun InfoChip(icon: ImageVector, label: String) {
    // Informational only: a static pill, never a control that does nothing.
    Surface(shape = CircleShape, color = MaterialTheme.colorScheme.secondaryContainer) {
        Row(
            Modifier.padding(horizontal = 12.dp, vertical = 6.dp),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(icon, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSecondaryContainer)
            Text(label, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSecondaryContainer)
        }
    }
}

@Composable
private fun DownloadCard(content: @Composable ColumnScope.() -> Unit) {
    Surface(
        shape = RoundedCornerShape(24.dp),
        color = MaterialTheme.colorScheme.surfaceContainer,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp), content = content)
    }
}
