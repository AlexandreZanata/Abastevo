package com.anpfuel.app.ui.enterprise

import android.app.Activity
import android.app.KeyguardManager
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.anpfuel.app.R
import com.anpfuel.app.ui.auth.SecureAccountWindow
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseContextState
import com.anpfuel.application.enterprise.EnterpriseContextSummary

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EnterpriseContextScreen(
    onPersonal: () -> Unit,
    onSignIn: () -> Unit,
    onInvitation: () -> Unit,
    viewModel: EnterpriseContextViewModel = hiltViewModel(),
    syncViewModel: EnterpriseSyncViewModel = hiltViewModel(),
) {
    // These are cheap local invalidation flows; keep private presentation current
    // while stopped rather than briefly resuming an earlier confirmed context.
    val state by viewModel.state.collectAsState()
    val connected by viewModel.connected.collectAsState()
    val syncState by syncViewModel.state.collectAsState()
    val renderedGeneration = syncState.generation
    val renderedSelected = state.selected
    var showSync by remember { mutableStateOf(false) }
    var openedFor by remember { mutableStateOf<EnterpriseContextSummary?>(null) }
    var resumed by remember { mutableStateOf(false) }
    // Old UI callbacks retain their rendered primitive, never the newer delegate.
    val renderedEpoch = viewModel.epochs.collectAsState().value
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    var choosing by remember { mutableStateOf(false) }
    var recovery by remember { mutableStateOf<Pair<EnterpriseContextSummary, Long>?>(null) }
    var protectionNotice by remember { mutableStateOf<Int?>(null) }
    val androidContext = LocalContext.current
    val unlock = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        // Unlock never resumes an old request or grants authority. Selecting
        // again starts a fresh owner/proof ceremony after lifecycle invalidation.
        viewModel.unconfirm()
        if (result.resultCode == Activity.RESULT_OK) protectionNotice = R.string.enterprise_protection_unlock_done
    }
    fun currentGesture(): Boolean = syncViewModel.isCurrent(renderedEpoch, renderedGeneration, allowBusy = true) &&
        viewModel.state.value.selected == renderedSelected
    fun withdrawForGesture(): Boolean {
        if (!currentGesture()) return false
        syncViewModel.withdraw()
        showSync = false
        openedFor = null
        return true
    }
    fun requestUnlock() {
        if (!withdrawForGesture()) return
        viewModel.unconfirm()
        recovery = null
        val manager = androidContext.getSystemService(KeyguardManager::class.java)
        if (manager?.isDeviceSecure != true) { protectionNotice = R.string.enterprise_protection_device_lock_required; return }
        @Suppress("DEPRECATION")
        val intent = manager.createConfirmDeviceCredentialIntent(androidContext.getString(R.string.enterprise_protection_unlock_title),
            androidContext.getString(R.string.enterprise_protection_unlock_copy))
        if (intent == null) { protectionNotice = R.string.enterprise_context_unavailable; return }
        try { unlock.launch(intent) }
        catch (_: Exception) { protectionNotice = R.string.enterprise_context_unavailable }
    }
    val personal = { if (withdrawForGesture()) { viewModel.selectPersonal(); onPersonal() } }
    val signIn = { if (withdrawForGesture()) { viewModel.unconfirm(); onSignIn() } }
    val invitation = { if (withdrawForGesture()) { viewModel.unconfirm(); onInvitation() } }
    val syncMatches = resumed && renderedSelected != null && syncState.epoch == renderedEpoch &&
        syncState.context?.partition == renderedSelected.partition && syncState.context?.membershipId == renderedSelected.membershipId
    fun syncBack() {
        if (!currentGesture()) return
        if (syncMatches && syncState.detail != null) syncViewModel.backToList(renderedEpoch, renderedGeneration)
        else if (withdrawForGesture()) {
            renderedSelected?.let { syncViewModel.open(it, renderedEpoch, syncViewModel.state.value.generation) }
        }
    }
    SecureAccountWindow()
    DisposableEffect(viewModel, syncViewModel) { onDispose { syncViewModel.withdraw(); viewModel.unconfirm() } }
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { resumed = true; syncViewModel.invalidateIfStale(); viewModel.invalidateIfStale() }
    LifecycleEventEffect(Lifecycle.Event.ON_STOP) { resumed = false; showSync = false; openedFor = null; recovery = null; syncViewModel.withdraw(); viewModel.unconfirm() }
    LaunchedEffect(renderedSelected, renderedEpoch, resumed, choosing, state.busy, state.selectionConfirmed) {
        val selected = renderedSelected
        if (viewModel.state.value.selected != selected || viewModel.epochs.value != renderedEpoch) return@LaunchedEffect
        if (openedFor != null && (openedFor?.partition != selected?.partition || openedFor?.membershipId != selected?.membershipId)) {
            showSync = false
            openedFor = null
        }
        if (!resumed || selected == null) { syncViewModel.withdraw(); showSync = false; openedFor = null }
        else if (!choosing && !state.busy && resumed && lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED) &&
            !viewModel.state.value.busy && viewModel.state.value.selected == selected && viewModel.epochs.value == renderedEpoch) {
            // An old effect may not borrow a newer status generation for its old selector.
            val current = syncViewModel.state.value
            if (current.epoch != renderedEpoch || current.context?.partition != selected.partition || current.context?.membershipId != selected.membershipId) {
                syncViewModel.open(selected, renderedEpoch, current.generation)
            }
        }
    }
    LaunchedEffect(renderedEpoch, state.confirmationTarget) {
        recovery?.let { if (it.second != renderedEpoch || it.first != state.confirmationTarget) recovery = null }
    }
    LaunchedEffect(state.selected, state.selectionConfirmed) {
        if (state.selectionConfirmed && state.selected != null) choosing = false
    }
    LaunchedEffect(state.unlockRequired, state.erasedWorkAcknowledgementRequired) {
        if (state.unlockRequired || state.erasedWorkAcknowledgementRequired) choosing = false
    }
    BackHandler {
        when {
            showSync && resumed && openedFor?.partition == renderedSelected?.partition &&
                openedFor?.membershipId == renderedSelected?.membershipId -> syncBack()
            choosing -> choosing = false
            state.selected != null -> if (withdrawForGesture()) choosing = true
            else -> personal()
        }
    }
    AnpScaffold(
        modifier = Modifier.fillMaxSize().testTag("enterprise-context-screen"),
        topBar = {
            Column {
                AnpTopAppBar(title = { Text(stringResource(R.string.enterprise_context_home)) })
                state.selected?.let { selected ->
                    Text(
                        selected.organizationName,
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)
                            .testTag("enterprise-context-header-name").semantics { heading() },
                    )
                }
            }
        },
    ) { padding ->
        if (showSync && resumed && renderedSelected != null && !choosing &&
            openedFor?.partition == renderedSelected.partition && openedFor?.membershipId == renderedSelected.membershipId) EnterpriseSyncPanel(
            // A source loss can intentionally clear the flow owner. Keep its safe
            // recovery message visible while withholding every previous private fact.
            state = if (syncMatches) syncState else syncState.copy(items = emptyList(), totals = null,
                detail = null, active = emptySet(), hasMore = false, manualAttention = 0, manualWaitingMillis = null,
                error = syncState.error ?: EnterpriseAccessError.STALE),
            connected = connected,
            onBack = ::syncBack,
            onRefresh = { if (currentGesture()) {
                if (syncMatches) syncViewModel.refresh(renderedEpoch, renderedGeneration)
                else syncViewModel.open(renderedSelected, renderedEpoch, renderedGeneration)
            } },
            onMore = { if (currentGesture()) syncViewModel.loadMore(renderedEpoch, renderedGeneration) },
            onMode = { if (currentGesture()) syncViewModel.changeMode(it, renderedEpoch, renderedGeneration) },
            onDetail = { if (currentGesture()) syncViewModel.detail(it, renderedEpoch, renderedGeneration) },
            onManualCheck = { if (currentGesture()) syncViewModel.manualCheck(renderedEpoch, renderedGeneration) },
            onDismiss = { identity, receipt -> if (currentGesture()) syncViewModel.dismiss(identity, receipt, renderedEpoch, renderedGeneration) },
            modifier = Modifier.fillMaxSize().padding(padding),
        ) else Column(
            Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(stringResource(R.string.enterprise_test_environment), style = MaterialTheme.typography.labelLarge)
            protectionNotice?.let { Text(stringResource(it), modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite }) }
            val selected = state.selected
            if (selected == null) {
                ContextChooser(state, connected, renderedEpoch, viewModel, personal, signIn, invitation, ::withdrawForGesture)
            } else {
                Column(Modifier.testTag("enterprise-home"), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                    Text(
                        stringResource(R.string.enterprise_context_current, selected.organizationName),
                        style = MaterialTheme.typography.headlineSmall,
                        modifier = Modifier.testTag("enterprise-context-name").semantics { heading(); liveRegion = LiveRegionMode.Polite },
                    )
                    ContextStatus(state, connected)
                    ContextCard {
                        Text(stringResource(R.string.enterprise_context_empty_home))
                        Text(stringResource(R.string.enterprise_context_ownership), style = MaterialTheme.typography.bodyMedium)
                    }
                    EnterpriseSyncHomeStatus(if (syncMatches && syncState.error == null && !syncState.unlockRequired && !syncState.unrecoverable) syncState.totals else null,
                        onOpen = {
                            if (currentGesture() && !viewModel.state.value.busy && syncViewModel.open(selected, renderedEpoch, renderedGeneration)) {
                                openedFor = selected
                                showSync = true
                            }
                        })
                    if (!state.selectionConfirmed) Button(
                        onClick = { if (withdrawForGesture()) viewModel.select(selected, renderedEpoch) },
                        enabled = connected && !state.busy,
                        modifier = Modifier.fillMaxWidth().testTag("enterprise-context-reconfirm"),
                    ) { Text(stringResource(R.string.enterprise_context_reconfirm)) }
                    TextButton(
                        onClick = { if (withdrawForGesture()) choosing = true },
                        modifier = Modifier.fillMaxWidth().testTag("enterprise-context-switch"),
                    ) { Text(stringResource(R.string.enterprise_context_switch)) }
                    FreshContextSignIn(state, signIn)
                }
            }
            if (state.unlockRequired) {
                Text(stringResource(R.string.enterprise_protection_unlock_required))
                Button(onClick = ::requestUnlock, enabled = !state.busy, modifier = Modifier.fillMaxWidth().testTag("enterprise-protection-unlock")) {
                    Text(stringResource(R.string.enterprise_protection_unlock_action))
                }
            }
            state.confirmationTarget?.let { target -> if (state.erasedWorkAcknowledgementRequired) {
                Text(stringResource(R.string.enterprise_protection_recovery_required), modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite })
                Button(onClick = { if (withdrawForGesture()) recovery = target to renderedEpoch }, enabled = connected && !state.busy,
                    modifier = Modifier.fillMaxWidth().testTag("enterprise-protection-recover")) {
                    Text(stringResource(R.string.enterprise_protection_recovery_action, target.organizationName))
                }
            } }
        }
    }
    recovery?.let { (target, epoch) -> AlertDialog(
        onDismissRequest = { recovery = null },
        title = { Text(stringResource(R.string.enterprise_protection_recovery_title)) },
        text = { Text(stringResource(R.string.enterprise_protection_recovery_copy)) },
        confirmButton = { TextButton(onClick = {
            recovery = null
            if (viewModel.state.value.confirmationTarget == target && epoch == renderedEpoch && withdrawForGesture())
                viewModel.select(target, epoch, acknowledgeErasedWork = true)
        }, enabled = connected && !state.busy) { Text(stringResource(R.string.enterprise_protection_recovery_confirm)) } },
        dismissButton = { TextButton(onClick = { recovery = null }) { Text(stringResource(R.string.action_cancel)) } },
    ) }
    if (choosing && state.selected != null) ModalBottomSheet(
        onDismissRequest = { choosing = false },
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
    ) {
        Column(
            Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            ContextChooser(state, connected, renderedEpoch, viewModel, personal, signIn, invitation, ::withdrawForGesture)
            TextButton(onClick = { choosing = false }, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.enterprise_context_close))
            }
        }
    }
}

