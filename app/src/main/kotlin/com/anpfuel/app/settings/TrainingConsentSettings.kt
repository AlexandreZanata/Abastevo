package com.anpfuel.app.settings

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.selection.toggleable
import androidx.compose.ui.semantics.Role
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.*
import androidx.compose.runtime.*
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
    val busy=consent is TrainingConsentUiState.Updating || consent is TrainingConsentUiState.Loading
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp),verticalArrangement=Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.privacy_training_heading),style=MaterialTheme.typography.titleMedium,modifier=Modifier.semantics {heading()})
            Text(stringResource(R.string.privacy_training_unavailable))
            Text(stringResource(R.string.privacy_training_optional))
            if(document!=null) {
                Text(stringResource(R.string.privacy_terms_language))
                TextButton(onClick={reader=true}) {Text(stringResource(R.string.privacy_read_terms))}
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
                if(content?.needsRegrant==true) Text(stringResource(R.string.training_consent_regrant))
                if(document!=null && content?.granted!=true) {
                    Row(Modifier.fillMaxWidth().toggleable(value=review.checked,enabled=review.read && !busy,role=Role.Checkbox,onValueChange={review=review.check(it)})) {
                        Checkbox(checked=review.checked,onCheckedChange=null,enabled=review.read && !busy)
                        Text(stringResource(R.string.privacy_consent_check),Modifier.weight(1f).padding(top=12.dp))
                    }
                    Button(onClick={ if(review.canConfirm) { onDecision(true,review.version,review.digest);review=review.copy(checked=false) } },enabled=review.canConfirm && !busy,modifier=Modifier.fillMaxWidth()) {
                        Text(stringResource(R.string.privacy_confirm_preference))
                    }
                }
                // OFF remains available without review, connectivity, terms, or a completed ON request.
                OutlinedButton(onClick={review=review.copy(checked=false);onDecision(false,"","")},modifier=Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.privacy_revoke_delete))
                }
                Text(stringResource(R.string.privacy_deletion_receipt_note),style=MaterialTheme.typography.bodySmall)
                TextButton(onClick=onRetry,enabled=!busy) {Text(stringResource(R.string.action_retry))}
            }
        }
    }
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp),verticalArrangement=Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.privacy_rights_heading),style=MaterialTheme.typography.titleMedium,modifier=Modifier.semantics {heading()})
            SelectionContainer {Text(stringResource(R.string.privacy_rights_body))}
        }
    }
    if(reader && document!=null) {
        Dialog(onDismissRequest={reader=false},properties=DialogProperties(usePlatformDefaultWidth=false)) {
            Surface(Modifier.fillMaxSize()) {
                Column(Modifier.fillMaxSize().padding(20.dp)) {
                    Text(stringResource(R.string.privacy_read_terms),style=MaterialTheme.typography.titleLarge)
                    TextButton(onClick={reader=false}) {Text(stringResource(R.string.action_back))}
                    Column(Modifier.weight(1f).verticalScroll(rememberScrollState()),verticalArrangement=Arrangement.spacedBy(16.dp)) {
                        SelectionContainer {Text(document.text)}
                        Text(stringResource(R.string.privacy_digest,document.digest),style=MaterialTheme.typography.bodySmall)
                        Button(onClick={review=review.finishReading();reader=false},modifier=Modifier.fillMaxWidth()) {
                            Text(stringResource(R.string.privacy_finish_reading))
                        }
                    }
                }
            }
        }
    }
}
