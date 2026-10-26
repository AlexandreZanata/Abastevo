package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.PriceFormatter
import com.anpfuel.domain.ev.EvAccess
import com.anpfuel.domain.ev.EvAlternative
import com.anpfuel.domain.ev.EvHoursStatus
import com.anpfuel.domain.ev.EvIssueCode
import com.anpfuel.domain.ev.EvPlanStatus
import com.anpfuel.domain.ev.EvPlannedStop
import com.anpfuel.domain.ev.EvTariff
import com.anpfuel.domain.ev.EvTripPlan
import com.anpfuel.domain.ev.EnergyEstimate
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.domain.valueobject.Propulsion
import java.math.BigDecimal
import java.math.RoundingMode
import java.text.NumberFormat
import java.util.Currency
import java.util.Locale

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun EvTripSection(
    origin: RoutePoint?,
    waypoints: List<RoutePoint>,
    destination: RoutePoint?,
    avoid: Set<RoadAvoidance>,
    savedMessage: String?,
    onSavePlan: ((String, List<Pair<Double, Double>>, Set<String>) -> Unit)?,
    onDownloads: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: EvTripViewModel = hiltViewModel(),
) {
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]
    val vehicle = ui.selectedVehicle
    val evTitle = stringResource(R.string.ev_title)

    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        HorizontalDivider()
        Text(stringResource(R.string.ev_title), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.ev_assumptions), style = MaterialTheme.typography.bodySmall)

        if (ui.vehicles.isEmpty()) {
            Text(stringResource(R.string.ev_no_vehicle), color = MaterialTheme.colorScheme.error)
        } else {
            Text(stringResource(R.string.ev_vehicle), style = MaterialTheme.typography.labelLarge)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ui.vehicles.forEachIndexed { index, option ->
                    FilterChip(
                        selected = option.id == ui.selectedVehicleId,
                        onClick = { viewModel.selectVehicle(option.id) },
                        label = { Text(option.name) },
                        modifier = Modifier.heightIn(min = 48.dp).testTag("ev-vehicle-$index"),
                    )
                }
            }
            if (vehicle != null) {
                Text(
                    stringResource(
                        R.string.ev_capacity,
                        formatDecimal(BigDecimal.valueOf(vehicle.capacityWh).movePointLeft(3), locale, 1),
                    ),
                )
                Text(
                    stringResource(
                        R.string.ev_connector,
                        vehicle.connector?.name ?: stringResource(R.string.ev_unknown),
                    ),
                )
            }
            if (vehicle?.connector == null) {
                Text(stringResource(R.string.ev_no_connector), color = MaterialTheme.colorScheme.error)
            }

            NumberField(
                value = ui.socInput,
                onValueChange = viewModel::onSocChanged,
                label = stringResource(R.string.ev_soc),
                tag = "ev-soc",
            )
            NumberField(
                value = ui.reserveInput,
                onValueChange = viewModel::onReserveChanged,
                label = stringResource(R.string.ev_reserve),
                tag = "ev-reserve",
            )
            NumberField(
                value = ui.consumptionInput,
                onValueChange = viewModel::onConsumptionChanged,
                label = stringResource(R.string.ev_consumption),
                tag = "ev-consumption",
            )
            NumberField(
                value = ui.targetInput,
                onValueChange = viewModel::onTargetChanged,
                label = stringResource(R.string.ev_target),
                tag = "ev-target",
            )
            if (vehicle?.propulsion == Propulsion.PHEV) {
                NumberField(
                    value = ui.phevShareInput,
                    onValueChange = viewModel::onPhevShareChanged,
                    label = stringResource(R.string.ev_phev_share),
                    tag = "ev-phev-share",
                )
            }

            val canPlan = origin != null && destination != null && !ui.planning && vehicle?.connector != null
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(
                    onClick = {
                        if (origin != null && destination != null) {
                            viewModel.plan(origin, waypoints, destination, avoid, Locale.getDefault().language)
                        }
                    },
                    enabled = canPlan,
                    modifier = Modifier.heightIn(min = 48.dp).testTag("ev-plan"),
                ) { Text(stringResource(R.string.ev_plan)) }
                if (ui.planning) {
                    OutlinedButton(
                        onClick = viewModel::cancelPlan,
                        modifier = Modifier.heightIn(min = 48.dp),
                    ) { Text(stringResource(R.string.ev_cancel)) }
                }
            }
            if (ui.planning) LinearProgressIndicator(Modifier.fillMaxWidth())
            ui.error?.let {
                Text(errorText(it), color = MaterialTheme.colorScheme.error, modifier = Modifier.testTag("ev-error"))
            }
            if (ui.replannedAfterFailure) {
                Text(
                    stringResource(R.string.ev_replanned),
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.testTag("ev-replanned"),
                )
            }
            ui.plan?.let { plan ->
                PlanContent(
                    plan = plan,
                    locale = locale,
                    onFailPrimary = viewModel::markPrimaryFailed,
                    onDownloads = onDownloads,
                    onSavePlan = if (onSavePlan != null && origin != null && destination != null) {
                        {
                            val name = origin.label.ifBlank { evTitle }
                                .take(80) + " - " + destination.label.ifBlank { "" }.take(40)
                            onSavePlan(
                                name.take(120),
                                (listOf(origin) + waypoints + listOf(destination)).map { it.latitude to it.longitude },
                                avoid.map { it.name }.toSet(),
                            )
                        }
                    } else {
                        null
                    },
                    savedMessage = savedMessage,
                )
            }
        }
    }
}

