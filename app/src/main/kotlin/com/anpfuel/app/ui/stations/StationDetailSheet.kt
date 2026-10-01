package com.anpfuel.app.ui.stations

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.FuelProductLabel
import com.anpfuel.app.ui.components.PriceSourceKind
import com.anpfuel.app.ui.components.SourceTimeBadge
import com.anpfuel.app.ui.model.StationDetailUiModel
import com.anpfuel.app.ui.model.StationPriceUiModel
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P20-T03 — Station detail sheet.
 *
 * Community first: an explicit no-coverage slot (UNKNOWN until P04, never
 * an ANP substitution), then the dated ANP reference with stale/unknown
 * states, then route and update-price actions. Source badges carry
 * text + icon + screen-reader labels; condition qualifiers arrive with
 * backend community coverage.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StationDetailSheet(
    detail: StationDetailUiModel,
    fuelProduct: FuelProduct,
    locationLabel: String?,
    onDismiss: () -> Unit,
    onRoute: () -> Unit,
    onUpdatePrice: () -> Unit,
    modifier: Modifier = Modifier,
) {
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        modifier = modifier,
        contentWindowInsets = { WindowInsets(0) },
    ) {
        StationDetailSheetContent(
            detail = detail,
            fuelProduct = fuelProduct,
            locationLabel = locationLabel,
            onRoute = onRoute,
            onUpdatePrice = onUpdatePrice,
        )
    }
}

@Composable
internal fun StationDetailSheetContent(
    detail: StationDetailUiModel,
    fuelProduct: FuelProduct,
    locationLabel: String?,
    onRoute: () -> Unit,
    onUpdatePrice: () -> Unit,
    modifier: Modifier = Modifier,
) {
        Column(
            modifier = modifier
                .fillMaxWidth()
                .navigationBarsPadding()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                text = detail.station.displayName,
                style = MaterialTheme.typography.headlineSmall,
                color = MaterialTheme.colorScheme.onSurface,
                modifier = Modifier.semantics { heading() },
            )
            detail.station.brand?.takeIf { it.isNotBlank() }?.let { brand ->
                Text(
                    text = stringResource(R.string.stations_brand_label, brand),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                text = detail.station.address,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            locationLabel?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
                ),
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.station_detail_anp_title),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.semantics { heading() },
                    )
                    FuelProductLabel(product = fuelProduct)
                    Text(
                        text = detail.station.priceFormatted,
                        style = MaterialTheme.typography.headlineSmall,
                        color = MaterialTheme.colorScheme.primary,
                    )
                    if (detail.dateUnknown) {
                        Text(
                            text = stringResource(R.string.station_detail_unknown_date),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                        )
                    } else {
                        detail.station.collectedAtLabel?.let { collectedAt ->
                            Text(
                                text = stringResource(
                                    R.string.stations_collected_at_label,
                                    collectedAt,
                                ),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                    detail.surveyWeekLabel?.let { weekLabel ->
                        Text(
                            text = stringResource(R.string.station_detail_survey_week, weekLabel),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    if (detail.isStale) {
                        Text(
                            text = stringResource(R.string.station_detail_stale_badge),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                        )
                    }
                    SourceTimeBadge(
                        kind = PriceSourceKind.ANP_DATED,
                        detail = detail.surveyWeekLabel
                            ?: detail.station.collectedAtLabel.orEmpty(),
                    )
                }
            }

            Card(modifier = Modifier.fillMaxWidth()) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.community_section_title),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.semantics { heading() },
                    )
                    if (detail.communityDisputed) {
                        Text(
                            text = stringResource(R.string.station_detail_disputed),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                        )
                    } else {
                        Text(
                            text = stringResource(R.string.community_pending_p04),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                    }
                    Text(
                        text = stringResource(R.string.community_confidence_note),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Button(
                    onClick = onRoute,
                    modifier = Modifier.weight(1f),
                ) {
                    Text(text = stringResource(R.string.stations_navigate_action))
                }
                OutlinedButton(
                    onClick = onUpdatePrice,
                    modifier = Modifier.weight(1f),
                ) {
                    Text(text = stringResource(R.string.nav_update_price))
                }
            }
        }
}

@Preview(showBackground = true)
@Composable
private fun StationDetailSheetPreview() {
    AnpFuelTheme {
        StationDetailSheetContent(
            detail = StationDetailUiModel(
                station = StationPriceUiModel(
                    cnpjDigits = "12345678000195",
                    displayName = "Posto Centro",
                    brand = "BR",
                    address = "Rua XV de Novembro, 1000",
                    priceFormatted = "R$ 5,79",
                    collectedAtLabel = "10 de jun. de 2026",
                    navigationQuery = "Rua XV de Novembro, 1000, Curitiba - PR, Brazil",
                ),
                surveyWeekLabel = "7–13 de jun. de 2026",
                isStale = true,
                dateUnknown = false,
                communityDisputed = false,
            ),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            locationLabel = "Curitiba, PR",
            onRoute = {},
            onUpdatePrice = {},
        )
    }
}
