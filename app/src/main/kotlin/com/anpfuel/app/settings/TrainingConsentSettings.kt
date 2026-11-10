package com.anpfuel.app.settings

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.semantics.Role
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.LocalBottomSafePadding
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.OutlinedButton
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.repository.TrainingConsentReview

@Composable
fun TrainingConsentSettings(onSignIn:()->Unit,host:TrainingConsentHostViewModel=hiltViewModel(),vm:TrainingConsentViewModel=hiltViewModel()) {
    val state by host.state.collectAsStateWithLifecycle()
    val consent by vm.state.collectAsStateWithLifecycle()
    val lifecycle=LocalLifecycleOwner.current
    DisposableEffect(lifecycle,host) {
        val observer=LifecycleEventObserver { _,event -> if(event==Lifecycle.Event.ON_RESUME) host.refresh() }
        lifecycle.lifecycle.addObserver(observer)
        onDispose { lifecycle.lifecycle.removeObserver(observer) }
    }
    val caller=state.caller
    LaunchedEffect(state) { vm.prepare(caller?.accountId.orEmpty(),caller?.scope) }
    TrainingConsentContent(state,consent,vm::setEnabled,vm::retry,onSignIn)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun TrainingConsentContent(
    state:ConsentHostState,
    consent:TrainingConsentUiState,
    onDecision:(Boolean,String,String)->Unit,
    onRetry:()->Unit,
    onSignIn:()->Unit,
) {
    val caller=state.caller
    val document=state.terms
    val ownerKey=caller?.scope?.let { it.origin+"|"+it.ownerScope }.orEmpty()
    var review by remember(ownerKey,document?.version,document?.digest,state.loaded) {
        mutableStateOf(TrainingConsentReview(ownerKey,document?.version.orEmpty(),document?.digest.orEmpty()))
    }
    var reader by remember(ownerKey,document?.digest,state.loaded) { mutableStateOf(false) }
    // The modal box starts unticked every time the complete document is opened.
    var modalChecked by remember(reader) { mutableStateOf(false) }
    val busy=consent is TrainingConsentUiState.Updating || consent is TrainingConsentUiState.Loading
    val containerColor = MaterialTheme.colorScheme.surfaceContainerHighest
    Card(
        Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(Modifier.padding(16.dp),verticalArrangement=Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.privacy_training_heading),style=MaterialTheme.typography.titleMedium,modifier=Modifier.semantics {heading()})
            Text(stringResource(R.string.privacy_training_unavailable))
            Text(stringResource(R.string.privacy_training_optional))
            if(document!=null) {
                Text(stringResource(R.string.privacy_terms_language))
                OutlinedButton(onClick={reader=true},shape=RoundedCornerShape(12.dp),modifier=Modifier.fillMaxWidth()) {Text(stringResource(R.string.privacy_read_terms))}
                Text(document.version,style=MaterialTheme.typography.bodySmall)
            } else if(state.loaded) Text(stringResource(R.string.privacy_terms_unavailable))
            if(!state.loaded) CircularProgressIndicator()
            else if(caller==null) TextButton(onClick=onSignIn) {Text(stringResource(R.string.privacy_sign_in))}
            else {
                val content=consent as? TrainingConsentUiState.Content
                Text(stringResource(when {
                    consent is TrainingConsentUiState.Pending || content?.pendingRevocation==true -> R.string.privacy_revocation_pending
                    consent is TrainingConsentUiState.Offline -> R.string.privacy_offline
                    consent is TrainingConsentUiState.Rejected -> R.string.privacy_request_failed
                    content?.granted==true -> R.string.privacy_preference_received
                    else -> R.string.privacy_preference_off
                }))
                if(content?.granted==true) {
                    val version=content.termsVersion
                    val decided=content.decidedAt?.let(::consentDecisionText)
                    if(version!=null && decided!=null) Text(stringResource(R.string.privacy_recorded_details,version,decided),style=MaterialTheme.typography.bodySmall)
                }
                if(content?.needsRegrant==true) Text(stringResource(R.string.training_consent_regrant))
                if(document!=null && content?.granted!=true) {
                    Row(
                        // A tap is never a no-op: before the complete document was displayed it opens the modal.
                        Modifier.fillMaxWidth().toggleable(value=review.checked,enabled=!busy,role=Role.Checkbox,onValueChange={ if(review.read) review=review.check(it) else reader=true }),
                        verticalAlignment=Alignment.Top,
                        horizontalArrangement=Arrangement.spacedBy(12.dp),
                    ) {
                        Checkbox(checked=review.checked,onCheckedChange=null,enabled=!busy,modifier=Modifier.padding(top=2.dp))
                        Text(stringResource(R.string.privacy_consent_check),style=MaterialTheme.typography.bodyMedium,modifier=Modifier.weight(1f))
                    }
                    Button(onClick={ if(review.canConfirm) { onDecision(true,review.version,review.digest);review=review.copy(checked=false) } },enabled=review.canConfirm && !busy,shape=RoundedCornerShape(12.dp),modifier=Modifier.fillMaxWidth()) {
                        Text(stringResource(R.string.privacy_confirm_preference))
                    }
                }
                // OFF remains available without review, connectivity, terms, or a completed ON request.
                OutlinedButton(onClick={review=review.copy(checked=false);onDecision(false,"","")},shape=RoundedCornerShape(12.dp),modifier=Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.privacy_revoke_delete))
                }
                Text(stringResource(R.string.privacy_deletion_receipt_note),style=MaterialTheme.typography.bodySmall)
                TextButton(onClick=onRetry,enabled=!busy) {Text(stringResource(R.string.action_retry))}
            }
        }
    }
    if(reader && document!=null) {
        // Letterboxed dialog frame: the window manager keeps the frame
        // outside the status and navigation bars, so the footer can never
        // slide under them. Framework insets apply when a device
        // dispatches them; otherwise they read zero and add nothing.
        Dialog(
            onDismissRequest = { reader = false },
            properties = DialogProperties(usePlatformDefaultWidth = false),
        ) {
            Surface(Modifier.fillMaxSize()) {
                // Back the framework bottom value with the shell-provided
                // safe padding: dialog windows on some devices report a
                // partial bottom inset while the main window reports the
                // full navigation height. maxOf keeps a single pass on
                // devices where both agree.
                val frameworkBottom = WindowInsets.navigationBars
                    .asPaddingValues()
                    .calculateBottomPadding()
                val shellBottom = LocalBottomSafePadding.current
                Column(
                    Modifier
                        .fillMaxSize()
                        .statusBarsPadding()
                        .padding(bottom = 32.dp + maxOf(frameworkBottom, shellBottom)),
                ) {
                    AnpTopAppBar(
                        title = { Text(stringResource(R.string.privacy_read_terms)) },
                        onNavigateUp = { reader = false },
                        applyStatusBarInsets = false,
                    )
                    Column(Modifier.weight(1f).verticalScroll(rememberScrollState()).padding(horizontal = 20.dp, vertical = 12.dp),verticalArrangement=Arrangement.spacedBy(16.dp)) {
                        SelectionContainer {Text(document.text)}
                        Text(stringResource(R.string.privacy_digest,document.digest),style=MaterialTheme.typography.bodySmall)
                    }
                    // B-BR-CS06: the complete document and the unticked box share this modal.
                    val canGrant = caller != null && (consent as? TrainingConsentUiState.Content)?.granted != true
                    if (canGrant) {
                        Text(
                            stringResource(R.string.privacy_modal_hint),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(horizontal = 20.dp, vertical = 4.dp),
                        )
                        Row(
                            Modifier
                                .fillMaxWidth()
                                .toggleable(value = modalChecked, enabled = !busy, role = Role.Checkbox, onValueChange = { modalChecked = it })
                                .padding(horizontal = 20.dp, vertical = 8.dp),
                            verticalAlignment = Alignment.Top,
                            horizontalArrangement = Arrangement.spacedBy(12.dp),
                        ) {
                            Checkbox(checked = modalChecked, onCheckedChange = null, enabled = !busy, modifier = Modifier.padding(top = 2.dp))
                            Text(stringResource(R.string.privacy_consent_check), style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
                        }
                        Button(
                            onClick = {
                                val accepted = review.finishReading().check(modalChecked)
                                if (accepted.canConfirm) onDecision(true, accepted.version, accepted.digest)
                                review = accepted.copy(checked = false)
                                reader = false
                            },
                            enabled = modalChecked && !busy,
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 20.dp),
                        ) {
                            Text(stringResource(R.string.privacy_accept_record))
                        }
                    }
                    TextButton(
                        onClick = { review = review.finishReading(); reader = false },
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 20.dp),
                    ) {
                        Text(stringResource(R.string.privacy_finish_reading))
                    }
                }
            }
        }
    }
}

/**
 * Rights contact card (B-BR-CS06): access, correction, revocation or
 * erasure without accepting training. Account-container standard
 * (rounded 24 dp, low container, 24 dp padding), same formatting as
 * the guarantees card; shown right below it on the Privacy screen.
 */
@Composable
fun TrainingConsentRightsCard(modifier: Modifier = Modifier) {
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = CardDefaults.shape,
        colors = CardDefaults.cardColors(
            containerColor = containerColor,
        ),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(
            modifier = Modifier.padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = stringResource(R.string.privacy_rights_heading),
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.semantics { heading() },
            )
            SelectionContainer {
                Text(
                    text = stringResource(R.string.privacy_rights_body),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

/** Server decision instant (RFC 3339) in the device locale and zone; null when it cannot be read. */
internal fun consentDecisionText(decidedAt:String,zone:java.time.ZoneId=java.time.ZoneId.systemDefault()):String? = runCatching {
    java.time.format.DateTimeFormatter.ofLocalizedDateTime(java.time.format.FormatStyle.MEDIUM,java.time.format.FormatStyle.SHORT)
        .format(java.time.OffsetDateTime.parse(decidedAt).atZoneSameInstant(zone))
}.getOrNull()
