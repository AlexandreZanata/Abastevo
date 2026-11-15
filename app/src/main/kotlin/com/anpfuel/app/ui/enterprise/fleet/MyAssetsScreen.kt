package com.anpfuel.app.ui.enterprise.fleet

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.domain.enterprise.fleet.AssetKind
import com.anpfuel.domain.enterprise.fleet.AssetLifecycle
import com.anpfuel.domain.enterprise.fleet.MyAsset
import com.anpfuel.domain.enterprise.fleet.ReferenceAsset
import com.anpfuel.domain.enterprise.fleet.ReferenceAssignmentKind
import com.anpfuel.domain.enterprise.fleet.ReferenceMeterKind
import com.anpfuel.domain.enterprise.fleet.UsageBasis
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/*
 * SCR-FLEET-08 "Meus ativos", SCR-FLEET-10 read-only detail and SCR-FLEET-09 picker
 * (EMP-FLEET-013). Sanitized presentation of the protected cache only; no identifier,
 * code or protocol word is shown, and nothing here claims availability the feed lacks.
 */

/** "Meus ativos" with its five states and the read-only detail of one asset. */
@Composable
internal fun MyAssetsPanel(
    state: MyAssetsViewState,
    connected: Boolean,
    onBack: () -> Unit,
    onRefresh: () -> Unit,
    onRetry: () -> Unit,
    onOpen: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val detail = state.detail
    LazyColumn(modifier.testTag("enterprise-assets-panel").padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        item {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(stringResource(if (detail == null) R.string.enterprise_assets_title else R.string.enterprise_assets_detail_title),
                    style = MaterialTheme.typography.headlineSmall, modifier = Modifier.semantics { heading() })
                TextButton(onClick = onBack, modifier = Modifier.fillMaxWidth().testTag("enterprise-assets-back")) {
                    Text(stringResource(if (detail == null) R.string.enterprise_assets_back_home else R.string.enterprise_assets_back_list))
                }
                AssetsStatus(state, connected, onRefresh, onRetry)
            }
        }
        val snapshot = state.snapshot
        if (detail != null) item { AssetDetail(detail, state.snapshot?.assets?.firstOrNull { it.asset.id == detail.id }) }
        else if (snapshot != null && state.error == null) {
            if (snapshot.forbidden) item { Forbidden() }
            else if (snapshot.assets.isEmpty()) item { Empty() }
            else items(snapshot.assets, key = { it.asset.id }) { row ->
                AssetRow(row, enabled = true, modifier = Modifier.testTag("enterprise-assets-row-${row.asset.id}")) { onOpen(row.asset.id) }
            }
        }
    }
}

@Composable
private fun AssetsStatus(state: MyAssetsViewState, connected: Boolean, onRefresh: () -> Unit, onRetry: () -> Unit) {
    val snapshot = state.snapshot
    when {
        state.error != null || state.unlockRequired || state.unrecoverable -> {
            Notice(stringResource(R.string.enterprise_assets_unavailable), Modifier.testTag("enterprise-assets-error"))
            Button(onClick = onRetry, enabled = !state.busy, modifier = Modifier.fillMaxWidth().testTag("enterprise-assets-retry")) {
                Text(stringResource(R.string.enterprise_assets_retry))
            }
            return
        }
        !state.loaded -> {
            CircularProgressIndicator(Modifier.testTag("enterprise-assets-loading"))
            Notice(stringResource(R.string.enterprise_assets_loading))
            return
        }
        snapshot == null -> Notice(stringResource(if (!connected) R.string.enterprise_assets_needs_connection else R.string.enterprise_assets_refresh_failed_empty),
            Modifier.testTag("enterprise-assets-unavailable"))
        else -> {
            Text(stringResource(R.string.enterprise_assets_confirmed_at, formatted(snapshot.confirmedAt)),
                modifier = Modifier.testTag("enterprise-assets-confirmed-at"))
            if (!snapshot.listConfirmed) Notice(stringResource(R.string.enterprise_assets_not_confirmed), Modifier.testTag("enterprise-assets-not-confirmed"))
            if (state.refreshFailed) Notice(stringResource(R.string.enterprise_assets_refresh_failed), Modifier.testTag("enterprise-assets-refresh-failed"))
            if (!connected) Notice(stringResource(R.string.enterprise_assets_offline), Modifier.testTag("enterprise-assets-offline"))
        }
    }
    if (state.busy) {
        CircularProgressIndicator()
        Notice(stringResource(R.string.enterprise_assets_loading))
    }
    if (state.detail == null) Button(onClick = onRefresh, enabled = connected && !state.busy,
        modifier = Modifier.fillMaxWidth().testTag("enterprise-assets-refresh")) { Text(stringResource(R.string.enterprise_assets_refresh)) }
}

