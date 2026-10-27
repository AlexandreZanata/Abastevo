package com.anpfuel.app.ui.contributors

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.contributor.Claim
import com.anpfuel.domain.contributor.ClaimState
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.ui.platform.LocalContext

/**
 * B-BR-CTB claim self-service (CTB-02/03/04), signed-in only.
 *
 * Enter GitHub login → one-time code with copy button and gist
 * instructions → submit gist id → claim status. Withdraw is available
 * while pending. The server owns quotas and decisions.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ClaimScreen(
    onNavigateBack: () -> Unit,
    onEditProfile: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ClaimViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val context = LocalContext.current
    LaunchedEffect(viewModel) { viewModel.refresh() }
    ClaimContent(
        state = state,
        onOpenClaim = viewModel::open,
        onSubmitProof = viewModel::prove,
        onWithdraw = viewModel::withdraw,
        onDismissIssued = viewModel::dismissIssued,
        onCopyCode = { code ->
            val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
            clipboard.setPrimaryClip(ClipData.newPlainText("contributor-code", code))
        },
        onEditProfile = onEditProfile,
        onRetry = viewModel::refresh,
        onNavigateBack = onNavigateBack,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ClaimContent(
    state: ClaimUiState,
    onOpenClaim: (String) -> Unit,
    onSubmitProof: (claimId: String, gistId: String) -> Unit,
    onWithdraw: (claimId: String) -> Unit,
    onDismissIssued: () -> Unit,
    onCopyCode: (String) -> Unit,
    onEditProfile: () -> Unit,
    onRetry: () -> Unit,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.claim_screen_title)) },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { padding ->
        when {
            state.loading -> SkeletonGroup(modifier = Modifier.fillMaxWidth().padding(padding)) {
                SkeletonCard(height = 96.dp)
                SkeletonCard(height = 64.dp)
            }

            state.notice != null -> Column(
                modifier = Modifier.fillMaxSize().padding(padding).padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text(
                    text = stringResource(
                        when (state.notice) {
                            ClaimNotice.OFFLINE -> R.string.contributors_offline
                            ClaimNotice.DENIED -> R.string.claim_signed_out
                            ClaimNotice.QUOTA -> R.string.claim_quota
                            ClaimNotice.CONFLICT -> R.string.claim_conflict
                            ClaimNotice.INVALID -> R.string.claim_invalid
                            ClaimNotice.ERROR -> R.string.contributors_error
                        },
                    ),
                    style = MaterialTheme.typography.bodyLarge,
                )
                Button(onClick = onRetry) {
                    Text(stringResource(R.string.contributors_retry))
                }
            }

            else -> LazyColumn(
                modifier = Modifier.fillMaxSize().padding(padding),
                contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                state.issued?.let { issued ->
                    item(key = "issued") {
                        IssuedCodeCard(
                            code = issued.code,
                            onCopyCode = onCopyCode,
                            onDismiss = onDismissIssued,
                        )
                    }
                }
                item(key = "open") { OpenClaimForm(onOpenClaim = onOpenClaim, working = state.working) }
                if (state.claims.isEmpty()) {
                    item(key = "empty") {
                        Text(
                            text = stringResource(R.string.claim_empty),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                } else {
                    items(state.claims, key = { "claim:${it.id}" }) { claim ->
                        ClaimRow(
                            claim = claim,
                            working = state.working,
                            onSubmitProof = onSubmitProof,
                            onWithdraw = onWithdraw,
                            onCopyCode = onCopyCode,
                        )
                    }
                }
                item(key = "profile") {
                    OutlinedButton(onClick = onEditProfile, modifier = Modifier.fillMaxWidth()) {
                        Text(stringResource(R.string.claim_edit_profile_button))
                    }
                }
            }
        }
    }
}

@Composable
private fun IssuedCodeCard(
    code: String,
    onCopyCode: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val containerColor = MaterialTheme.colorScheme.primaryContainer
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = stringResource(R.string.claim_code_title),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.semantics { heading() },
            )
            Text(text = code, style = MaterialTheme.typography.bodyLarge)
            Text(
                text = stringResource(R.string.claim_code_instructions),
                style = MaterialTheme.typography.bodySmall,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(
                    onClick = { onCopyCode(code) },
                    modifier = Modifier.testTag(CLAIM_COPY_BUTTON_TAG),
                ) {
                    Text(stringResource(R.string.claim_copy_button))
                }
                TextButton(onClick = onDismiss) {
                    Text(stringResource(R.string.claim_dismiss_button))
                }
            }
        }
    }
}

@Composable
private fun OpenClaimForm(
    onOpenClaim: (String) -> Unit,
    working: Boolean,
) {
    var login by remember { mutableStateOf("") }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        OutlinedTextField(
            value = login,
            onValueChange = { login = it },
            label = { Text(stringResource(R.string.claim_login_label)) },
            singleLine = true,
            modifier = Modifier
                .fillMaxWidth()
                .testTag(CLAIM_LOGIN_FIELD_TAG),
        )
        Button(
            onClick = { onOpenClaim(login) },
            enabled = !working,
            modifier = Modifier.testTag(CLAIM_OPEN_BUTTON_TAG),
        ) {
            Text(stringResource(R.string.claim_open_button))
        }
    }
}

@Composable
private fun ClaimRow(
    claim: Claim,
    working: Boolean,
    onSubmitProof: (claimId: String, gistId: String) -> Unit,
    onWithdraw: (claimId: String) -> Unit,
    onCopyCode: (String) -> Unit,
) {
    var gistId by remember(claim.id) { mutableStateOf("") }
    val containerColor = MaterialTheme.colorScheme.surface
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = claim.githubLogin,
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = stringResource(
                    when (claim.state) {
                        ClaimState.PENDING -> R.string.claim_state_pending
                        ClaimState.APPROVED -> R.string.claim_state_approved
                        ClaimState.REJECTED -> R.string.claim_state_rejected
                        ClaimState.WITHDRAWN -> R.string.claim_state_withdrawn
                        ClaimState.EXPIRED -> R.string.claim_state_expired
                        ClaimState.UNKNOWN -> R.string.claim_state_expired
                    },
                ),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            val code = claim.code
            if (claim.isOpen && code != null) {
                Text(text = code, style = MaterialTheme.typography.bodyLarge)
                Text(
                    text = stringResource(R.string.claim_code_instructions),
                    style = MaterialTheme.typography.bodySmall,
                )
                TextButton(
                    onClick = { onCopyCode(code) },
                    modifier = Modifier.testTag(CLAIM_ROW_COPY_BUTTON_TAG),
                ) {
                    Text(stringResource(R.string.claim_copy_button))
                }
            }
            if (claim.isOpen) {
                OutlinedTextField(
                    value = gistId,
                    onValueChange = { gistId = it },
                    label = { Text(stringResource(R.string.claim_gist_label)) },
                    singleLine = true,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag(CLAIM_GIST_FIELD_TAG),
                )
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(
                        onClick = { onSubmitProof(claim.id, gistId) },
                        enabled = !working,
                        modifier = Modifier.testTag(CLAIM_PROVE_BUTTON_TAG),
                    ) {
                        Text(stringResource(R.string.claim_prove_button))
                    }
                    OutlinedButton(
                        onClick = { onWithdraw(claim.id) },
                        enabled = !working,
                        modifier = Modifier.testTag(CLAIM_WITHDRAW_BUTTON_TAG),
                    ) {
                        Text(stringResource(R.string.claim_withdraw_button))
                    }
                }
            }
        }
    }
}

const val CLAIM_LOGIN_FIELD_TAG = "claim-login-field"
const val CLAIM_OPEN_BUTTON_TAG = "claim-open-button"
const val CLAIM_GIST_FIELD_TAG = "claim-gist-field"
const val CLAIM_PROVE_BUTTON_TAG = "claim-prove-button"
const val CLAIM_WITHDRAW_BUTTON_TAG = "claim-withdraw-button"
const val CLAIM_COPY_BUTTON_TAG = "claim-copy-button"
const val CLAIM_ROW_COPY_BUTTON_TAG = "claim-row-copy-button"
