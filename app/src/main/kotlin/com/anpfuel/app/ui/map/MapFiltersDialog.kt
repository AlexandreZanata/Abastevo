package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R

@Composable
fun MapFiltersDialog(
    source: MapPriceSource,
    onSelectSource: (MapPriceSource) -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        confirmButton = {
            TextButton(onClick = onDismiss) { Text("OK") }
        },
        title = { Text(stringResource(R.string.map_filters)) },
        text = {
            Column(modifier = Modifier.fillMaxWidth().padding(4.dp)) {
                FilterChip(
                    selected = source == MapPriceSource.ANP,
                    onClick = { onSelectSource(MapPriceSource.ANP) },
                    label = { Text(stringResource(R.string.map_source_anp)) },
                )
                FilterChip(
                    selected = source == MapPriceSource.COMMUNITY,
                    onClick = { onSelectSource(MapPriceSource.COMMUNITY) },
                    label = { Text(stringResource(R.string.map_source_community)) },
                )
            }
        },
    )
}
