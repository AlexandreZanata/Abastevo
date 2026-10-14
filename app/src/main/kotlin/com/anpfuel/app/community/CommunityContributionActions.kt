package com.anpfuel.app.community

import androidx.compose.foundation.layout.Column
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.*
import androidx.compose.ui.res.stringResource
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.domain.community.CommunityFeedItem

/** Exact helpfulness tallies and explicit signed price/report actions, scoped to a public revision. */
@Composable
fun CommunityContributionActions(item:CommunityFeedItem,onSignIn:()->Unit) {
    val reference=item.contribution
    if(reference==null) {Text(stringResource(R.string.community_action_unavailable));return}
    val vm:CommunityHelpfulnessViewModel=hiltViewModel(key="community-actions:${reference.observationId}:${reference.decisionSequence}")
    val state by vm.state.collectAsStateWithLifecycle()
    val received by vm.actionReceived.collectAsStateWithLifecycle()
    var showReport by remember(reference) {mutableStateOf(false)}
    LaunchedEffect(reference,vm) {vm.prepare(reference.observationId,reference.decisionSequence,vm.signedAccount())}
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) {vm.prepare(reference.observationId,reference.decisionSequence,vm.signedAccount())}
    DisposableEffect(vm) {onDispose {vm.clear()}}
    Column {
        CommunityHelpfulnessPanel(state,vm::vote,vm::remove,vm::refresh,{showReport=true},onSignIn)
        if(state is CommunityHelpfulnessUiState.Content) {
            TextButton(onClick=vm::confirmPrice) {Text(stringResource(R.string.community_confirm_price))}
        }
        if(received) Text(stringResource(R.string.community_action_received))
    }
    if(showReport) AlertDialog(
        onDismissRequest={showReport=false},
        title={Text(stringResource(R.string.community_helpful_report))},
        text={Column {
            listOf("PRICE_CHANGED" to R.string.community_report_price_changed,
                "WRONG_STATION" to R.string.community_report_wrong_station,
                "WRONG_PRODUCT" to R.string.community_report_wrong_product,
                "WRONG_CONDITION" to R.string.community_report_wrong_condition,
                "EVIDENCE_MISMATCH" to R.string.community_report_evidence,
                "OTHER" to R.string.community_report_other).forEach {(reason,label) ->
                TextButton(onClick={showReport=false;vm.report(reason)}) {Text(stringResource(label))}
            }
        }},
        confirmButton={TextButton(onClick={showReport=false}) {Text(stringResource(R.string.action_back))}},
    )
}