@Composable
private fun Empty() {
    AssetCard(Modifier.testTag("enterprise-assets-empty")) {
        Text(stringResource(R.string.enterprise_assets_empty), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.enterprise_assets_empty_who))
    }
}

@Composable
private fun Forbidden() {
    AssetCard(Modifier.testTag("enterprise-assets-forbidden")) {
        Text(stringResource(R.string.enterprise_assets_forbidden_title), style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_assets_forbidden))
    }
}

/** One asset per row with a large target; every fact is text, never colour alone. */
@Composable
private fun AssetRow(row: MyAsset, enabled: Boolean, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val asset = row.asset
    val notConfirmed = stringResource(R.string.enterprise_assets_row_not_confirmed)
    val unavailable = if (row.available) null else stringResource(R.string.enterprise_assets_unavailable_state, stateText(asset.state))
    val fill = MaterialTheme.colorScheme.surfaceContainerLow
    Card(onClick = onClick, enabled = enabled,
        modifier = modifier.fillMaxWidth().heightIn(min = 72.dp).semantics(mergeDescendants = true) {
            stateDescription = listOfNotNull(unavailable, notConfirmed.takeIf { !row.confirmed }).joinToString(". ")
        },
        shape = AnpContainerDefaults.containerShape(), colors = CardDefaults.cardColors(containerColor = fill),
        border = AnpContainerDefaults.borderStroke(fill)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(asset.name, style = MaterialTheme.typography.titleMedium)
            Identifiers(asset)
            Text(stringResource(kindText(asset.kind)))
            Assignment(asset)
            unavailable?.let { Text(it) }
            if (row.assignmentEnded) asset.assignment.validTo?.let {
                Text(stringResource(R.string.enterprise_assets_assignment_ended, formatted(it)))
            }
            if (!row.confirmed) Text(notConfirmed)
        }
    }
}

@Composable
private fun Identifiers(asset: ReferenceAsset) {
    if (asset.fleetNumber.isNotEmpty()) Text(stringResource(R.string.enterprise_assets_fleet_number, asset.fleetNumber))
    if (asset.plate.isNotEmpty()) Text(stringResource(R.string.enterprise_assets_plate, asset.plate))
}

@Composable
private fun Assignment(asset: ReferenceAsset) {
    Text(stringResource(when (asset.assignment.kind) {
        ReferenceAssignmentKind.PRIMARY -> R.string.enterprise_assets_assignment_primary
        ReferenceAssignmentKind.ADDITIONAL -> R.string.enterprise_assets_assignment_additional
        ReferenceAssignmentKind.TEMPORARY -> R.string.enterprise_assets_assignment_temporary
    }))
    asset.assignment.validTo?.let { Text(stringResource(R.string.enterprise_assets_assignment_until, formatted(it))) }
}

