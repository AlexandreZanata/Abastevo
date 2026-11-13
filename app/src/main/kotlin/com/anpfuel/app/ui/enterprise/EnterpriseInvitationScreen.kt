package com.anpfuel.app.ui.enterprise

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.Alignment
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.auth.SecureAccountWindow
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseInvitationDecision
import com.anpfuel.application.enterprise.EnterpriseInvitationStage

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EnterpriseInvitationScreen(
    onNavigateBack: () -> Unit,
    onSignIn: () -> Unit,
    onContexts: () -> Unit = {},
    viewModel: EnterpriseInvitationViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val connected by viewModel.connected.collectAsStateWithLifecycle()
    // Observe this cheap invalidation signal even while stopped: the first resumed
    // input must already use the new identity epoch, before other UI collectors restart.
    // Capture the rendered primitive: stale callbacks must not read a newer epoch
    // while still retaining the preceding composition's private input holder.
    val inputEpoch = viewModel.epochs.collectAsState().value
    // Never use rememberSaveable, navigation arguments or SavedState for this input.
    // Reset during composition, so an old asynchronous effect cannot erase new input.
    var code by remember(viewModel, inputEpoch, state.stage, state.error) { mutableStateOf("") }
    var consent by remember(viewModel, inputEpoch, state.preview, state.error) { mutableStateOf(false) }
    val focus = LocalFocusManager.current
    val back = { viewModel.restart(); onNavigateBack() }
    SecureAccountWindow()
    BackHandler(onBack = back)
    DisposableEffect(viewModel) { onDispose { viewModel.restart() } }
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { viewModel.invalidateIfStale() }
    LifecycleEventEffect(Lifecycle.Event.ON_STOP) {
        code = ""; consent = false
        focus.clearFocus()
        viewModel.invalidateIfStale()
    }
    AnpScaffold(
        modifier = Modifier.fillMaxSize().testTag("enterprise-invitation-screen"),
        topBar = { AnpTopAppBar(title = { Text(stringResource(R.string.enterprise_invitation_title)) }, onNavigateUp = back) },
    ) { padding ->
        Column(
            Modifier.fillMaxSize().padding(padding).imePadding().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(stringResource(R.string.enterprise_test_environment), style = MaterialTheme.typography.labelLarge)
            if (!connected) Text(
                stringResource(R.string.enterprise_invitation_offline),
                modifier = Modifier.testTag("enterprise-invitation-offline").semantics { liveRegion = LiveRegionMode.Polite },
            )
            val fill = MaterialTheme.colorScheme.surfaceContainerLow
            Card(
                modifier = Modifier.fillMaxWidth(),
                shape = AnpContainerDefaults.containerShape(),
                colors = CardDefaults.cardColors(containerColor = fill),
                border = AnpContainerDefaults.borderStroke(fill),
            ) {
                Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                    when (state.stage) {
                        EnterpriseInvitationStage.ENTRY -> {
                            Text(stringResource(R.string.enterprise_invitation_intro))
                            if (!state.signedIn) Text(stringResource(R.string.enterprise_invitation_account_required))
                            OutlinedTextField(
                                value = code,
                                onValueChange = { if (inputEpoch == viewModel.epochs.value) code = it.take(256) },
                                label = { Text(stringResource(R.string.enterprise_invitation_code_label)) },
                                modifier = Modifier.fillMaxWidth().testTag("enterprise-invitation-code"),
                                enabled = !state.busy && state.signedIn,
                                singleLine = true,
                                visualTransformation = PasswordVisualTransformation(),
                                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, autoCorrectEnabled = false),
                            )
                            Button(
                                onClick = { val submitted = code; focus.clearFocus(); viewModel.preview(submitted, inputEpoch) },
                                enabled = connected && state.signedIn && !state.busy && code.isNotBlank(),
                                modifier = Modifier.fillMaxWidth().testTag("enterprise-invitation-preview"),
                            ) { Text(stringResource(R.string.enterprise_invitation_preview)) }
                        }
                        EnterpriseInvitationStage.PREVIEW -> state.preview?.let { preview ->
                            Text(preview.organizationName, style = MaterialTheme.typography.titleLarge, modifier = Modifier.semantics { heading() })
                            Text(stringResource(if (preview.legalEntityVerified) R.string.enterprise_invitation_verified else R.string.enterprise_invitation_unverified))
                            Text(stringResource(R.string.enterprise_invitation_roles), style = MaterialTheme.typography.titleSmall)
                            for (grant in preview.grants) Text(
                                stringResource(R.string.enterprise_invitation_role_scope, enterpriseRoleName(grant.role), stringResource(if (grant.scope == "ORG") R.string.enterprise_scope_organization else R.string.enterprise_scope_self)),
                            )
                            Text(stringResource(R.string.enterprise_invitation_privacy))
                            if (preview.confirmationRequired) Text(stringResource(R.string.enterprise_invitation_confirmation_required))
                            Row(
                                modifier = Modifier.fillMaxWidth().testTag("enterprise-invitation-consent").toggleable(
                                    value = consent,
                                    enabled = connected && !state.busy && state.retryDecision == null,
                                    role = Role.Checkbox,
                                    onValueChange = { consent = it },
                                ).semantics(mergeDescendants = true) {},
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Checkbox(
                                    checked = consent,
                                    onCheckedChange = null,
                                    enabled = connected && !state.busy && state.retryDecision == null,
                                )
                                Text(stringResource(R.string.enterprise_invitation_consent), modifier = Modifier.weight(1f))
                            }
                            Button(
                                onClick = { viewModel.accept(consent || state.retryDecision == EnterpriseInvitationDecision.ACCEPT) },
                                enabled = connected && !state.busy && (consent || state.retryDecision == EnterpriseInvitationDecision.ACCEPT) && state.retryDecision != EnterpriseInvitationDecision.DECLINE,
                                modifier = Modifier.fillMaxWidth().testTag("enterprise-invitation-accept"),
                            ) { Text(stringResource(if (state.retryDecision == EnterpriseInvitationDecision.ACCEPT) R.string.enterprise_invitation_retry else R.string.enterprise_invitation_accept)) }
                            TextButton(
                                onClick = viewModel::decline,
                                enabled = connected && !state.busy && state.retryDecision != EnterpriseInvitationDecision.ACCEPT,
                                modifier = Modifier.fillMaxWidth().testTag("enterprise-invitation-decline"),
                            ) { Text(stringResource(if (state.retryDecision == EnterpriseInvitationDecision.DECLINE) R.string.enterprise_invitation_retry else R.string.enterprise_invitation_decline)) }
                        }
                        EnterpriseInvitationStage.RESULT -> state.acknowledgement?.let { acknowledgement ->
                            Text(
                                stringResource(when {
                                    acknowledgement.active -> R.string.enterprise_invitation_active
                                    acknowledgement.awaitingConfirmation -> R.string.enterprise_invitation_pending
                                    else -> R.string.enterprise_invitation_declined
                                }),
                                modifier = Modifier.testTag("enterprise-invitation-result").semantics { liveRegion = LiveRegionMode.Polite },
                                style = MaterialTheme.typography.titleMedium,
                            )
                            if (acknowledgement.active) Button(
                                onClick = { viewModel.restart(); onContexts() },
                                modifier = Modifier.fillMaxWidth().testTag("enterprise-invitation-contexts"),
                            ) { Text(stringResource(R.string.enterprise_context_my_companies)) }
                            Button(onClick = back, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.enterprise_invitation_done)) }
                        }
                    }
                    if (state.busy) {
                        CircularProgressIndicator()
                        Text(stringResource(R.string.enterprise_invitation_busy), modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite })
                    }
                    state.error?.let { Text(stringResource(invitationError(it)), color = MaterialTheme.colorScheme.error, modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite }) }
                    if (!state.signedIn || state.error == EnterpriseAccessError.FRESH_AUTH_REQUIRED || state.error == EnterpriseAccessError.SIGN_IN_REQUIRED) {
                        Button(
                            onClick = { code = ""; consent = false; viewModel.restart(); onSignIn() },
                            enabled = !state.busy,
                            modifier = Modifier.fillMaxWidth().testTag("enterprise-invitation-sign-in"),
                        ) { Text(stringResource(R.string.enterprise_invitation_sign_in)) }
                    }
                    if (state.stage != EnterpriseInvitationStage.RESULT) TextButton(
                        onClick = { code = ""; consent = false; viewModel.restart() },
                        enabled = !state.busy && state.retryDecision == null,
                        modifier = Modifier.fillMaxWidth().testTag("enterprise-invitation-restart"),
                    ) { Text(stringResource(R.string.enterprise_invitation_restart)) }
                }
            }
        }
    }
}

