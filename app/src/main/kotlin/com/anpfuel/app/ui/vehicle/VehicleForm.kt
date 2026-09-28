package com.anpfuel.app.ui.vehicle

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.VehiclePriceSourceMode
import java.util.Locale

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun VehicleForm(
    form: VehicleFormState,
    stationOptions: List<VehicleStationOptionUiModel>,
    isLoadingStations: Boolean,
    isSaving: Boolean,
    showNotificationPermissionHint: Boolean = false,
    locale: Locale,
    onDisplayNameChanged: (String) -> Unit,
    onTankCapacityChanged: (String) -> Unit,
    onFuelProductSelected: (FuelProduct) -> Unit,
    onPriceSourceModeSelected: (VehiclePriceSourceMode) -> Unit,
    onStationSelected: (Cnpj) -> Unit,
    onPriceDropAlertChanged: (Boolean) -> Unit,
    onSave: () -> Unit,
    onCancel: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        OutlinedTextField(
            value = form.displayName,
            onValueChange = onDisplayNameChanged,
            modifier = Modifier.fillMaxWidth(),
            label = { Text(text = stringResource(R.string.vehicle_name_hint)) },
            singleLine = true,
            isError = form.validationErrorKey == VehicleFormValidationError.NAME_REQUIRED,
            supportingText = if (form.validationErrorKey == VehicleFormValidationError.NAME_REQUIRED) {
                { Text(text = stringResource(R.string.vehicle_validation_name_required)) }
            } else {
                null
            },
        )

        OutlinedTextField(
            value = form.tankCapacityInput,
            onValueChange = onTankCapacityChanged,
            modifier = Modifier.fillMaxWidth(),
            label = { Text(text = stringResource(R.string.vehicle_tank_capacity_hint)) },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
            isError = form.validationErrorKey == VehicleFormValidationError.TANK_CAPACITY_INVALID,
            supportingText = if (form.validationErrorKey == VehicleFormValidationError.TANK_CAPACITY_INVALID) {
                { Text(text = stringResource(R.string.vehicle_validation_tank_capacity)) }
            } else {
                null
            },
        )

        Text(
            text = stringResource(R.string.vehicle_fuel_product_label),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurface,
        )
        FlowRow(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            FuelProduct.entries.forEach { product ->
                FilterChip(
                    selected = form.fuelProduct == product,
                    onClick = { onFuelProductSelected(product) },
                    label = { Text(text = stringResource(FuelProductI18n.toStringRes(product))) },
                )
            }
        }

        Text(
            text = stringResource(R.string.vehicle_price_source_label),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurface,
        )
        FlowRow(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            FilterChip(
                selected = form.priceSourceMode == VehiclePriceSourceMode.CHEAPEST_STATION,
                onClick = { onPriceSourceModeSelected(VehiclePriceSourceMode.CHEAPEST_STATION) },
                label = { Text(text = stringResource(R.string.vehicle_price_source_cheapest)) },
            )
            FilterChip(
                selected = form.priceSourceMode == VehiclePriceSourceMode.SPECIFIC_STATION,
                onClick = { onPriceSourceModeSelected(VehiclePriceSourceMode.SPECIFIC_STATION) },
                label = { Text(text = stringResource(R.string.vehicle_price_source_specific)) },
            )
        }

        if (form.priceSourceMode == VehiclePriceSourceMode.SPECIFIC_STATION) {
            Text(
                text = stringResource(R.string.vehicle_select_station),
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onSurface,
            )
            when {
                isLoadingStations -> {
                    Text(
                        text = stringResource(R.string.state_loading),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                stationOptions.isEmpty() -> {
                    Text(
                        text = stringResource(R.string.stations_empty),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                else -> {
                    Column(
                        modifier = Modifier.fillMaxWidth(),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        stationOptions.forEach { station ->
                            VehicleStationOptionRow(
                                label = stringResource(
                                    R.string.vehicle_station_option_format,
                                    station.displayName,
                                    station.priceFormatted,
                                ),
                                selected = form.selectedStationCnpj == station.cnpj,
                                onClick = { onStationSelected(station.cnpj) },
                            )
                        }
                    }
                }
            }
            if (form.validationErrorKey == VehicleFormValidationError.STATION_REQUIRED) {
                Text(
                    text = stringResource(R.string.vehicle_validation_station_required),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
            }
        }

        Column(
            modifier = Modifier.fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text(
                text = stringResource(R.string.vehicle_price_drop_alert_label),
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onSurface,
            )
            Text(
                text = stringResource(R.string.vehicle_price_drop_alert_description),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Switch(
                checked = form.priceDropAlertEnabled,
                onCheckedChange = onPriceDropAlertChanged,
                modifier = Modifier.align(Alignment.End),
            )
            if (showNotificationPermissionHint) {
                Text(
                    text = stringResource(R.string.notification_permission_denied_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
            }
        }

        Button(
            onClick = onSave,
            enabled = !isSaving,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(text = stringResource(R.string.vehicle_save))
        }

        Button(
            onClick = onCancel,
            enabled = !isSaving,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(text = stringResource(R.string.action_cancel))
        }
    }
}

@Composable
private fun VehicleStationOptionRow(
    label: String,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Surface(
        onClick = onClick,
        modifier = modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.medium,
        color = if (selected) {
            MaterialTheme.colorScheme.primaryContainer
        } else {
            MaterialTheme.colorScheme.surface
        },
        border = BorderStroke(
            width = if (selected) 2.dp else 1.dp,
            color = if (selected) {
                MaterialTheme.colorScheme.primary
            } else {
                MaterialTheme.colorScheme.outlineVariant
            },
        ),
    ) {
        Text(
            text = label,
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 12.dp),
            style = MaterialTheme.typography.bodyMedium,
            color = if (selected) {
                MaterialTheme.colorScheme.onPrimaryContainer
            } else {
                MaterialTheme.colorScheme.onSurface
            },
            textAlign = TextAlign.Start,
        )
    }
}
