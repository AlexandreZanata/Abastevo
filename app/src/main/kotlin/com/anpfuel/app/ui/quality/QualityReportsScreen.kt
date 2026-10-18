package com.anpfuel.app.ui.quality

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.auth.SecureAccountWindow
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.domain.quality.QualityReportFailure

@OptIn(ExperimentalMaterial3Api::class,ExperimentalLayoutApi::class)
@Composable
fun QualityReportsScreen(onBack:()->Unit,onSignIn:()->Unit,viewModel:QualityReportsViewModel=hiltViewModel()) {
    SecureAccountWindow()
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME){viewModel.resume()}
    LifecycleEventEffect(Lifecycle.Event.ON_STOP){viewModel.pause()}
    val state by viewModel.state.collectAsStateWithLifecycle()
    var guidance by remember {mutableStateOf(false)}
    var browserFailed by remember {mutableStateOf(false)}
    val uri=LocalUriHandler.current
    AnpScaffold(topBar={AnpTopAppBar(title={Text(stringResource(R.string.quality_reports_title))},onNavigateUp=onBack)}) {padding ->
        LazyColumn(Modifier.fillMaxSize().padding(padding),contentPadding=PaddingValues(20.dp),verticalArrangement=Arrangement.spacedBy(16.dp)) {
            item {
                Text(stringResource(R.string.quality_reports_note))
                if(state.busy)LinearProgressIndicator(Modifier.fillMaxWidth())
                if(state.visible && state.owner==null)Button(onClick=onSignIn){Text(stringResource(R.string.quality_reports_login))}
                state.error?.takeIf {state.visible}?.let {error ->
                    Text(stringResource(when(error){
                        QualityReportFailure.INVALID->R.string.quality_reports_invalid
                        QualityReportFailure.AUTH_REQUIRED,QualityReportFailure.OWNER_CHANGED->R.string.quality_reports_account
                        QualityReportFailure.CONFLICT->R.string.quality_reports_conflict
                        QualityReportFailure.NOT_FOUND->R.string.quality_reports_missing
                        QualityReportFailure.QUOTA->R.string.quality_reports_quota
                        else->R.string.quality_reports_failed
                    }),color=MaterialTheme.colorScheme.error)
                }
                if(state.pending!=null && state.visible) {
                    Text(stringResource(R.string.quality_reports_pending))
                    Button(onClick=viewModel::retry,enabled=!state.busy){Text(stringResource(R.string.action_retry))}
                }
                if(state.receipt!=null && state.visible)Text(stringResource(R.string.quality_reports_received))
                OutlinedButton(onClick={guidance=true}){Text(stringResource(R.string.quality_reports_anp))}
                if(browserFailed)Text(stringResource(R.string.quality_reports_browser_failed))
            }
            if(state.owner!=null && state.visible) {
                val selected=state.selected
                val editable=!state.busy && state.pending==null && state.confirmation==null
                item {
                    Text(stringResource(if(selected==null && viewModel.linkageReview)R.string.quality_reports_link else R.string.quality_reports_experience),style=MaterialTheme.typography.titleMedium)
                    if(selected!=null) {
                        Text(stringResource(reportStatus(selected.status)))
                        Text(stringResource(if(selected.evidenceLevel=="USER_CLAIM")R.string.quality_reports_claim else R.string.quality_reports_evidence_unknown))
                        if(selected.reviewReason.isNotEmpty())Text(stringResource(reviewReason(selected.reviewReason)))
                    }
                    OutlinedTextField(value=state.day,onValueChange=viewModel::day,label={Text(stringResource(R.string.quality_reports_day))},enabled=editable,modifier=Modifier.fillMaxWidth(),singleLine=true)
                    FlowRow(horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                        listOf("STARTING","PERFORMANCE","CONSUMPTION","OTHER").forEach {symptom ->
                            FilterChip(selected=state.symptom==symptom,onClick={viewModel.symptom(symptom)},enabled=editable,label={Text(stringResource(when(symptom){"STARTING"->R.string.quality_reports_starting;"PERFORMANCE"->R.string.quality_reports_performance;"CONSUMPTION"->R.string.quality_reports_consumption;else->R.string.quality_reports_other}))})
                        }
                    }
                    OutlinedTextField(value=state.text,onValueChange=viewModel::text,label={Text(stringResource(R.string.quality_reports_text))},supportingText={Text(stringResource(R.string.quality_reports_text_limit))},enabled=editable,modifier=Modifier.fillMaxWidth(),minLines=3,maxLines=8)
                    if(selected==null || selected.status in setOf("PENDING_REVIEW","REVIEWED"))Button(onClick={viewModel.request(if(selected==null)"SUBMIT" else "EDIT")},enabled=editable){Text(stringResource(if(selected==null)R.string.quality_reports_submit else R.string.quality_reports_edit))}
                    if(selected!=null && selected.status in setOf("HIDDEN","REVIEWED"))OutlinedButton(onClick={viewModel.request("APPEAL")},enabled=editable){Text(stringResource(R.string.quality_reports_appeal))}
                    if(selected!=null && selected.status in setOf("PENDING_REVIEW","REVIEWED","HIDDEN"))OutlinedButton(onClick={viewModel.request("WITHDRAW")},enabled=editable){Text(stringResource(R.string.quality_reports_withdraw))}
                    TextButton(onClick=viewModel::newReport,enabled=editable){Text(stringResource(R.string.quality_reports_new))}
                }
                item {
                    Text(stringResource(R.string.quality_reports_own),style=MaterialTheme.typography.titleMedium)
                    OutlinedButton(onClick=viewModel::resume,enabled=!state.busy){Text(stringResource(R.string.quality_reports_refresh))}
                    if(state.reports.isEmpty() && !state.busy)Text(stringResource(R.string.quality_reports_empty))
                }
                items(state.reports,key={it.id}) {report ->
                    OutlinedCard(onClick={viewModel.select(report)},enabled=editable,modifier=Modifier.fillMaxWidth()) {
                        Column(Modifier.padding(16.dp),verticalArrangement=Arrangement.spacedBy(6.dp)) {
                            Text(report.occurredOn)
                            Text(stringResource(reportStatus(report.status)))
                            Text(stringResource(if(report.evidenceLevel=="USER_CLAIM")R.string.quality_reports_claim else R.string.quality_reports_evidence_unknown))
                            Text(report.text)
                        }
                    }
                }
                if(state.nextAfterId!=null)item {Button(onClick=viewModel::nextPage,enabled=!state.busy && state.pending==null){Text(stringResource(R.string.quality_reports_more))}}
            }
        }
    }
    state.confirmation?.takeIf {state.visible}?.let {command ->
        AlertDialog(onDismissRequest=viewModel::dismissConfirmation,title={Text(stringResource(R.string.quality_reports_confirm_title))},text={Text(stringResource(when(command.action){"WITHDRAW"->R.string.quality_reports_confirm_withdraw;"APPEAL"->R.string.quality_reports_confirm_appeal;else->R.string.quality_reports_confirm_submit}))},confirmButton={TextButton(onClick=viewModel::confirm){Text(stringResource(R.string.quality_reports_confirm))}},dismissButton={TextButton(onClick=viewModel::dismissConfirmation){Text(stringResource(R.string.action_cancel))}})
    }
    if(guidance)AlertDialog(onDismissRequest={guidance=false},title={Text(stringResource(R.string.quality_reports_anp))},text={Text(stringResource(R.string.quality_reports_anp_handoff))},confirmButton={TextButton(onClick={guidance=false;browserFailed=runCatching {uri.openUri("https://www.gov.br/anp/pt-br/canais_atendimento/ouvidoria")}.isFailure}){Text(stringResource(R.string.quality_reports_open))}},dismissButton={TextButton(onClick={guidance=false}){Text(stringResource(R.string.action_cancel))}})
}

private fun reportStatus(status:String):Int=when(status){"PENDING_REVIEW"->R.string.quality_reports_status_pending;"REVIEWED"->R.string.quality_reports_status_reviewed;"HIDDEN"->R.string.quality_reports_status_hidden;"WITHDRAWN"->R.string.quality_reports_status_withdrawn;else->R.string.quality_reports_status_unknown}

private fun reviewReason(reason:String):Int=when(reason){
    "TRIAGE"->R.string.quality_reports_reason_triage
    "CLAIM_RECORDED"->R.string.quality_reports_reason_recorded
    "DUPLICATE"->R.string.quality_reports_reason_duplicate
    "INSUFFICIENT_EVIDENCE"->R.string.quality_reports_reason_insufficient
    "UNSUPPORTED_LINK"->R.string.quality_reports_reason_link
    "ABUSE"->R.string.quality_reports_reason_abuse
    "PRIVACY"->R.string.quality_reports_reason_privacy
    else->R.string.quality_reports_reason_unknown
}
