package com.anpfuel.app.ui.stationprofile

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.profile.ProfileBusinessFieldsRule
import com.anpfuel.domain.valueobject.FuelProduct

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StationManagementScreen(onBack: () -> Unit, onSignIn: () -> Unit, viewModel: StationManagementViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    DisposableEffect(lifecycle, viewModel) {
        val observer = LifecycleEventObserver { _, event -> if (event == Lifecycle.Event.ON_RESUME) viewModel.refresh() }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer) }
    }
    LaunchedEffect(viewModel) { viewModel.refresh() }
    Scaffold(topBar = { AnpTopAppBar(title = { Text(stringResource(R.string.station_management_title)) }, onNavigateUp = onBack) }) { padding ->
        StationManagementContent(state, viewModel::field, viewModel::save, viewModel::reply, viewModel::fuel, viewModel::sendReply, viewModel::refresh, onSignIn, Modifier.padding(padding))
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun StationManagementContent(
    state: StationManagementState,
    onField: (String, String) -> Unit,
    onSave: () -> Unit,
    onReply: (String) -> Unit,
    onFuel: (String) -> Unit,
    onSendReply: () -> Unit,
    onRefresh: () -> Unit,
    onSignIn: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(stringResource(R.string.station_management_server))
        Text(stringResource(R.string.station_management_canonical))
        if (state.busy) CircularProgressIndicator()
        if (state.notice != ManagementNotice.None) Text(stringResource(when (state.notice) {
            ManagementNotice.SignIn -> R.string.station_claim_sign_in
            ManagementNotice.Invalid -> R.string.station_management_invalid
            ManagementNotice.Refused -> R.string.station_claim_refused
            ManagementNotice.ReloadRequired -> R.string.station_management_changed
            ManagementNotice.Saved -> R.string.station_management_saved
            ManagementNotice.ReplySent -> R.string.station_management_reply_sent
            else -> R.string.station_management_unavailable
        }))
        if (state.notice == ManagementNotice.SignIn) Button(onClick = onSignIn) { Text(stringResource(R.string.station_claim_login)) }
        OutlinedButton(onClick = onRefresh, enabled = !state.busy) { Text(stringResource(R.string.station_claim_refresh)) }
        if (state.scopes.isEmpty() && !state.busy) Text(stringResource(R.string.station_management_no_scope))
        if ("profile.edit" in state.scopes) {
            for ((key, label) in listOf(
                "opening_hours" to R.string.station_profile_hours, "phone" to R.string.station_profile_phone,
                "website" to R.string.station_profile_website, "description" to R.string.station_profile_description,
            )) {
                OutlinedTextField(value = state.fields[key].orEmpty(), onValueChange = { onField(key, it) },
                    label = { Text(stringResource(label)) }, enabled = state.canEdit, modifier = Modifier.fillMaxWidth())
            }
            Text(stringResource(R.string.station_profile_services), modifier = Modifier.semantics { heading() })
            val chosen = state.fields["services"].orEmpty().split(',').map { it.trim() }.filter { it.isNotBlank() }.toSet()
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ProfileBusinessFieldsRule.services.forEach { service ->
                    FilterChip(selected = service in chosen, enabled = state.canEdit,
                        onClick = { onField("services", (if (service in chosen) chosen - service else chosen + service).sorted().joinToString(",")) },
                        label = { Text(stringResource(serviceLabel(service))) })
                }
            }
            Button(onClick = onSave, enabled = state.canEdit && ProfileBusinessFieldsRule.valid(state.changed), modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.station_management_save)) }
        }
        if ("reply.official" in state.scopes) {
            Text(stringResource(R.string.station_management_reply), modifier = Modifier.semantics { heading() })
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FuelProduct.entries.forEach { fuel ->
                    val wire = WireFuelMapper.toWire(fuel)
                    FilterChip(selected = state.fuelWire == wire, enabled = state.canReply, onClick = { onFuel(wire) }, label = { Text(stringResource(FuelProductI18n.toStringRes(fuel))) })
                }
            }
            OutlinedTextField(value = state.reply, onValueChange = onReply, label = { Text(stringResource(R.string.station_management_reply_text)) }, enabled = state.canReply, modifier = Modifier.fillMaxWidth())
            Text(stringResource(R.string.station_management_reply_count, state.reply.codePointCount(0, state.reply.length)))
            Button(onClick = onSendReply, enabled = state.canReply && state.reply.isNotBlank() && state.reply.codePointCount(0, state.reply.length) <= 280, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.station_management_publish)) }
        }
        Text(stringResource(R.string.station_management_other), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.station_management_other_unavailable))
    }
}

internal fun serviceLabel(service: String): Int = when (service) {
    "fuel" -> R.string.station_service_fuel
    "convenience" -> R.string.station_service_convenience
    "carwash" -> R.string.station_service_carwash
    "tire-service" -> R.string.station_service_tires
    "oil-change" -> R.string.station_service_oil
    "restaurant" -> R.string.station_service_restaurant
    "atm" -> R.string.station_service_atm
    "restroom" -> R.string.station_service_restroom
    "wifi" -> R.string.station_service_wifi
    else -> R.string.station_service_parking
}
