package com.anpfuel.app.ui.components

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.accessibility.liveRegionSemantics
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.state.DataReadinessState

/**
 * Offline/stale/sync status banner (BR-004, data freshness rules).
 */
@Composable
fun SyncStatusBanner(
    readiness: DataReadinessState,
    modifier: Modifier = Modifier,
) {
    val messageRes = when (readiness) {
        DataReadinessState.EMPTY -> null
        DataReadinessState.SYNCING -> R.string.banner_syncing
        DataReadinessState.PARTIAL -> R.string.banner_partial_data
        DataReadinessState.READY -> null
        DataReadinessState.STALE -> R.string.banner_stale_data
        DataReadinessState.ERROR -> R.string.banner_sync_error
    } ?: return

    val containerColor = when (readiness) {
        DataReadinessState.SYNCING -> MaterialTheme.colorScheme.primaryContainer
        DataReadinessState.STALE -> MaterialTheme.colorScheme.tertiaryContainer
        DataReadinessState.ERROR -> MaterialTheme.colorScheme.errorContainer
        else -> MaterialTheme.colorScheme.secondaryContainer
    }

    val contentColor = when (readiness) {
        DataReadinessState.SYNCING -> MaterialTheme.colorScheme.onPrimaryContainer
        DataReadinessState.STALE -> MaterialTheme.colorScheme.onTertiaryContainer
        DataReadinessState.ERROR -> MaterialTheme.colorScheme.onErrorContainer
        else -> MaterialTheme.colorScheme.onSecondaryContainer
    }

    Surface(
        modifier = modifier
            .fillMaxWidth()
            .liveRegionSemantics(),
        color = containerColor,
    ) {
        Text(
            text = stringResource(messageRes),
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
            style = MaterialTheme.typography.bodyMedium,
            color = contentColor,
        )
    }
}

@Composable
fun OfflineBanner(
    modifier: Modifier = Modifier,
) {
    Surface(
        modifier = modifier
            .fillMaxWidth()
            .liveRegionSemantics(),
        color = MaterialTheme.colorScheme.tertiaryContainer,
    ) {
        Text(
            text = stringResource(R.string.banner_offline),
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onTertiaryContainer,
        )
    }
}

@Preview(showBackground = true)
@Composable
private fun SyncStatusBannerPreview() {
    AnpFuelTheme {
        SyncStatusBanner(readiness = DataReadinessState.STALE)
    }
}

@Preview(showBackground = true)
@Composable
private fun OfflineBannerPreview() {
    AnpFuelTheme {
        OfflineBanner()
    }
}
