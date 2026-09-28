package com.anpfuel.app.ui.components

import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.EventBusy
import androidx.compose.material.icons.outlined.LocationOff
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.mapper.DataAvailabilityI18n
import com.anpfuel.app.ui.accessibility.headingSemantics
import com.anpfuel.domain.valueobject.DataAvailability

@Composable
fun Br010EmptyState(dataAvailability: DataAvailability, modifier: Modifier = Modifier, operationalNote: String? = null) {
    Column(modifier.fillMaxWidth().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Icon(br010Icon(dataAvailability), null, Modifier.size(48.dp), MaterialTheme.colorScheme.onSurfaceVariant)
        Text(stringResource(R.string.state_empty_title), style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.headingSemantics())
        Text(stringResource(DataAvailabilityI18n.toEmptyStateStringRes(dataAvailability)), style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
        operationalNote?.let { Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant) }
    }
}
private fun br010Icon(a: DataAvailability): ImageVector = when (a) {
    DataAvailability.NEVER_IN_ANP -> Icons.Outlined.LocationOff
    else -> Icons.Outlined.EventBusy
}
