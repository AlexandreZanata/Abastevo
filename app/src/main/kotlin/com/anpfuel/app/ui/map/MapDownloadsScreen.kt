package com.anpfuel.app.ui.map

import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.ui.platform.testTag
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R

/**
 * Usable map-download screen: configured origin -> real envelope -> pinned-key
 * verification -> catalog with sizes -> explicit download (unique work) with
 * progress/cancel/retry -> verified activation -> observable state. Missing
 * configuration shows the specific pendency; network failure is retryable and
 * never presented as "no package" nor success.
 */
@Composable
fun MapDownloadsScreen(
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: MapDownloadsViewModel = hiltViewModel(),
) {
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    Column(
        modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(stringResource(R.string.map_downloads), style = MaterialTheme.typography.titleLarge)
        when {
            ui.loading -> Text(stringResource(R.string.map_loading))
            ui.unconfigured -> {
                Text(stringResource(R.string.map_dist_unconfigured))
                if (ui.missingUrl) Text("map_dist_base_url: missing")
                if (ui.missingRegion) Text("map_dist_region: missing")
                if (ui.missingKeys) Text("map_dist_keys: missing")
                if (ui.keysInvalid) Text("map_dist_keys: invalid")
                TextButton(onClick = { viewModel.refresh() }, modifier = Modifier.heightIn(min = 48.dp)) {
                    Text(stringResource(R.string.map_retry))
                }
            }
            ui.error != null -> {
                val retryDesc = stringResource(R.string.map_retry)
                Text(ui.error ?: "")
                TextButton(
                    onClick = { viewModel.retry() },
                    modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = retryDesc },
                    enabled = ui.retryable || ui.candidateGeneration != null,
                ) {
                    Text(retryDesc)
                }
            }
            else -> {
                if (ui.activeGeneration != null) {
                    Text(stringResource(R.string.map_downloads_active_generation, ui.activeGeneration ?: ""))
                    if (ui.published) {
                        Text(stringResource(R.string.map_published))
                    } else {
                        val activateLabel = stringResource(R.string.map_activate)
                        Text(stringResource(R.string.map_needs_activation))
                        Button(
                            onClick = { viewModel.activateActive() },
                            modifier = Modifier.heightIn(min = 48.dp).semantics {
                                contentDescription = activateLabel
                            },
                        ) {
                            Text(activateLabel)
                        }
                    }
                } else {
                    Text(stringResource(R.string.map_no_package))
                }
                val candidate = ui.candidateGeneration
                if (candidate != null) {
                    Text(stringResource(R.string.map_downloads_update_generation, candidate))
                    Text(
                        stringResource(
                            R.string.map_downloads_size_summary,
                            (ui.candidateBytes / 1024).toInt(),
                            ui.candidatePackages.size,
                        )
                    )
                    if (ui.candidateDependencies.isNotEmpty()) {
                        Text(
                            stringResource(
                                R.string.map_downloads_depends_on,
                                ui.candidateDependencies.joinToString(),
                            )
                        )
                    }
                    ui.candidatePackages.forEach { pkg ->
                        Text("${pkg.packageId}: ${pkg.downloadedBytes}/${pkg.bytes} ${pkg.status}")
                    }
                    if (ui.downloading) {
                        val cancelLabel = stringResource(R.string.map_cancel)
                        val total = ui.candidateBytes.coerceAtLeast(1)
                        LinearProgressIndicator(
                            progress = { (ui.downloadedBytes.toFloat() / total).coerceIn(0f, 1f) },
                            modifier = Modifier.fillMaxWidth(),
                        )
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            TextButton(
                                onClick = { viewModel.cancel() },
                                modifier = Modifier.heightIn(min = 48.dp).semantics {
                                    contentDescription = cancelLabel
                                },
                            ) {
                                Text(cancelLabel)
                            }
                        }
                    } else {
                        val downloadLabel = stringResource(R.string.map_download)
                        Button(
                            onClick = { viewModel.download() },
                            modifier = Modifier.testTag("map-download-start").heightIn(min = 48.dp).semantics {
                                contentDescription = downloadLabel
                            },
                        ) {
                            Text(downloadLabel)
                        }
                    }
                } else {
                    val checkUpdateLabel = stringResource(R.string.map_check_update)
                    Button(
                        onClick = { viewModel.checkForUpdate() },
                        modifier = Modifier.testTag("map-download-update").heightIn(min = 48.dp).semantics {
                            contentDescription = checkUpdateLabel
                        },
                    ) {
                        Text(checkUpdateLabel)
                    }
                }
            }
        }
        Button(onClick = onBack, modifier = Modifier.testTag("map-download-back").heightIn(min = 48.dp)) {
            Text(stringResource(R.string.map_downloads_ok))
        }
    }
}
