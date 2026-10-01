package com.anpfuel.app.community

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R

/**
 * P10-T06 — Distinct official/community price sections (B-BR-001,
 * PRODUCT_CONTRACT).
 *
 * The official (ANP/backend) section stays visually separate from the
 * community (beta) section; nothing is blended into one number. Every
 * row carries source, condition (+qualifier), unit and age. Community is
 * UNKNOWN until P04; stale fallbacks are labelled STALE; confidence
 * renders as support text, never a guarantee badge. Flag OFF renders
 * nothing (caller keeps the ANP panel intact).
 */
@Composable
fun CommunityPricePanel(
    state: CommunityPricesUiState,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    when (state) {
        is CommunityPricesUiState.Disabled -> Unit
        is CommunityPricesUiState.Loading -> {
            CircularProgressIndicator(modifier = modifier.padding(16.dp))
        }
        is CommunityPricesUiState.Content -> {
            PriceSections(
                titleRes = R.string.community_section_title,
                rows = state.rows,
                source = state.source,
                version = state.version,
                stale = false,
                onRetry = onRetry,
                modifier = modifier,
            )
        }
        is CommunityPricesUiState.Stale -> {
            PriceSections(
                titleRes = R.string.community_section_title,
                rows = state.rows,
                source = state.source,
                version = state.version,
                stale = true,
                onRetry = onRetry,
                modifier = modifier,
            )
        }
        is CommunityPricesUiState.Unavailable -> {
            Card(modifier = modifier.fillMaxWidth()) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.community_section_title),
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        text = stringResource(R.string.community_unavailable_retry),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    Button(onClick = onRetry, modifier = Modifier.fillMaxWidth()) {
                        Text(text = stringResource(R.string.action_retry))
                    }
                }
            }
        }
    }
}

@Composable
private fun PriceSections(
    titleRes: Int,
    rows: List<CommunityPriceRowUiModel>,
    source: String,
    version: String,
    stale: Boolean,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Card(modifier = modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = stringResource(titleRes),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = stringResource(R.string.community_source_version, source, version),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (stale) {
                Text(
                    text = stringResource(R.string.community_stale_label),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (rows.isEmpty()) {
                Text(
                    text = stringResource(R.string.community_pending_p04),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            rows.forEach { row ->
                CommunityPriceRow(row = row)
            }
            Text(
                text = stringResource(R.string.community_confidence_note),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (stale) {
                Button(onClick = onRetry, modifier = Modifier.fillMaxWidth()) {
                    Text(text = stringResource(R.string.action_retry))
                }
            }
        }
    }
}

@Composable
private fun CommunityPriceRow(row: CommunityPriceRowUiModel) {
    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        val officialText = if (row.officialAmountMilli == null) {
            stringResource(R.string.community_official_empty)
        } else {
            CommunityPriceDisplay.formatMilliBrl(row.officialAmountMilli) + " / " + row.unit
        }
        Text(
            text = row.fuelProductWire + " · " + officialText,
            style = MaterialTheme.typography.bodyMedium,
        )
        Text(
            text = stringResource(
                R.string.community_official_detail,
                row.officialSource ?: "—",
                row.conditionLabel,
                row.officialCollectedOn ?: "—",
            ),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            text = stringResource(
                R.string.community_availability_detail,
                CommunityPriceDisplay.availabilityLabel(row.availability),
                CommunityPriceDisplay.freshnessLabel(row.freshness),
            ),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}
