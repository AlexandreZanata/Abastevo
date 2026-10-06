package com.anpfuel.app.ui.home

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.navigation.Routes
import com.anpfuel.app.ui.components.FuelProductIcon
import com.anpfuel.app.ui.model.AveragePriceUiModel
import com.anpfuel.app.ui.theme.FuelProductTint

/** These are municipal ANP averages, never synthetic nearby station offers. */
@Composable
internal fun HomeReferencePriceCard(price: AveragePriceUiModel, darkTheme: Boolean, onClick: () -> Unit) {
    val fuelLabel = stringResource(FuelProductI18n.toStringRes(price.fuelProduct))
    val average = price.averageFormatted ?: stringResource(R.string.prices_not_available)
    val description = stringResource(R.string.a11y_fuel_price_card, fuelLabel, average)
    Card(
        onClick = onClick,
        modifier = Modifier.fillMaxWidth().semantics(mergeDescendants = true) { contentDescription = description },
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
    ) {
        Row(
            modifier = Modifier.padding(16.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            FuelProductIcon(price.fuelProduct, size = 36.dp, contentDescription = null)
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(fuelLabel, style = MaterialTheme.typography.bodyMedium)
                Text(
                    average,
                    style = MaterialTheme.typography.headlineSmall,
                    fontWeight = FontWeight.Bold,
                    color = FuelProductTint.colorFor(price.fuelProduct, darkTheme),
                )
                price.stationCount?.let { count ->
                    Text(
                        stringResource(R.string.prices_station_count, count),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Surface(
                color = MaterialTheme.colorScheme.primaryContainer,
                contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
                shape = RoundedCornerShape(8.dp),
            ) {
                Text(
                    stringResource(R.string.home_reference_source),
                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                    style = MaterialTheme.typography.labelSmall,
                )
            }
            Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = null)
        }
    }
}

@Composable
internal fun HomeShortcuts(onNavigate: (String) -> Unit) {
    if (LocalDensity.current.fontScale > 1.3f) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            HomeShortcut(R.drawable.ic_home_stations, R.string.home_shortcut_stations, R.string.home_shortcut_stations_description,
                Modifier.fillMaxWidth()) { onNavigate(Routes.STATIONS) }
            HomeShortcut(R.drawable.ic_home_history, R.string.home_shortcut_history, R.string.home_shortcut_history_description,
                Modifier.fillMaxWidth()) { onNavigate(Routes.HISTORY) }
        }
    } else {
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            HomeShortcut(R.drawable.ic_home_stations, R.string.home_shortcut_stations, R.string.home_shortcut_stations_description,
                Modifier.weight(1f)) { onNavigate(Routes.STATIONS) }
            HomeShortcut(R.drawable.ic_home_history, R.string.home_shortcut_history, R.string.home_shortcut_history_description,
                Modifier.weight(1f)) { onNavigate(Routes.HISTORY) }
        }
    }
}

@Composable
private fun HomeShortcut(icon: Int, title: Int, description: Int, modifier: Modifier, onClick: () -> Unit) {
    Card(
        onClick = onClick,
        modifier = modifier,
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
    ) {
        Row(
            modifier = Modifier.padding(14.dp),
            horizontalArrangement = Arrangement.spacedBy(10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(painterResource(icon), contentDescription = null, tint = Color.Unspecified, modifier = Modifier.size(28.dp))
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(stringResource(title), style = MaterialTheme.typography.labelLarge, fontWeight = FontWeight.Bold)
                Text(stringResource(description), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}