private fun invitationError(error: EnterpriseAccessError): Int = when (error) {
    EnterpriseAccessError.SIGN_IN_REQUIRED, EnterpriseAccessError.FRESH_AUTH_REQUIRED -> R.string.enterprise_invitation_fresh_required
    EnterpriseAccessError.INVALID_CODE -> R.string.enterprise_invitation_invalid
    EnterpriseAccessError.RATE_LIMITED -> R.string.enterprise_invitation_limited
    EnterpriseAccessError.ALREADY_MEMBER -> R.string.enterprise_invitation_already_member
    EnterpriseAccessError.STALE -> R.string.enterprise_invitation_stale
    EnterpriseAccessError.MANAGED_RESTRICTED -> R.string.enterprise_invitation_unavailable
    EnterpriseAccessError.CONFLICT, EnterpriseAccessError.UNAVAILABLE, EnterpriseAccessError.ACCESS_DENIED -> R.string.enterprise_invitation_unavailable
}

@Composable
internal fun enterpriseRoleName(role: String): String = stringResource(when (role) {
    "OWNER" -> R.string.enterprise_role_owner
    "ADMIN" -> R.string.enterprise_role_admin
    "FLEET_MANAGER" -> R.string.enterprise_role_fleet_manager
    "SUPERVISOR" -> R.string.enterprise_role_supervisor
    "FINANCE" -> R.string.enterprise_role_finance
    "APPROVER" -> R.string.enterprise_role_approver
    "DRIVER" -> R.string.enterprise_role_driver
    "OPERATIONS_STAFF" -> R.string.enterprise_role_operations_staff
    "AUDITOR" -> R.string.enterprise_role_auditor
    "DATA_MANAGER" -> R.string.enterprise_role_data_manager
    else -> R.string.enterprise_role_unknown
})
