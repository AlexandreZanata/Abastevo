package com.anpfuel.app.ui.map

import android.text.format.Formatter
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpContainerDefaults

/** "Baixar mapa de <região> (N MB)" with Baixar / Agora não, for the opened city's region. */
@Composable
internal fun MapRegionSuggestionCard(
    suggestion: MapRegionSuggestion,
    onDownload: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    val title = suggestion.bytes?.let { bytes ->
        stringResource(
            R.string.map_region_suggestion,
            suggestion.region.name,
            Formatter.formatShortFileSize(context, bytes),
        )
    } ?: stringResource(R.string.map_region_suggestion_no_size, suggestion.region.name)
    val containerColor = MaterialTheme.colorScheme.surface
    Surface(
        modifier = modifier.fillMaxWidth().testTag("map-region-suggestion"),
        shape = AnpContainerDefaults.containerShape(),
        color = containerColor,
        border = AnpContainerDefaults.borderStroke(containerColor),
        shadowElevation = 3.dp,
    ) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 12.dp)) {
            Text(title, style = MaterialTheme.typography.titleSmall)
            Text(
                stringResource(R.string.map_region_suggestion_detail),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 2.dp),
            )
            Row(
                Modifier.fillMaxWidth().padding(top = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                TextButton(
                    onClick = onDismiss,
                    modifier = Modifier.heightIn(min = 48.dp).testTag("map-region-suggestion-dismiss"),
                ) {
                    Text(stringResource(R.string.map_region_not_now))
                }
                Button(
                    onClick = onDownload,
                    modifier = Modifier.heightIn(min = 48.dp).testTag("map-region-suggestion-download"),
                ) {
                    Text(stringResource(R.string.map_download))
                }
            }
        }
    }
}
