package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R

/**
 * Minimal downloads entry: shows the active public generation from Room.
 * Real download scheduling lives in MAP-15 (`MapDownloadScheduler`);
 * this screen proves the active generation reaches the UI.
 */
@Composable
fun MapDownloadsScreen(
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: MapDownloadsViewModel = hiltViewModel(),
) {
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    Column(
        modifier.fillMaxSize().padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(stringResource(R.string.map_downloads), style = MaterialTheme.typography.titleLarge)
        when {
            ui.loading -> Text(stringResource(R.string.map_loading))
            ui.activeGeneration == null -> {
                Text(stringResource(R.string.map_no_package))
                Button(onClick = { viewModel.refresh() }) {
                    Text(stringResource(R.string.map_download))
                }
            }
            else -> {
                Text("Active: ${ui.activeGeneration}")
                Text("Packages: ${ui.packageCount}")
            }
        }
        Button(onClick = onBack) { Text("OK") }
    }
}