/** SCR-FLEET-10, read-only: only what the feed carries (no reading, no tank capacity). */
@Composable
private fun AssetDetail(asset: ReferenceAsset, row: MyAsset?) {
    AssetCard(Modifier.testTag("enterprise-assets-detail")) {
        Text(asset.name, style = MaterialTheme.typography.titleLarge, modifier = Modifier.semantics { heading() })
        Identifiers(asset)
        Text(stringResource(kindText(asset.kind)))
        Text(stringResource(R.string.enterprise_assets_state, stateText(asset.state)))
        Text(stringResource(if (asset.usageBasis == UsageBasis.DISTANCE) R.string.enterprise_assets_usage_distance else R.string.enterprise_assets_usage_hours))
        Assignment(asset)
        if (row != null && row.assignmentEnded) asset.assignment.validTo?.let {
            Text(stringResource(R.string.enterprise_assets_assignment_ended, formatted(it)))
        }
        if (row != null && !row.confirmed) Text(stringResource(R.string.enterprise_assets_row_not_confirmed))
        if (asset.products.isEmpty()) Text(stringResource(R.string.enterprise_assets_products_none))
        else Text(stringResource(R.string.enterprise_assets_products, asset.products.map { productText(it) }.joinToString(", ")))
        if (asset.meters.isEmpty()) Text(stringResource(R.string.enterprise_assets_meters_none))
        for (meter in asset.meters) Text(stringResource(if (meter.kind == ReferenceMeterKind.ODOMETER) R.string.enterprise_assets_meter_odometer
            else R.string.enterprise_assets_meter_hour, formatted(meter.installedAt)))
        Text(stringResource(R.string.enterprise_assets_offline_limits), style = MaterialTheme.typography.bodySmall)
    }
}

/**
 * SCR-FLEET-09: the reusable picker. It needs no network; a row that is not available or not
 * confirmed is chosen only after the review warning (B-BR-EMP-352). [accepts] lets the calling
 * operation narrow the list to what it accepts.
 */
@Composable
fun EnterpriseAssetPicker(
    state: MyAssetsViewState,
    connected: Boolean,
    onChoose: (assetId: String, warningAccepted: Boolean) -> Unit,
    onCancel: () -> Unit,
    modifier: Modifier = Modifier,
    accepts: (ReferenceAsset) -> Boolean = { true },
) {
    var warning by remember(state.generation) { mutableStateOf<MyAsset?>(null) }
    val snapshot = state.snapshot
    LazyColumn(modifier.testTag("enterprise-asset-picker").padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        item {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(stringResource(R.string.enterprise_assets_pick_title), style = MaterialTheme.typography.headlineSmall,
                    modifier = Modifier.semantics { heading() })
                TextButton(onClick = onCancel, modifier = Modifier.fillMaxWidth().testTag("enterprise-asset-picker-cancel")) {
                    Text(stringResource(R.string.enterprise_assets_pick_cancel))
                }
                when {
                    state.error != null || state.unlockRequired || state.unrecoverable -> Notice(stringResource(R.string.enterprise_assets_unavailable))
                    !state.loaded -> { CircularProgressIndicator(); Notice(stringResource(R.string.enterprise_assets_loading)) }
                    snapshot == null -> Notice(stringResource(if (!connected) R.string.enterprise_assets_needs_connection
                        else R.string.enterprise_assets_refresh_failed_empty))
                    else -> {
                        Text(stringResource(R.string.enterprise_assets_confirmed_at, formatted(snapshot.confirmedAt)))
                        if (!snapshot.listConfirmed) Notice(stringResource(R.string.enterprise_assets_not_confirmed))
                        if (!connected) Notice(stringResource(R.string.enterprise_assets_offline))
                    }
                }
            }
        }
        if (snapshot != null && state.error == null) {
            val rows = snapshot.assets.filter { accepts(it.asset) }
            if (snapshot.forbidden) item { Forbidden() }
            else if (rows.isEmpty()) item { Empty() }
            else items(rows, key = { it.asset.id }) { row ->
                AssetRow(row, enabled = !state.busy, modifier = Modifier.testTag("enterprise-asset-picker-row-${row.asset.id}")) {
                    if (row.reviewWarning) warning = row else onChoose(row.asset.id, false)
                }
            }
        }
    }
    warning?.let { row -> AlertDialog(
        onDismissRequest = { warning = null },
        title = { Text(stringResource(R.string.enterprise_assets_warning_title)) },
        text = { Text(stringResource(R.string.enterprise_assets_warning_copy)) },
        confirmButton = { TextButton(onClick = { warning = null; onChoose(row.asset.id, true) },
            modifier = Modifier.testTag("enterprise-asset-picker-warning-confirm")) { Text(stringResource(R.string.enterprise_assets_warning_confirm)) } },
        dismissButton = { TextButton(onClick = { warning = null }, modifier = Modifier.testTag("enterprise-asset-picker-warning-cancel")) {
            Text(stringResource(R.string.enterprise_assets_pick_cancel)) } },
    ) }
}

