package com.anpfuel.app.ui.suggest

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.application.usecase.intake.SubmitSuggestionOutcome

/**
 * P27-T04 suggest/correct/status journey (free accounts).
 *
 * Structured form with search-before-submit duplicate hint, explicit
 * submit (one stable id across offline retries), private status list
 * with cancel, and honest guest/denial/error states. No background
 * permission is demanded and no photo is required; review stays
 * server-side.
 */
@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun SuggestStationScreen(
    onNavigateBack: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
    viewModel: SuggestStationViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    LaunchedEffect(viewModel) {
        viewModel.refreshMine()
    }

    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                onNavigateUp = onNavigateBack,
                title = { Text(text = stringResource(R.string.suggest_title)) },
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            OutlinedTextField(
                value = uiState.displayName,
                onValueChange = {
                    viewModel.onField(it, uiState.municipalityCode, uiState.state, uiState.cnpj)
                },
                modifier = Modifier.fillMaxWidth(),
                label = { Text(text = stringResource(R.string.suggest_display)) },
                singleLine = true,
            )
            OutlinedTextField(
                value = uiState.municipalityCode,
                onValueChange = {
                    viewModel.onField(uiState.displayName, it, uiState.state, uiState.cnpj)
                },
                modifier = Modifier.fillMaxWidth(),
                label = { Text(text = stringResource(R.string.suggest_municipality)) },
                singleLine = true,
            )
            OutlinedTextField(
                value = uiState.state,
                onValueChange = {
                    viewModel.onField(uiState.displayName, uiState.municipalityCode, it, uiState.cnpj)
                },
                modifier = Modifier.fillMaxWidth(),
                label = { Text(text = stringResource(R.string.suggest_state)) },
                singleLine = true,
            )
            OutlinedTextField(
                value = uiState.cnpj,
                onValueChange = {
                    viewModel.onField(uiState.displayName, uiState.municipalityCode, uiState.state, it)
                },
                modifier = Modifier.fillMaxWidth(),
                label = { Text(text = stringResource(R.string.suggest_cnpj)) },
                singleLine = true,
            )
            uiState.duplicateHint?.let { hint ->
                Text(
                    text = stringResource(R.string.suggest_duplicate_hint, hint),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                )
            }
            when (val submit = uiState.submit) {
                is SubmitSuggestionOutcome.Submitted ->
                    Text(
                        text = stringResource(R.string.suggest_sent),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                is SubmitSuggestionOutcome.Conflict ->
                    Text(
                        text = stringResource(R.string.suggest_conflict),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                is SubmitSuggestionOutcome.QuotaExceeded ->
                    Text(
                        text = stringResource(R.string.suggest_quota),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                is SubmitSuggestionOutcome.SessionInvalid ->
                    Text(
                        text = stringResource(R.string.suggest_signin),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                is SubmitSuggestionOutcome.Invalid ->
                    Text(
                        text = stringResource(R.string.suggest_invalid),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                is SubmitSuggestionOutcome.Unavailable ->
                    Text(
                        text = stringResource(
                            R.string.suggest_unavailable,
                            submit.cause.message ?: submit.cause.javaClass.simpleName,
                        ),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                null -> Unit
            }
            Button(
                onClick = { viewModel.submitProposal() },
                modifier = Modifier.fillMaxWidth(),
                enabled = !uiState.isSending,
            ) {
                Text(text = stringResource(R.string.suggest_submit))
            }
            OutlinedButton(
                onClick = { viewModel.newForm() },
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(text = stringResource(R.string.suggest_new_form))
            }

            if (uiState.ownItems.isNotEmpty()) {
                Text(
                    text = stringResource(R.string.suggest_mine_title),
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.semantics { heading() },
                )
                uiState.ownItems.forEach { item ->
                    Card(
                        modifier = Modifier.fillMaxWidth(),
                        colors = CardDefaults.cardColors(
                            containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
                        ),
                    ) {
                        Column(
                            modifier = Modifier.padding(12.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp),
                        ) {
                            Text(
                                text = item.id.take(8) + " · " + item.state,
                                style = MaterialTheme.typography.bodyMedium,
                            )
                            TextButton(onClick = { viewModel.loadStatus(item.id) }) {
                                Text(text = stringResource(R.string.suggest_refresh_status))
                            }
                            TextButton(onClick = { viewModel.cancelSuggestion(item.id) }) {
                                Text(text = stringResource(R.string.suggest_cancel))
                            }
                        }
                    }
                }
            }
        }
    }
}