@Composable
private fun NumberField(value: String, onValueChange: (String) -> Unit, label: String, tag: String) {
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        label = { Text(label) },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
        modifier = Modifier.fillMaxWidth().testTag(tag),
    )
}

@Composable
private fun PlanContent(
    plan: EvTripPlan,
    locale: Locale,
    onFailPrimary: () -> Unit,
    onDownloads: () -> Unit,
    onSavePlan: (() -> Unit)?,
    savedMessage: String?,
) {
    if (plan.status != EvPlanStatus.PLANNED) {
        Text(
            statusText(plan.status),
            color = MaterialTheme.colorScheme.error,
            modifier = Modifier.testTag("ev-plan-status"),
        )
    }
    val mainRoad = plan.mainRoute?.road as? com.anpfuel.domain.ev.EvRoadLeg.Ready
    if (mainRoad != null) {
        Text(
            stringResource(
                R.string.ev_route,
                formatDecimal(BigDecimal.valueOf(mainRoad.distanceMeters).movePointLeft(3), locale, 1),
                formatDuration(mainRoad.durationSeconds, locale),
            ),
        )
    }
    plan.issues.map { it.code }.distinct().take(8).forEach { code ->
        issueText(code)?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
    }
    plan.stops.forEachIndexed { index, stop ->
        StopCard(
            index = index,
            stop = stop,
            locale = locale,
            onFailPrimary = if (index == 0) onFailPrimary else null,
        )
    }
    val arrival = plan.arrivalSocBp ?: plan.mainRoute?.arrivalSocBp
    if (arrival != null) {
        Text(stringResource(R.string.ev_final_arrival, socEstimateText(arrival, locale)))
    }
    Text(
        stringResource(
            R.string.ev_source,
            plan.assumptions.sourceRevision ?: stringResource(R.string.ev_unknown),
            plan.assumptions.generation?.toString() ?: stringResource(R.string.ev_unknown),
        ),
        style = MaterialTheme.typography.bodySmall,
    )
    Text(stringResource(R.string.ev_no_live), style = MaterialTheme.typography.bodySmall)
    if (plan.issues.any { it.code == EvIssueCode.MISSING_MAPS || it.code == EvIssueCode.SNAPSHOT_MISSING || it.code == EvIssueCode.COVERAGE_UNKNOWN }) {
        OutlinedButton(
            onClick = onDownloads,
            modifier = Modifier.heightIn(min = 48.dp).testTag("ev-downloads"),
        ) { Text(stringResource(R.string.ev_downloads)) }
    }
    if (onSavePlan != null) {
        OutlinedButton(
            onClick = onSavePlan,
            modifier = Modifier.heightIn(min = 48.dp).testTag("ev-save"),
        ) { Text(stringResource(R.string.ev_save)) }
        if (savedMessage == "trip-saved") {
            Text(stringResource(R.string.ev_saved), modifier = Modifier.testTag("ev-saved"))
        }
    }
}