/** Home entry card for "Meus ativos"; it opens the protected list, never a server read by itself. */
@Composable
internal fun MyAssetsHomeCard(onOpen: () -> Unit, modifier: Modifier = Modifier) {
    AssetCard(modifier.testTag("enterprise-assets-home")) {
        Text(stringResource(R.string.enterprise_assets_title), style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_assets_home_copy))
        Button(onClick = onOpen, modifier = Modifier.fillMaxWidth().testTag("enterprise-assets-open")) {
            Text(stringResource(R.string.enterprise_assets_open))
        }
    }
}

@Composable
private fun AssetCard(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    val fill = MaterialTheme.colorScheme.surfaceContainerLow
    Card(modifier.fillMaxWidth(), shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = fill), border = AnpContainerDefaults.borderStroke(fill)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) { content() }
    }
}

@Composable
private fun Notice(message: String, modifier: Modifier = Modifier) {
    Text(message, modifier = modifier.semantics { liveRegion = LiveRegionMode.Polite })
}

@Composable
private fun formatted(instant: Instant): String {
    val locale = LocalConfiguration.current.locales[0]
    val zone = remember { ZoneId.systemDefault() }
    val formatter = remember(locale, zone) { DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM).withLocale(locale).withZone(zone) }
    return formatter.format(instant)
}

@Composable
private fun stateText(state: AssetLifecycle): String = stringResource(when (state) {
    AssetLifecycle.DRAFT -> R.string.enterprise_assets_state_draft
    AssetLifecycle.ACTIVE -> R.string.enterprise_assets_state_active
    AssetLifecycle.INACTIVE -> R.string.enterprise_assets_state_inactive
    AssetLifecycle.DISPOSED -> R.string.enterprise_assets_state_disposed
    AssetLifecycle.ARCHIVED -> R.string.enterprise_assets_state_archived
})

private fun kindText(kind: AssetKind): Int = when (kind) {
    AssetKind.VEHICLE -> R.string.enterprise_assets_kind_vehicle
    AssetKind.MACHINE -> R.string.enterprise_assets_kind_machine
    AssetKind.IMPLEMENT -> R.string.enterprise_assets_kind_implement
}

/** fuel-products-v1 codes to the app's existing product names; an unknown code is shown as it came. */
@Composable
private fun productText(code: String): String = when (code) {
    "ETHANOL" -> stringResource(R.string.fuel_product_ethanol)
    "GASOLINE_REGULAR" -> stringResource(R.string.fuel_product_gasoline_regular)
    "GASOLINE_ADDITIVED" -> stringResource(R.string.fuel_product_gasoline_premium)
    "GASOLINE_PREMIUM_GRADE" -> stringResource(R.string.fuel_product_gasoline_premium_grade)
    "DIESEL_S10" -> stringResource(R.string.fuel_product_diesel_s10)
    "DIESEL_S500" -> stringResource(R.string.fuel_product_diesel_s500)
    "CNG" -> stringResource(R.string.fuel_product_cng)
    "ELECTRICITY" -> stringResource(R.string.enterprise_assets_product_electricity)
    "ARLA_32" -> stringResource(R.string.enterprise_assets_product_arla)
    else -> code
}
