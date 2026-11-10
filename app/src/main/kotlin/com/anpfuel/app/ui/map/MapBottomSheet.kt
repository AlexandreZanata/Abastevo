package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Bookmark
import androidx.compose.material.icons.filled.BookmarkBorder
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.EvStation
import androidx.compose.material.icons.filled.LocalGasStation
import androidx.compose.material.icons.filled.LocationOn
import androidx.compose.material.icons.filled.Navigation
import androidx.compose.material.icons.filled.Storefront
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.community.CommunityPriceDisplay
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.mapper.SurveyWeekFormatter
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.FilterChip
import com.anpfuel.app.ui.designsystem.OutlinedButton
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.maps.engine.MapStationLocationQuality
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale

@Composable
internal fun localizedMapProductLabel(rawProduct: String): String {
    val norm = rawProduct.trim().uppercase(Locale.ROOT)
    val fuelProduct = when (norm) {
        "ETHANOL" -> FuelProduct.ETHANOL
        "GASOLINE_REGULAR" -> FuelProduct.GASOLINE_REGULAR
        "GASOLINE_ADDITIVED", "GASOLINE_PREMIUM" -> FuelProduct.GASOLINE_PREMIUM
        "GASOLINE_PREMIUM_GRADE" -> FuelProduct.GASOLINE_PREMIUM_GRADE
        "DIESEL_S500" -> FuelProduct.DIESEL_S500
        "DIESEL_S10" -> FuelProduct.DIESEL_S10
        "CNG" -> FuelProduct.CNG
        "LPG_P13" -> FuelProduct.LPG_P13
        else -> null
    }
    if (fuelProduct != null) {
        return stringResource(FuelProductI18n.toStringRes(fuelProduct))
    }
    return when (norm) {
        "CCS2" -> "CCS2"
        "TYPE2" -> "Type 2"
        "CHADEMO" -> "CHAdeMO"
        "NACS" -> "NACS"
        "UNKNOWN" -> stringResource(R.string.charging_explore_unknown)
        else -> rawProduct
    }
}

@Composable
internal fun localizedMapConditionLabel(conditionKind: String, qualifier: String?): String {
    val norm = conditionKind.trim().uppercase(Locale.ROOT)
    val label = when (norm) {
        "STANDARD" -> R.string.charging_community_condition_standard
        "OFF_PEAK" -> R.string.charging_community_condition_off_peak
        "PEAK" -> R.string.charging_community_condition_peak
        "MEMBER" -> R.string.charging_community_condition_member
        "UNKNOWN" -> R.string.charging_community_condition_unknown
        "CASH" -> R.string.map_sheet_condition_cash
        "LOYALTY" -> R.string.map_sheet_condition_loyalty
        "APP" -> R.string.map_sheet_condition_app
        "OTHER" -> R.string.map_sheet_condition_other
        else -> return CommunityPriceDisplay.formatCondition(norm, qualifier)
    }
    return listOfNotNull(stringResource(label), qualifier?.takeIf { it.isNotBlank() }).joinToString(" · ")
}

