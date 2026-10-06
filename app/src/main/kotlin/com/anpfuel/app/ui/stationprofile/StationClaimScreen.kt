package com.anpfuel.app.ui.stationprofile

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.application.usecase.profile.OwnedProfileClaim

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StationClaimScreen(
    onBack: () -> Unit,
    onSignIn: () -> Unit,
    onManage: (String) -> Unit,
    viewModel: StationClaimViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val resolver = LocalContext.current.contentResolver
    var exportTarget by remember { mutableStateOf<Pair<String, String>?>(null) }
    var importTarget by remember { mutableStateOf<Pair<String, String>?>(null) }
    val export = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("text/plain")) { uri ->
        val target = exportTarget
        exportTarget = null
        if (uri != null && target != null) viewModel.export(resolver, uri, target.first, target.second)
    }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        val target = importTarget
        importTarget = null
        if (uri != null && target != null) viewModel.import(resolver, uri, target.first, target.second)
    }
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    DisposableEffect(lifecycle, viewModel) {
        val observer = LifecycleEventObserver { _, event -> if (event == Lifecycle.Event.ON_RESUME) viewModel.refresh() }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer) }
    }
    LaunchedEffect(viewModel) { viewModel.refresh() }
    Scaffold(topBar = { AnpTopAppBar(title = { Text(stringResource(R.string.station_claim_title)) }, onNavigateUp = onBack) }) { padding ->
        StationClaimContent(
            state, viewModel::role, viewModel::open, viewModel::refresh, viewModel::select,
            onExport = { exportTarget = it.id to it.declarationId; export.launch("declaration.txt") },
            onPick = { importTarget = it.id to it.declarationId; picker.launch(arrayOf("application/pdf")) },
            onSubmit = viewModel::submitPrepared, onReissue = viewModel::reissue, onCancel = viewModel::cancel,
            onSignIn = onSignIn, onManage = { onManage(viewModel.stationId) }, modifier = Modifier.padding(padding),
        )
    }
}

@Composable
internal fun StationClaimContent(
    state: StationClaimState,
    onRole: (String) -> Unit,
    onOpen: () -> Unit,
    onRefresh: () -> Unit,
    onSelect: (OwnedProfileClaim) -> Unit,
    onExport: (OwnedProfileClaim) -> Unit,
    onPick: (OwnedProfileClaim) -> Unit,
    onSubmit: () -> Unit,
    onReissue: () -> Unit,
    onCancel: () -> Unit,
    onSignIn: () -> Unit,
    onManage: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var cancelConfirmed by remember { mutableStateOf(false) }
    Column(modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(stringResource(R.string.station_claim_explanation))
        Text(stringResource(R.string.station_claim_privacy))
        if (state.busy) CircularProgressIndicator()
        if (state.notice != ClaimNotice.None) Text(stringResource(noticeLabel(state.notice)), modifier = Modifier.semantics { liveRegion = androidx.compose.ui.semantics.LiveRegionMode.Polite })
        if (state.notice == ClaimNotice.SignIn) {
            Button(onClick = onSignIn) { Text(stringResource(R.string.station_claim_login)) }
        } else {
            Text(stringResource(R.string.station_claim_role), modifier = Modifier.semantics { heading() })
            for (role in listOf("manager", "administrator")) {
                FilterChip(selected = state.role == role, onClick = { onRole(role) }, enabled = !state.busy,
                    label = { Text(stringResource(if (role == "manager") R.string.station_claim_manager else R.string.station_claim_administrator)) })
            }
            Text(stringResource(R.string.station_claim_scopes))
            Button(onClick = onOpen, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.station_claim_open)) }
        }
        OutlinedButton(onClick = onRefresh, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.station_claim_refresh)) }
        state.claims.forEach { claim ->
            OutlinedButton(onClick = { onSelect(claim); cancelConfirmed = false }, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(claimStateLabel(claim.state)))
            }
        }
        state.selected?.let { claim ->
            Text(stringResource(claimStateLabel(claim.state)), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
            if (claim.canSupplyProof) {
                Text(stringResource(R.string.station_claim_export_help))
                SelectionContainer { Text(claim.declaration, style = MaterialTheme.typography.bodySmall) }
                OutlinedButton(onClick = { onExport(claim) }, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.station_claim_export)) }
                OutlinedButton(onClick = { onPick(claim) }, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.station_claim_pick)) }
                if (state.fileBytes > 0) {
                    Text(stringResource(R.string.station_claim_ready_bytes, state.fileBytes))
                    Button(onClick = onSubmit, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.station_claim_send)) }
                }
                OutlinedButton(onClick = onReissue, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.station_claim_reissue)) }
            }
            if (claim.state == "approved") OutlinedButton(onClick = onManage, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.station_profile_claim)) }
            if (claim.state in setOf("draft", "awaiting_proof", "checking", "needs_information", "in_review")) {
                OutlinedButton(onClick = { cancelConfirmed = true }, enabled = !state.busy) { Text(stringResource(R.string.station_claim_cancel)) }
                if (cancelConfirmed) {
                    Text(stringResource(R.string.station_claim_cancel_question))
                    Button(onClick = { cancelConfirmed = false; onCancel() }, enabled = !state.busy) { Text(stringResource(R.string.station_claim_cancel_confirm)) }
                    TextButton(onClick = { cancelConfirmed = false }) { Text(stringResource(R.string.action_back)) }
                }
            }
        }
    }
}

internal fun claimStateLabel(state: String): Int = when (state) {
    "draft", "awaiting_proof" -> R.string.station_claim_draft
    "checking", "in_review" -> R.string.station_claim_review
    "needs_information" -> R.string.station_claim_needs_info
    "approved" -> R.string.station_claim_approved
    "denied" -> R.string.station_claim_denied
    "cancelled" -> R.string.station_claim_cancelled
    "expired" -> R.string.station_claim_expired
    else -> R.string.station_claim_unknown
}

internal fun noticeLabel(notice: ClaimNotice): Int = when (notice) {
    ClaimNotice.SignIn -> R.string.station_claim_sign_in
    ClaimNotice.Invalid, ClaimNotice.Changed -> R.string.station_claim_invalid
    ClaimNotice.FileUnavailable -> R.string.station_claim_file_unavailable
    ClaimNotice.FileSaved -> R.string.station_claim_file_saved
    ClaimNotice.FileReady -> R.string.station_claim_file_ready
    ClaimNotice.ProofReceived -> R.string.station_claim_received
    ClaimNotice.Refused -> R.string.station_claim_refused
    ClaimNotice.Cancelled -> R.string.station_claim_cancelled
    else -> R.string.station_profile_unavailable
}
