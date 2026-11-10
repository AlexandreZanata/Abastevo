package com.anpfuel.app.ui.driver

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ReceiptLong
import androidx.compose.material.icons.filled.BarChart
import androidx.compose.material.icons.filled.DirectionsCar
import androidx.compose.material.icons.filled.EvStation
import androidx.compose.material.icons.filled.LocalGasStation
import androidx.compose.material.icons.filled.Schedule
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.CloudOff
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.components.ProfileActionRow
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.theme.AnpContainerDefaults

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DriverHubScreen(
    onNavigateBack: () -> Unit,
    onOpenVehicles: () -> Unit,
    onOpenEntries: (String) -> Unit,
    onOpenMonthly: (String) -> Unit,
    onOpenRefuel: (String) -> Unit,
    onOpenCharge: (String) -> Unit,
    onOpenWork: (String) -> Unit = {},
    onOpenPremium: () -> Unit,
    onSignIn: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: DriverHubViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    LaunchedEffect(Unit) { viewModel.refresh() }
    DriverHubContent(
        state = state, onNavigateBack = onNavigateBack, onOpenVehicles = onOpenVehicles,
        onOpenEntries = onOpenEntries, onOpenMonthly = onOpenMonthly, onOpenRefuel = onOpenRefuel, onOpenCharge = onOpenCharge, onOpenWork = onOpenWork,
        onOpenPremium = onOpenPremium, onSignIn = onSignIn, onRetry = viewModel::refresh, modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun DriverHubContent(
    state: DriverHubUiState,
    onNavigateBack: () -> Unit,
    onOpenVehicles: () -> Unit,
    onOpenEntries: (String) -> Unit,
    onOpenMonthly: (String) -> Unit,
    onOpenRefuel: (String) -> Unit,
    onOpenCharge: (String) -> Unit,
    onOpenWork: (String) -> Unit = {},
    onOpenPremium: () -> Unit,
    onSignIn: () -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                title = {
                    Text(
                        text = stringResource(R.string.driver_hub_title),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { innerPadding ->
        when {
            state.loading -> LoadingState(modifier = Modifier.padding(innerPadding))
            state.vehicleLoadFailed && state.vehicles.isEmpty() -> ErrorState(
                message = stringResource(R.string.driver_hub_load_error),
                modifier = Modifier.padding(innerPadding),
                onRetry = onRetry,
            )
            else -> Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(innerPadding)
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 12.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                AccessCard(state = state, onRetry = onRetry, onSignIn = onSignIn, onOpenPremium = onOpenPremium)
                val vehicle = state.vehicles.firstOrNull()
                if (vehicle == null) {
                    Card(
                        modifier = Modifier.fillMaxWidth(),
                        shape = AnpContainerDefaults.containerShape(),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                        border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surface),
                    ) {
                        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(stringResource(R.string.driver_hub_first_vehicle_title), style = MaterialTheme.typography.titleMedium)
                            Text(stringResource(R.string.driver_hub_first_vehicle_body), style = MaterialTheme.typography.bodyMedium)
                            Button(onClick = onOpenVehicles, modifier = Modifier.fillMaxWidth()) {
                                Text(stringResource(R.string.driver_hub_add_vehicle))
                            }
                        }
                    }
                } else {
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(
                            stringResource(R.string.driver_hub_current_vehicle),
                            style = MaterialTheme.typography.labelLarge,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        Text(vehicle.displayName, style = MaterialTheme.typography.headlineSmall)
                    }

                    DriverActionGroup(R.string.driver_hub_register_group) {
                        ProfileActionRow(
                            icon = Icons.Default.LocalGasStation,
                            title = stringResource(R.string.driver_hub_quick_refuel),
                            subtitle = stringResource(R.string.driver_hub_quick_refuel_body),
                            onClick = { onOpenRefuel(vehicle.id.value) },
                            modifier = driverActionTarget,
                            iconTint = MaterialTheme.colorScheme.primary,
                        )
                        HorizontalDivider(Modifier.padding(horizontal = 16.dp))
                        ProfileActionRow(
                            icon = Icons.Default.EvStation,
                            title = stringResource(R.string.driver_hub_quick_charge),
                            subtitle = stringResource(R.string.driver_hub_quick_charge_body),
                            onClick = { onOpenCharge(vehicle.id.value) },
                            modifier = driverActionTarget,
                            iconTint = MaterialTheme.colorScheme.primary,
                        )
                    }

                    DriverActionGroup(R.string.driver_hub_work_group) {
                        ProfileActionRow(
                            icon = Icons.Default.Schedule,
                            title = stringResource(R.string.driver_hub_open_work),
                            subtitle = stringResource(R.string.driver_hub_open_work_body),
                            onClick = { onOpenWork(vehicle.id.value) },
                            modifier = driverActionTarget,
                        )
                        HorizontalDivider(Modifier.padding(horizontal = 16.dp))
                        ProfileActionRow(
                            icon = Icons.AutoMirrored.Filled.ReceiptLong,
                            title = stringResource(R.string.driver_hub_open_entries),
                            subtitle = stringResource(R.string.driver_hub_open_entries_body),
                            onClick = { onOpenEntries(vehicle.id.value) },
                            modifier = driverActionTarget,
                        )
                        HorizontalDivider(Modifier.padding(horizontal = 16.dp))
                        ProfileActionRow(
                            icon = Icons.Default.BarChart,
                            title = stringResource(R.string.driver_hub_open_export),
                            subtitle = stringResource(R.string.driver_hub_open_export_body),
                            onClick = { onOpenMonthly(vehicle.id.value) },
                            modifier = driverActionTarget,
                        )
                    }

                    DriverActionGroup(R.string.driver_hub_vehicle_group) {
                        ProfileActionRow(
                            icon = Icons.Default.DirectionsCar,
                            title = stringResource(R.string.driver_hub_open_vehicles),
                            subtitle = stringResource(R.string.driver_hub_open_vehicles_body),
                            onClick = onOpenVehicles,
                            modifier = driverActionTarget,
                        )
                    }
                }
                Text(
                    text = stringResource(R.string.driver_hub_retained_note),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    text = stringResource(R.string.driver_hub_no_reputation),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

private val driverActionTarget = Modifier.heightIn(min = 64.dp).semantics { role = Role.Button }

@Composable
private fun DriverActionGroup(
    @androidx.annotation.StringRes title: Int,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            stringResource(title),
            modifier = Modifier.semantics { heading() },
            style = MaterialTheme.typography.titleMedium,
        )
        val surface = MaterialTheme.colorScheme.surfaceContainerLow
        Card(
            modifier = Modifier.fillMaxWidth(),
            shape = AnpContainerDefaults.containerShape(),
            colors = CardDefaults.cardColors(containerColor = surface),
            border = AnpContainerDefaults.borderStroke(surface),
            content = content,
        )
    }
}

@Composable
private fun AccessCard(
    state: DriverHubUiState,
    onRetry: () -> Unit,
    onSignIn: () -> Unit,
    onOpenPremium: () -> Unit,
) {
    val (title, body, icon) = when (state.access) {
        DriverHubAccess.VERIFIED -> Triple(R.string.driver_hub_status_verified_title, R.string.driver_hub_status_verified_body, Icons.Default.CheckCircle)
        DriverHubAccess.OFFLINE -> Triple(R.string.driver_hub_status_offline_title, R.string.driver_hub_status_offline_body, Icons.Default.CloudOff)
        DriverHubAccess.LAPSED -> Triple(R.string.driver_hub_status_lapsed_title, R.string.driver_hub_status_lapsed_body, Icons.Default.Lock)
        DriverHubAccess.SIGNED_OUT -> Triple(R.string.driver_hub_status_signed_out_title, R.string.driver_hub_status_signed_out_body, Icons.Default.Lock)
        DriverHubAccess.UNAVAILABLE -> Triple(R.string.driver_hub_status_unavailable_title, R.string.driver_hub_status_unavailable_body, Icons.Default.ErrorOutline)
    }
    val surface = if (state.access == DriverHubAccess.VERIFIED) {
        MaterialTheme.colorScheme.surfaceContainerLow
    } else {
        MaterialTheme.colorScheme.secondaryContainer
    }
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = surface),
        border = AnpContainerDefaults.borderStroke(surface),
    ) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Icon(icon, contentDescription = null)
                Text(stringResource(title), style = MaterialTheme.typography.titleMedium)
            }
            Text(stringResource(body), style = MaterialTheme.typography.bodyMedium)
            when (state.access) {
                DriverHubAccess.SIGNED_OUT -> Button(onClick = onSignIn, modifier = Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.driver_hub_sign_in))
                }
                DriverHubAccess.OFFLINE, DriverHubAccess.UNAVAILABLE -> Button(onClick = onRetry, modifier = Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.driver_hub_retry))
                }
                DriverHubAccess.LAPSED -> Button(onClick = onOpenPremium, modifier = Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.driver_hub_open_premium))
                }
                DriverHubAccess.VERIFIED -> Unit
            }
        }
    }
}