@Composable
private fun StopCard(index: Int, stop: EvPlannedStop, locale: Locale, onFailPrimary: (() -> Unit)?) {
    Card(Modifier.fillMaxWidth().testTag("ev-stop-$index")) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(stringResource(R.string.ev_stop, index + 1), style = MaterialTheme.typography.titleSmall)
            Text(
                stop.candidate.siteName,
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.testTag("ev-stop-site-$index"),
            )
            Text(stringResource(R.string.ev_arrival_soc, formatPercent(stop.arrivalSocBp, locale)))
            Text(stringResource(R.string.ev_target_soc, formatPercent(stop.targetSocBp, locale)))
            Text(
                stringResource(
                    R.string.ev_charge_energy,
                    formatDecimal(BigDecimal.valueOf(stop.chargeEnergyWh).movePointLeft(3), locale, 1),
                ),
            )
            Text(stringResource(R.string.ev_charge_time, durationEstimateText(stop.minimumChargeSeconds, locale)))
            Text(stringResource(R.string.ev_charge_cost, moneyText(stop.estimatedCostMilliBrl, locale)))
            Text(
                stringResource(
                    R.string.ev_power,
                    stop.candidate.powerW?.let { "${formatDecimal(BigDecimal.valueOf(it).movePointLeft(3), locale, 0)} kW" }
                        ?: stringResource(R.string.ev_unknown),
                ),
            )
            Text(stringResource(R.string.ev_tariff, tariffText(stop, locale)))
            if (stop.candidate.access == EvAccess.UNKNOWN) Text(stringResource(R.string.ev_access_unknown))
            if (stop.hoursStatus == EvHoursStatus.UNKNOWN) Text(stringResource(R.string.ev_hours_unknown))
            if (stop.candidate.observedAt == null) Text(stringResource(R.string.ev_report_unknown))
            if (stop.candidate.provenance == com.anpfuel.domain.ev.EvProvenance.COMMUNITY_REPORT) {
                Text(stringResource(R.string.ev_provenance_community))
            } else {
                Text(stringResource(R.string.ev_provenance_static))
            }
            stop.issues.filter { it != EvIssueCode.ACCESS_UNKNOWN && it != EvIssueCode.HOURS_UNKNOWN && it != EvIssueCode.POWER_UNKNOWN && it != EvIssueCode.TARIFF_UNKNOWN && it != EvIssueCode.REPORT_DATE_UNKNOWN }
                .forEach { code -> issueText(code)?.let { Text(it, style = MaterialTheme.typography.bodySmall) } }
            stop.alternative?.let { alt ->
                AlternativeContent(alt, locale)
            } ?: run {
                Text(
                    stringResource(R.string.ev_no_alternative),
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.testTag("ev-no-alternative"),
                )
            }
            if (onFailPrimary != null) {
                OutlinedButton(
                    onClick = onFailPrimary,
                    modifier = Modifier.heightIn(min = 48.dp).testTag("ev-fail-primary"),
                ) { Text(stringResource(R.string.ev_fail_primary)) }
            }
        }
    }
}

@Composable
private fun AlternativeContent(alternative: EvAlternative, locale: Locale) {
    Text(
        stringResource(R.string.ev_alternative, alternative.candidate.siteName),
        style = MaterialTheme.typography.titleSmall,
        modifier = Modifier.testTag("ev-alternative"),
    )
    Text(stringResource(R.string.ev_alternative_arrival, socEstimateText(alternative.arrivalSocBp, locale)))
    Text(stringResource(R.string.ev_operation_unknown), style = MaterialTheme.typography.bodySmall)
    alternative.issues.distinct().forEach { code -> issueText(code)?.let { Text(it, style = MaterialTheme.typography.bodySmall) } }
}

@Composable
private fun statusText(status: EvPlanStatus): String = when (status) {
    EvPlanStatus.PLANNED -> stringResource(R.string.ev_status_planned)
    EvPlanStatus.NO_ROUTE -> stringResource(R.string.ev_status_no_route)
    EvPlanStatus.INSUFFICIENT_DATA -> stringResource(R.string.ev_status_insufficient_data)
    EvPlanStatus.NO_CANDIDATES -> stringResource(R.string.ev_status_no_candidates)
    EvPlanStatus.INSUFFICIENT_ENERGY -> stringResource(R.string.ev_status_insufficient_energy)
    EvPlanStatus.BUDGET_EXHAUSTED -> stringResource(R.string.ev_status_budget)
    EvPlanStatus.CANCELLED -> stringResource(R.string.ev_status_cancelled)
    EvPlanStatus.INVALID_INPUT -> stringResource(R.string.ev_status_invalid)
}

@Composable
private fun errorText(code: String): String = when (code) {
    "no-electric-vehicle", "vehicle-load-failed" -> stringResource(R.string.ev_no_vehicle)
    "vehicle-connector-missing" -> stringResource(R.string.ev_no_connector)
    "invalid-soc" -> stringResource(R.string.ev_invalid_soc)
    "invalid-reserve" -> stringResource(R.string.ev_invalid_reserve)
    "invalid-consumption" -> stringResource(R.string.ev_invalid_consumption)
    "invalid-target" -> stringResource(R.string.ev_invalid_target)
    "invalid-phev-share" -> stringResource(R.string.ev_invalid_phev_share)
    "invalid-route", "invalid-battery", "invalid-input", "invalid-language" -> stringResource(R.string.ev_status_invalid)
    "plan-failed" -> stringResource(R.string.ev_status_insufficient_data)
    "cancelled" -> stringResource(R.string.ev_status_cancelled)
    else -> stringResource(R.string.ev_status_invalid)
}