/**
 * MAP-19 quick sheet: name, services, exact source/product/unit/price/condition/date,
 * quality/access and real actions (route, profile, report). ANP absence is
 * explicit; community never mixes silently; EV in ANP offers explicit switch.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MapBottomSheet(
    viewModel: MapSheetViewModel,
    onOpenProfile: (stationUuid: String) -> Unit,
    onRoute: (stationUuid: String) -> Unit,
    onReport: (stationUuid: String) -> Unit,
    onSwitchToCommunity: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
    routeError: Boolean = false,
    onSelectSource: (MapPriceSource) -> Unit = {},
    isFavorite: Boolean = false,
    onToggleFavorite: (() -> Unit)? = null,
) {
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    val marker = ui.marker ?: return
    val chargingOnly = marker.charging && !marker.fuel
    val serviceLabel = stringResource(when {
        marker.fuel && marker.charging -> R.string.map_filter_fuel_charging
        chargingOnly -> R.string.map_filter_charging
        else -> R.string.map_filter_fuel
    })
    val locationQualityLabel = when (marker.locationQuality) {
        MapStationLocationQuality.REVIEWED -> stringResource(R.string.map_location_reviewed)
        MapStationLocationQuality.OSM_MAPPED -> stringResource(R.string.map_location_osm_mapped)
        MapStationLocationQuality.ANP_REPORTED -> stringResource(R.string.map_location_anp_reported)
        MapStationLocationQuality.CHARGING_SOURCE -> stringResource(R.string.map_location_charging_source)
        MapStationLocationQuality.ADDRESS_POINT -> stringResource(R.string.map_location_address_point)
        MapStationLocationQuality.CITY_CENTROID -> stringResource(R.string.map_location_city_centroid)
        MapStationLocationQuality.UNKNOWN -> stringResource(R.string.map_location_unknown)
    }
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        modifier = modifier,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
    ) {
        Column(
            modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState())
                .padding(start = 20.dp, end = 20.dp, bottom = 12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Surface(
                        shape = MaterialTheme.shapes.medium,
                        color = MaterialTheme.colorScheme.secondaryContainer,
                        contentColor = MaterialTheme.colorScheme.onSecondaryContainer,
                    ) {
                        Icon(
                            if (chargingOnly) Icons.Default.EvStation else Icons.Default.LocalGasStation,
                            contentDescription = null,
                            modifier = Modifier.padding(10.dp).size(20.dp),
                        )
                    }
                    Text(
                        serviceLabel,
                        style = MaterialTheme.typography.labelLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.weight(1f).padding(horizontal = 12.dp),
                    )
                    IconButton(onClick = onDismiss, modifier = Modifier.testTag("map-sheet-close")) {
                        Icon(Icons.Default.Close, stringResource(R.string.feed_close))
                    }
                }
                Text(
                    marker.title,
                    style = MaterialTheme.typography.titleLarge,
                    modifier = Modifier.semantics { heading() },
                )
                marker.legalName?.let { name ->
                    Text(name, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                if (marker.subtitle.isNotBlank()) {
                    Text(marker.subtitle, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                if (marker.confirmedByTwoSources) {
                    Text(
                        stringResource(R.string.charging_place_two_sources),
                        style = MaterialTheme.typography.labelLarge,
                        color = MaterialTheme.colorScheme.primary,
                    )
                }
            }

            StationPricePanel(ui, chargingOnly, onSelectSource, onSwitchToCommunity)

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.Top) {
                Icon(
                    Icons.Default.LocationOn,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.size(18.dp),
                )
                Text(locationQualityLabel, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            if (!marker.canRouteByCoordinates) {
                Text(
                    stringResource(R.string.map_sheet_route_unavailable),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag("map-sheet-route-unavailable"),
                )
            }
            if (routeError) {
                Text(
                    stringResource(R.string.map22_error),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.testTag("map-sheet-route-error"),
                )
            }

            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Button(
                    onClick = { onRoute(marker.uuid) },
                    enabled = marker.canRouteByCoordinates,
                    modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("map-sheet-route"),
                ) {
                    StationActionLabel(Icons.Default.Navigation, stringResource(R.string.map_route_go))
                }
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(
                        onClick = { onOpenProfile(marker.uuid) },
                        modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-sheet-profile"),
                    ) {
                        StationActionLabel(Icons.Default.Storefront, stringResource(R.string.map_sheet_profile))
                    }
                    OutlinedButton(
                        onClick = { onReport(marker.uuid) },
                        modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-sheet-report"),
                    ) {
                        StationActionLabel(
                            Icons.Default.Edit,
                            stringResource(if (chargingOnly) R.string.map_sheet_charge_report else R.string.map_sheet_report),
                        )
                    }
                }
                if (onToggleFavorite != null) {
                    TextButton(
                        onClick = onToggleFavorite,
                        modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("map-sheet-favorite"),
                    ) {
                        StationActionLabel(
                            if (isFavorite) Icons.Default.Bookmark else Icons.Default.BookmarkBorder,
                            stringResource(if (isFavorite) R.string.map25_favorite_remove else R.string.map25_favorite_add),
                        )
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun StationPricePanel(
    ui: MapSheetUi,
    chargingOnly: Boolean,
    onSelectSource: (MapPriceSource) -> Unit,
    onSwitchToCommunity: () -> Unit,
) {
    val container = MaterialTheme.colorScheme.surfaceContainerHigh
    val secondaryText = MaterialTheme.colorScheme.onSurfaceVariant
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        color = container,
        border = AnpContainerDefaults.borderStroke(container),
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.map_sheet_price_source), style = MaterialTheme.typography.labelLarge, color = secondaryText)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                MapPriceSource.entries.forEach { source ->
                    FilterChip(
                        selected = ui.source == source,
                        onClick = { onSelectSource(source) },
                        label = {
                            Text(stringResource(if (source == MapPriceSource.ANP) R.string.map_source_anp else R.string.map_source_community))
                        },
                        modifier = Modifier.heightIn(min = 48.dp).testTag("map-sheet-source-${source.name.lowercase(Locale.ROOT)}"),
                    )
                }
            }

            val price = ui.price
            when {
                ui.loading -> {
                    Text(stringResource(R.string.map_sheet_loading_price), style = MaterialTheme.typography.bodyMedium)
                    LinearProgressIndicator(Modifier.fillMaxWidth())
                }
                price != null -> StationPriceDetails(price)
                ui.absentReason == "anp-no-tariff" -> {
                    Text(stringResource(R.string.map_sheet_anp_no_tariff), style = MaterialTheme.typography.bodyMedium)
                    TextButton(
                        onClick = onSwitchToCommunity,
                        modifier = Modifier.heightIn(min = 48.dp).testTag("map-sheet-switch-community"),
                    ) { Text(stringResource(R.string.map_sheet_show_community)) }
                }
                else -> Text(
                    stringResource(
                        if (ui.absentReason == "load-failed") R.string.map_sheet_price_load_failed else R.string.map_sheet_no_price,
                    ),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            if (chargingOnly) {
                Text(
                    stringResource(R.string.charging_open_sources),
                    style = MaterialTheme.typography.bodySmall,
                    color = secondaryText,
                    modifier = Modifier.testTag("map-sheet-source-label"),
                )
            }
        }
    }
}

@Composable
private fun StationPriceDetails(price: SheetPrice) {
    val locale = LocalConfiguration.current.locales[0]
    val secondaryText = MaterialTheme.colorScheme.onSurfaceVariant
    val unitStyle = MaterialTheme.typography.bodyLarge.toSpanStyle().copy(color = secondaryText)
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(localizedMapProductLabel(price.product), style = MaterialTheme.typography.titleSmall)
        Text(
            buildAnnotatedString {
                append(price.amount)
                withStyle(unitStyle) { append(" / ${price.unit}") }
            },
            style = MaterialTheme.typography.headlineSmall,
            modifier = Modifier.testTag("map-sheet-price"),
        )
        Text(
            localizedMapConditionLabel(price.conditionKind, price.conditionQualifier),
            style = MaterialTheme.typography.labelMedium,
            color = if (price.isConditional) MaterialTheme.colorScheme.tertiary else secondaryText,
            modifier = Modifier.testTag("map-sheet-condition"),
        )
        price.surveyWeek?.let { week ->
            Text(
                stringResource(R.string.station_detail_survey_week, SurveyWeekFormatter.formatRangeCompact(week, locale)),
                style = MaterialTheme.typography.bodySmall,
                color = secondaryText,
            )
        }
        price.observedOn?.let { day ->
            Text(
                stringResource(
                    if (price.source == MapPriceSource.ANP) R.string.stations_collected_at_label else R.string.map_sheet_observed_on,
                    day.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM).withLocale(locale)),
                ),
                style = MaterialTheme.typography.bodySmall,
                color = secondaryText,
            )
        }
        if (price.stale) {
            Text(stringResource(R.string.map_sheet_cached_price), style = MaterialTheme.typography.labelSmall, color = secondaryText)
        }
    }
}

@Composable
private fun StationActionLabel(icon: ImageVector, label: String) {
    Icon(icon, contentDescription = null, modifier = Modifier.size(18.dp))
    Spacer(Modifier.width(8.dp))
    Text(label)
}