@Composable
private fun ContextChooser(
    state: EnterpriseContextState,
    connected: Boolean,
    renderedEpoch: Long,
    viewModel: EnterpriseContextViewModel,
    personal: () -> Unit,
    onSignIn: () -> Unit,
    onInvitation: () -> Unit,
    beforeContextChange: () -> Boolean,
) {
    Column(Modifier.testTag("enterprise-context-chooser"), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Text(stringResource(R.string.enterprise_context_title), style = MaterialTheme.typography.titleLarge, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_context_intro))
        Button(onClick = personal, modifier = Modifier.fillMaxWidth().testTag("enterprise-context-personal")) {
            Text(stringResource(R.string.enterprise_context_personal))
        }
        ContextStatus(state, connected, catalog = true)
        if (!state.signedIn) Text(stringResource(R.string.enterprise_context_sign_in))
        else if (state.contexts.isEmpty() && state.catalogConfirmed && !state.busy) Text(stringResource(R.string.enterprise_context_empty))
        for (context in state.contexts) ContextOption(
            context,
            connected && state.signedIn && !state.busy,
            onSelect = { if (beforeContextChange()) viewModel.select(context, renderedEpoch) },
        )
        Button(
            onClick = { if (beforeContextChange()) viewModel.refresh(renderedEpoch) },
            enabled = connected && state.signedIn && !state.busy,
            modifier = Modifier.fillMaxWidth().testTag("enterprise-context-refresh"),
        ) { Text(stringResource(R.string.enterprise_context_refresh)) }
        if (state.nextAfterOrganizationId != null) TextButton(
            onClick = { if (beforeContextChange()) viewModel.loadMore(renderedEpoch) },
            enabled = connected && state.signedIn && !state.busy,
            modifier = Modifier.fillMaxWidth().testTag("enterprise-context-next"),
        ) { Text(stringResource(R.string.enterprise_context_next)) }
        FreshContextSignIn(state, onSignIn)
        TextButton(onClick = onInvitation, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.enterprise_context_invitation))
        }
    }
}

