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
) {
    // These are cheap local invalidation flows; keep private presentation current
    // while stopped rather than briefly resuming an earlier confirmed context.
    val state by viewModel.state.collectAsState()
    val connected by viewModel.connected.collectAsState()
    // Old UI callbacks retain their rendered primitive, never the newer delegate.
    val renderedEpoch = viewModel.epochs.collectAsState().value
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
    fun requestUnlock() {
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
    val personal = { viewModel.selectPersonal(); onPersonal() }
    SecureAccountWindow()
    DisposableEffect(viewModel) { onDispose { viewModel.unconfirm() } }
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { viewModel.invalidateIfStale() }
    LifecycleEventEffect(Lifecycle.Event.ON_STOP) { recovery = null; viewModel.unconfirm() }
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
            choosing -> choosing = false
            state.selected != null -> choosing = true
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
        Column(
            Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(stringResource(R.string.enterprise_test_environment), style = MaterialTheme.typography.labelLarge)
            protectionNotice?.let { Text(stringResource(it), modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite }) }
            val selected = state.selected
            if (selected == null) {
                ContextChooser(state, connected, renderedEpoch, viewModel, personal, onSignIn, onInvitation)
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
                    if (!state.selectionConfirmed) Button(
                        onClick = { viewModel.select(selected, renderedEpoch) },
                        enabled = connected && !state.busy,
                        modifier = Modifier.fillMaxWidth().testTag("enterprise-context-reconfirm"),
                    ) { Text(stringResource(R.string.enterprise_context_reconfirm)) }
                    TextButton(
                        onClick = { choosing = true },
                        modifier = Modifier.fillMaxWidth().testTag("enterprise-context-switch"),
                    ) { Text(stringResource(R.string.enterprise_context_switch)) }
                    FreshContextSignIn(state, viewModel, onSignIn)
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
                Button(onClick = { recovery = target to renderedEpoch }, enabled = connected && !state.busy,
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
            ContextChooser(state, connected, renderedEpoch, viewModel, personal, onSignIn, onInvitation)
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
            onSelect = { viewModel.select(context, renderedEpoch) },
        )
        Button(
            onClick = { viewModel.refresh(renderedEpoch) },
            enabled = connected && state.signedIn && !state.busy,
            modifier = Modifier.fillMaxWidth().testTag("enterprise-context-refresh"),
        ) { Text(stringResource(R.string.enterprise_context_refresh)) }
        if (state.nextAfterOrganizationId != null) TextButton(
            onClick = { viewModel.loadMore(renderedEpoch) },
            enabled = connected && state.signedIn && !state.busy,
            modifier = Modifier.fillMaxWidth().testTag("enterprise-context-next"),
        ) { Text(stringResource(R.string.enterprise_context_next)) }
        FreshContextSignIn(state, viewModel, onSignIn)
        TextButton(onClick = { viewModel.unconfirm(); onInvitation() }, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) {
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
private fun FreshContextSignIn(state: EnterpriseContextState, viewModel: EnterpriseContextViewModel, onSignIn: () -> Unit) {
    if (!state.signedIn || state.error in setOf(EnterpriseAccessError.SIGN_IN_REQUIRED, EnterpriseAccessError.FRESH_AUTH_REQUIRED)) Button(
        onClick = { viewModel.unconfirm(); onSignIn() },
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