@Composable
private fun issueText(code: EvIssueCode): String? = when (code) {
    EvIssueCode.ACCESS_UNKNOWN -> stringResource(R.string.ev_access_unknown)
    EvIssueCode.ACCESS_PRIVATE -> stringResource(R.string.ev_access_private)
    EvIssueCode.HOURS_UNKNOWN -> stringResource(R.string.ev_hours_unknown)
    EvIssueCode.HOURS_CLOSED -> stringResource(R.string.ev_hours_closed)
    EvIssueCode.POWER_UNKNOWN -> stringResource(R.string.ev_power_unknown)
    EvIssueCode.TARIFF_UNKNOWN -> stringResource(R.string.ev_tariff_unknown)
    EvIssueCode.REPORT_DATE_UNKNOWN -> stringResource(R.string.ev_report_unknown)
    EvIssueCode.ROAD_WARNING -> stringResource(R.string.ev_route_warning)
    EvIssueCode.SNAPSHOT_MISSING, EvIssueCode.COVERAGE_UNKNOWN -> stringResource(R.string.ev_snapshot_missing)
    EvIssueCode.MISSING_MAPS -> stringResource(R.string.ev_status_no_route)
    EvIssueCode.ALTERNATIVE_UNREACHABLE -> stringResource(R.string.ev_alt_unreachable)
    EvIssueCode.ALTERNATIVE_SAME_SITE -> stringResource(R.string.ev_alt_same_site)
    EvIssueCode.NO_EVIDENCED_ALTERNATIVE -> stringResource(R.string.ev_no_alternative)
    EvIssueCode.INCOMPATIBLE_CONNECTOR -> stringResource(R.string.ev_incompatible)
    EvIssueCode.STOP_UNREACHABLE, EvIssueCode.DESTINATION_UNREACHABLE ->
        stringResource(R.string.ev_status_insufficient_energy)
    EvIssueCode.ROUTE_BUDGET_EXHAUSTED, EvIssueCode.INCOMPLETE ->
        stringResource(R.string.ev_status_budget)
    else -> null
}

@Composable
private fun socEstimateText(value: EnergyEstimate<Long>, locale: Locale): String = when (value) {
    is EnergyEstimate.Known -> formatPercent(value.value, locale)
    is EnergyEstimate.Unknown -> stringResource(R.string.ev_unknown)
}

@Composable
private fun durationEstimateText(value: EnergyEstimate<Long>, locale: Locale): String = when (value) {
    is EnergyEstimate.Known -> stringResource(R.string.ev_minutes, formatDuration(value.value, locale))
    is EnergyEstimate.Unknown -> stringResource(R.string.ev_unknown)
}

private fun formatPercent(basisPoints: Long, locale: Locale): String =
    formatDecimal(BigDecimal.valueOf(basisPoints).movePointLeft(2), locale, 2)

private fun formatDuration(seconds: Long?, locale: Locale): String {
    if (seconds == null || seconds <= 0) return NumberFormat.getIntegerInstance(locale).format(0)
    val minutes = (seconds + 59) / 60
    return NumberFormat.getIntegerInstance(locale).format(minutes)
}

private fun formatDecimal(value: BigDecimal, locale: Locale, fractionDigits: Int): String =
    NumberFormat.getNumberInstance(locale).apply {
        minimumFractionDigits = 0
        maximumFractionDigits = fractionDigits
    }.format(value.setScale(fractionDigits, RoundingMode.HALF_UP))

private fun moneyText(value: EnergyEstimate<Long>, locale: Locale): String = when (value) {
    is EnergyEstimate.Known -> {
        val formatter = NumberFormat.getCurrencyInstance(Locale.forLanguageTag("pt-BR")).apply {
            currency = Currency.getInstance("BRL")
            minimumFractionDigits = 2
            maximumFractionDigits = 2
        }
        formatter.format(BigDecimal.valueOf(value.value).movePointLeft(3))
    }
    is EnergyEstimate.Unknown -> "?"
}

private fun tariffText(stop: EvPlannedStop, locale: Locale): String = when (val tariff = stop.candidate.tariff) {
    EvTariff.Unknown -> "?"
    is EvTariff.Known -> {
        val formatter = NumberFormat.getCurrencyInstance(Locale.forLanguageTag("pt-BR")).apply {
            currency = Currency.getInstance("BRL")
            minimumFractionDigits = 2
            maximumFractionDigits = 2
        }
        "${formatter.format(BigDecimal.valueOf(tariff.amountMilliBrlPerKwh).movePointLeft(3))}/kWh · ${tariff.condition}"
    }
}
