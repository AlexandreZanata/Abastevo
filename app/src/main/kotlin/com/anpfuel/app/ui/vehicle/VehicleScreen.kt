package com.anpfuel.app.ui.vehicle

import android.Manifest
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.AppErrorMapper
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.ui.components.AnpAttributionFooter
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import com.anpfuel.domain.valueobject.VehiclePriceSourceMode

@Composable
fun VehicleScreen(
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: VehicleViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]

    LaunchedEffect(Unit) {
        viewModel.load()
    }

    val notificationPermissionLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestPermission(),
    ) { granted ->
        if (granted) {
            viewModel.onNotificationPermissionGranted()
        } else {
            viewModel.onNotificationPermissionDenied()
        }
    }

    LaunchedEffect(viewModel) {
        viewModel.notificationPermissionRequest.collect {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                notificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        }
    }

    LaunchedEffect(Unit) {
        viewModel.refreshNotificationPermissionState()
    }

    VehicleContent(
        uiState = uiState,
        locale = locale,
        onNavigateBack = onNavigateBack,
        onAddVehicle = viewModel::startAddVehicle,
        onEditVehicle = { id -> viewModel.startEditVehicle(id, locale) },
        onRequestDeleteVehicle = viewModel::requestDeleteVehicle,
        onConfirmDeleteVehicle = viewModel::confirmDeleteVehicle,
        onDismissDeleteConfirm = viewModel::dismissDeleteConfirm,
        onDismissForm = viewModel::dismissForm,
        onDisplayNameChanged = viewModel::onDisplayNameChanged,
        onTankCapacityChanged = viewModel::onTankCapacityChanged,
        onFuelProductSelected = { product -> viewModel.onFuelProductSelected(product, locale) },
        onPriceSourceModeSelected = { mode -> viewModel.onPriceSourceModeSelected(mode, locale) },
        onStationSelected = viewModel::onStationSelected,
        onPriceDropAlertChanged = viewModel::onPriceDropAlertChanged,
        onSaveVehicle = { viewModel.saveVehicle(locale) },
        onRetry = viewModel::load,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun VehicleContent(
    uiState: VehicleUiState,
    locale: java.util.Locale,
    onNavigateBack: () -> Unit,
    onAddVehicle: () -> Unit,
    onEditVehicle: (DomainId) -> Unit,
    onRequestDeleteVehicle: (DomainId) -> Unit,
    onConfirmDeleteVehicle: () -> Unit,
    onDismissDeleteConfirm: () -> Unit,
    onDismissForm: () -> Unit,
    onDisplayNameChanged: (String) -> Unit,
    onTankCapacityChanged: (String) -> Unit,
    onFuelProductSelected: (FuelProduct) -> Unit,
    onPriceSourceModeSelected: (VehiclePriceSourceMode) -> Unit,
    onStationSelected: (com.anpfuel.domain.valueobject.Cnpj) -> Unit,
    onPriceDropAlertChanged: (Boolean) -> Unit,
    onSaveVehicle: () -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    if (uiState.showDeleteConfirm) {
        AlertDialog(
            onDismissRequest = onDismissDeleteConfirm,
            title = { Text(text = stringResource(R.string.vehicle_delete)) },
            text = { Text(text = stringResource(R.string.vehicle_delete_confirm)) },
            confirmButton = {
                TextButton(onClick = onConfirmDeleteVehicle) {
                    Text(text = stringResource(R.string.vehicle_delete))
                }
            },
            dismissButton = {
                TextButton(onClick = onDismissDeleteConfirm) {
                    Text(text = stringResource(R.string.action_cancel))
                }
            },
        )
    }

    val titleRes = if (uiState.showForm && uiState.editingVehicleId != null) {
        R.string.vehicle_edit
    } else if (uiState.showForm) {
        R.string.vehicle_add
    } else {
        R.string.vehicle_screen_title
    }

    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                title = {
                    Text(
                        text = stringResource(titleRes),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                },
                navigationIcon = {
                    TextButton(
                        onClick = if (uiState.showForm) onDismissForm else onNavigateBack,
                    ) {
                        Text(text = stringResource(R.string.action_back))
                    }
                },
            )
        },
        floatingActionButton = {
            if (!uiState.showForm && uiState.canAddVehicle && !uiState.isLoading) {
                FloatingActionButton(onClick = onAddVehicle) {
                    Icon(
                        imageVector = Icons.Default.Add,
                        contentDescription = stringResource(R.string.vehicle_add),
                    )
                }
            }
        },
        bottomBar = { AnpAttributionFooter() },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .padding(horizontal = 16.dp, vertical = 8.dp),
        ) {
            when {
                uiState.isLoading -> LoadingState(modifier = Modifier.fillMaxWidth())

                uiState.error != null && !uiState.showForm -> {
                    ErrorState(
                        message = stringResource(AppErrorMapper.toStringRes(uiState.error)),
                        modifier = Modifier.fillMaxWidth(),
                        onRetry = onRetry,
                    )
                }

                uiState.showForm -> {
                    VehicleForm(
                        form = uiState.form,
                        stationOptions = uiState.stationOptions,
                        isLoadingStations = uiState.isLoadingStations,
                        isSaving = uiState.isSaving,
                        showNotificationPermissionHint = uiState.showNotificationPermissionHint,
                        locale = locale,
                        onDisplayNameChanged = onDisplayNameChanged,
                        onTankCapacityChanged = onTankCapacityChanged,
                        onFuelProductSelected = onFuelProductSelected,
                        onPriceSourceModeSelected = onPriceSourceModeSelected,
                        onStationSelected = onStationSelected,
                        onPriceDropAlertChanged = onPriceDropAlertChanged,
                        onSave = onSaveVehicle,
                        onCancel = onDismissForm,
                    )
                }

                uiState.vehicles.isEmpty() -> {
                    EmptyState(
                        message = stringResource(R.string.vehicle_empty_message),
                        hint = stringResource(R.string.vehicle_empty_hint),
                        modifier = Modifier.fillMaxWidth(),
                    )
                }

                else -> {
                    Column(
                        modifier = Modifier
                            .fillMaxSize()
                            .verticalScroll(rememberScrollState()),
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        uiState.vehicles.forEach { vehicle ->
                            VehicleListItem(
                                vehicle = vehicle,
                                onClick = { onEditVehicle(vehicle.id) },
                                onDelete = { onRequestDeleteVehicle(vehicle.id) },
                            )
                        }
                        if (!uiState.canAddVehicle) {
                            Text(
                                text = stringResource(R.string.vehicle_max_reached),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun VehicleListItem(
    vehicle: Vehicle,
    onClick: () -> Unit,
    onDelete: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val fuelLabel = stringResource(FuelProductI18n.toStringRes(vehicle.fuelProduct))
    val priceSourceLabel = when (vehicle.priceSource.mode) {
        VehiclePriceSourceMode.CHEAPEST_STATION ->
            stringResource(R.string.vehicle_price_source_cheapest)
        VehiclePriceSourceMode.SPECIFIC_STATION ->
            stringResource(R.string.vehicle_price_source_specific)
    }
    val liters = vehicle.tankCapacity.liters.toInt()

    Card(
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        ),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Text(
                    text = vehicle.displayName,
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.onSurface,
                    modifier = Modifier.weight(1f),
                )
                IconButton(onClick = onDelete) {
                    Icon(
                        imageVector = Icons.Default.Delete,
                        contentDescription = stringResource(R.string.vehicle_delete),
                    )
                }
            }
            Text(
                text = stringResource(
                    R.string.home_tank_fill_cost_liters_format,
                    fuelLabel,
                    liters,
                ),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = priceSourceLabel,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Preview(showBackground = true, name = "Vehicle list")
@Composable
private fun VehicleListPreview() {
    AnpFuelTheme {
        VehicleContent(
            uiState = VehicleUiState(
                isLoading = false,
                vehicles = listOf(
                    Vehicle.create(
                        displayName = "Gol",
                        tankCapacity = TankCapacity.of(50.0),
                        fuelProduct = FuelProduct.GASOLINE_REGULAR,
                        priceSource = VehiclePriceSource.cheapest(),
                    ),
                ),
            ),
            locale = java.util.Locale.US,
            onNavigateBack = {},
            onAddVehicle = {},
            onEditVehicle = {},
            onRequestDeleteVehicle = {},
            onConfirmDeleteVehicle = {},
            onDismissDeleteConfirm = {},
            onDismissForm = {},
            onDisplayNameChanged = {},
            onTankCapacityChanged = {},
            onFuelProductSelected = {},
            onPriceSourceModeSelected = {},
            onStationSelected = {},
            onPriceDropAlertChanged = {},
            onSaveVehicle = {},
            onRetry = {},
        )
    }
}

@Preview(showBackground = true, name = "Vehicle empty")
@Composable
private fun VehicleEmptyPreview() {
    AnpFuelTheme {
        VehicleContent(
            uiState = VehicleUiState(isLoading = false),
            locale = java.util.Locale.US,
            onNavigateBack = {},
            onAddVehicle = {},
            onEditVehicle = {},
            onRequestDeleteVehicle = {},
            onConfirmDeleteVehicle = {},
            onDismissDeleteConfirm = {},
            onDismissForm = {},
            onDisplayNameChanged = {},
            onTankCapacityChanged = {},
            onFuelProductSelected = {},
            onPriceSourceModeSelected = {},
            onStationSelected = {},
            onPriceDropAlertChanged = {},
            onSaveVehicle = {},
            onRetry = {},
        )
    }
}
