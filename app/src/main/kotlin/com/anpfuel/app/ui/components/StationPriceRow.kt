package com.anpfuel.app.ui.components

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.model.StationPriceUiModel
import com.anpfuel.app.ui.theme.AnpFuelTheme

/**
 * P24-T02 pure TalkBack join: price part (name + price, already localized via
 * `a11y_station_price_row`) plus optional recency part (already localized via
 * `stations_collected_at_label`). Kept pure for JVM RED → GREEN coverage; the
 * composable supplies the localized parts.
 */
fun stationRowTalkBackDescription(pricePart: String, collectedPart: String?): String =
    if (collectedPart.isNullOrBlank()) pricePart else "$pricePart, $collectedPart"

@Composable
fun StationPriceRow(
    station: StationPriceUiModel,
    onNavigate: () -> Unit,
    modifier: Modifier = Modifier,
) {
    // P24-T02: TalkBack must announce content (name + price + recency for
    // B-BR-C01 comprehension), not only the navigate verb. Role.Button keeps
    // the row actionable; a11y_station_price_row already exists in all locales.
    val pricePart = stringResource(
        R.string.a11y_station_price_row,
        station.displayName,
        station.priceFormatted,
    )
    val collectedPart = station.collectedAtLabel?.takeIf { it.isNotBlank() }?.let {
        stringResource(R.string.stations_collected_at_label, it)
    }
    val rowDescription = stationRowTalkBackDescription(pricePart, collectedPart)

    Card(
        modifier = modifier
            .fillMaxWidth()
            .semantics(mergeDescendants = true) {
                contentDescription = rowDescription
                role = Role.Button
            }
            .clickable(onClick = onNavigate),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        ),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(16.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.Top,
        ) {
            com.anpfuel.app.ui.stations.StationArtworkIcon(modifier = Modifier.size(44.dp))
            Column(
                modifier = Modifier.weight(1f).padding(horizontal = 12.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                Text(
                    text = station.displayName,
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.onSurface,
                )
                station.brand?.takeIf { it.isNotBlank() }?.let { brand ->
                    Text(
                        text = stringResource(R.string.stations_brand_label, brand),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Text(
                    text = station.address,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                station.collectedAtLabel?.let { collectedAt ->
                    Text(
                        text = stringResource(R.string.stations_collected_at_label, collectedAt),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Text(
                text = station.priceFormatted,
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.primary,
            )
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun StationPriceRowPreview() {
    AnpFuelTheme {
        StationPriceRow(
            station = StationPriceUiModel(
                cnpjDigits = "12345678000195",
                displayName = "Posto Centro",
                brand = "BR",
                address = "Rua XV de Novembro, 1000",
                priceFormatted = "R$ 5,79",
                collectedAtLabel = "Jun 10, 2026",
                navigationQuery = "Rua XV de Novembro, 1000, Curitiba - PR, Brazil",
            ),
            onNavigate = {},
        )
    }
}