@Composable
private fun ContextOption(context: EnterpriseContextSummary, enabled: Boolean, onSelect: () -> Unit) {
    ContextCard {
        Text(context.organizationName, style = MaterialTheme.typography.titleMedium)
        Button(
            onClick = onSelect,
            enabled = enabled,
            modifier = Modifier.fillMaxWidth().testTag("enterprise-context-option-${context.partition.organizationId}"),
        ) { Text(stringResource(R.string.enterprise_context_select, context.organizationName)) }
    }
}

@Composable
private fun ContextStatus(state: EnterpriseContextState, connected: Boolean, catalog: Boolean = false) {
    if (!connected) Text(stringResource(R.string.enterprise_context_offline))
    val unconfirmed = if (catalog) state.contexts.isNotEmpty() && !state.catalogConfirmed
        else state.selected != null && !state.selectionConfirmed
    if (unconfirmed) Text(
        stringResource(R.string.enterprise_context_unconfirmed),
        modifier = Modifier.testTag("enterprise-context-unconfirmed").semantics { liveRegion = LiveRegionMode.Polite },
    )
    else if (!catalog && state.selected != null && state.selectionConfirmed) Text(stringResource(R.string.enterprise_context_confirmed))
    if (state.busy) {
        CircularProgressIndicator()
        Text(stringResource(R.string.enterprise_context_busy), modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite })
    }
    state.error?.let { error -> Text(
        stringResource(when (error) {
            EnterpriseAccessError.ACCESS_DENIED -> R.string.enterprise_context_denied
            EnterpriseAccessError.SIGN_IN_REQUIRED, EnterpriseAccessError.FRESH_AUTH_REQUIRED -> R.string.enterprise_invitation_fresh_required
            EnterpriseAccessError.STALE -> R.string.enterprise_context_unconfirmed
            EnterpriseAccessError.RATE_LIMITED -> R.string.enterprise_invitation_limited
            else -> R.string.enterprise_context_unavailable
        }),
        color = MaterialTheme.colorScheme.error,
        modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
    ) }
}

@Composable
private fun FreshContextSignIn(state: EnterpriseContextState, onSignIn: () -> Unit) {
    if (!state.signedIn || state.error in setOf(EnterpriseAccessError.SIGN_IN_REQUIRED, EnterpriseAccessError.FRESH_AUTH_REQUIRED)) Button(
        onClick = onSignIn,
        enabled = !state.busy,
        modifier = Modifier.fillMaxWidth().testTag("enterprise-context-sign-in"),
    ) { Text(stringResource(R.string.enterprise_invitation_sign_in)) }
}

@Composable
private fun ContextCard(content: @Composable () -> Unit) {
    val fill = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = fill),
        border = AnpContainerDefaults.borderStroke(fill),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) { content() }
    }
}
