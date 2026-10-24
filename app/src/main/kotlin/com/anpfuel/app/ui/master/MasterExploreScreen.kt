package com.anpfuel.app.ui.master

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.OfflineBanner
import com.anpfuel.app.ui.theme.AnpFuelTheme
import java.util.Locale

/** Frozen suggestion set, mirrored from the backend contract (PRM09-T06). */
private val FROZEN_SUGGESTIONS = listOf(
    "Maior preço observado" to "Qual o maior preço observado da gasolina comum no Brasil?",
    "Maior média por estado" to "Qual a maior média de preço do etanol por estado?",
    "Menor preço observado" to "Qual o menor preço observado do diesel s10 no Brasil?",
    "Ações de fiscalização" to "Quais ações de fiscalização da ANP no Distrito Federal?",
    "Análises de qualidade" to "Quais análises de qualidade do combustível em São Paulo?",
    "Infrações informadas" to "Quais infrações informadas pela fonte no Rio de Janeiro?",
)

@Composable
fun MasterExploreScreen(
    onNavigateBack: () -> Unit,
    onOpenOccurrences: () -> Unit,
    onOpenCharging: (String) -> Unit = {},
    onOpenVehicles: () -> Unit = {},
    modifier: Modifier = Modifier,
    viewModel: MasterExploreViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    MasterExploreContent(
        uiState = uiState,
        onQuestionChanged = viewModel::onQuestionChanged,
        onAsk = viewModel::ask,
        onSuggestion = viewModel::askSuggestion,
        onChooseAlternative = viewModel::chooseAlternative,
        onLoadMore = viewModel::loadMore,
        onOpenOccurrences = onOpenOccurrences,
        onOpenCharging = onOpenCharging,
        onOpenVehicles = onOpenVehicles,
        onDismissExecution = viewModel::clearExecutionNotice,
        onDismissError = viewModel::dismissError,
        onNavigateBack = onNavigateBack,
        modifier = modifier,
    )
}

@OptIn(ExperimentalLayoutApi::class, ExperimentalMaterial3Api::class)
@Composable
internal fun MasterExploreContent(
    uiState: MasterExploreUiState,
    onQuestionChanged: (String) -> Unit,
    onAsk: () -> Unit,
    onSuggestion: (String) -> Unit,
    onChooseAlternative: (Int) -> Unit,
    onLoadMore: () -> Unit,
    onOpenOccurrences: () -> Unit,
    onOpenCharging: (String) -> Unit = {},
    onOpenVehicles: () -> Unit = {},
    onDismissExecution: () -> Unit = {},
    onDismissError: () -> Unit,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val locale = LocalConfiguration.current.locales[0]

    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                title = {
                    Text(
                        text = stringResource(R.string.master_explore_title),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .padding(horizontal = 16.dp, vertical = 8.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            if (uiState.isOffline) {
                OfflineBanner()
            }
            OutlinedTextField(
                value = uiState.question.ifEmpty { "" },
                onValueChange = onQuestionChanged,
                modifier = Modifier.fillMaxWidth(),
                label = { Text(text = stringResource(R.string.master_explore_hint)) },
                singleLine = true,
            )
            Button(onClick = onAsk, modifier = Modifier.fillMaxWidth()) {
                Text(text = stringResource(R.string.master_explore_ask))
            }
            Text(
                text = stringResource(R.string.master_explore_suggestions),
                style = MaterialTheme.typography.titleSmall,
            )
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FROZEN_SUGGESTIONS.forEach { (label, question) ->
                    FilterChip(
                        selected = false,
                        onClick = { onSuggestion(question) },
                        label = { Text(text = label) },
                    )
                }
            }
            uiState.interpretation?.let { interpretation ->
                Card(modifier = Modifier.fillMaxWidth()) {
                    Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(
                            text = interpretation.kind,
                            style = MaterialTheme.typography.titleSmall,
                        )
                        interpretation.lines.forEach { line ->
                            Text(text = line, style = MaterialTheme.typography.bodyMedium)
                        }
                        if (interpretation.reason.isNotEmpty()) {
                            Text(
                                text = interpretation.reason,
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.error,
                            )
                        }
                    }
                }
            }
            if (uiState.alternatives.isNotEmpty()) {
                Text(
                    text = stringResource(R.string.master_explore_alternatives),
                    style = MaterialTheme.typography.titleSmall,
                )
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    uiState.alternatives.forEach { alternative ->
                        FilterChip(
                            selected = false,
                            onClick = { onChooseAlternative(alternative.index) },
                            label = { Text(text = alternative.label) },
                        )
                    }
                }
            }
            if (uiState.route == "OFFICIAL") {
                TextButton(onClick = onOpenOccurrences) {
                    Text(text = stringResource(R.string.master_explore_open_official))
                }
            }
            uiState.chargingVehicleId?.let { vehicleId ->
                Button(onClick = { onOpenCharging(vehicleId) }, modifier = Modifier.fillMaxWidth()) {
                    Text(text = stringResource(R.string.master_explore_open_charging))
                }
            }
            if (uiState.chargingVehicleChoices.isNotEmpty()) {
                Text(
                    text = stringResource(R.string.master_explore_choose_vehicle),
                    style = MaterialTheme.typography.titleSmall,
                )
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    uiState.chargingVehicleChoices.forEach { choice ->
                        FilterChip(
                            selected = false,
                            onClick = { onOpenCharging(choice.id) },
                            label = { Text(text = choice.name) },
                        )
                    }
                }
            }
            if (uiState.needsElectricVehicle) {
                Card(modifier = Modifier.fillMaxWidth()) {
                    Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text(
                            text = stringResource(R.string.master_explore_add_vehicle_message),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                        TextButton(onClick = onOpenVehicles) {
                            Text(text = stringResource(R.string.master_explore_add_vehicle))
                        }
                    }
                }
            }
            if (uiState.energyExecutionPending) {
                Button(onClick = onDismissExecution, modifier = Modifier.fillMaxWidth()) {
                    Text(text = stringResource(R.string.master_explore_energy_pending))
                }
            }
            if (uiState.scenarioExecutionPending) {
                Button(onClick = onDismissExecution, modifier = Modifier.fillMaxWidth()) {
                    Text(text = stringResource(R.string.master_explore_scenario_pending))
                }
            }
            if (uiState.isLoading) {
                CircularProgressIndicator(modifier = Modifier.align(Alignment.CenterHorizontally))
            }
            if (uiState.rows.isNotEmpty()) {
                Text(
                    text = stringResource(R.string.master_explore_results),
                    style = MaterialTheme.typography.titleSmall,
                )
                uiState.revisionId?.let {
                    Text(
                        text = stringResource(R.string.master_explore_revision, it.take(8)),
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
                uiState.rows.forEach { row ->
                    Card(modifier = Modifier.fillMaxWidth()) {
                        Column(modifier = Modifier.padding(16.dp)) {
                            Text(text = row.title, style = MaterialTheme.typography.titleMedium)
                            Text(text = row.subtitle, style = MaterialTheme.typography.bodySmall)
                        }
                    }
                }
                if (uiState.hasMore) {
                    Button(onClick = onLoadMore, modifier = Modifier.fillMaxWidth()) {
                        Text(text = stringResource(R.string.master_explore_more))
                    }
                }
            } else if (!uiState.isLoading && uiState.interpretation != null) {
                EmptyState(
                    message = stringResource(R.string.master_explore_no_results),
                    modifier = Modifier.fillMaxWidth(),
                )
            }
            uiState.error?.let { message ->
                ErrorState(message = message, modifier = Modifier.fillMaxWidth(), onRetry = onDismissError)
            }
        }
    }
}

@Preview(showBackground = true, name = "Master explore")
@Composable
private fun MasterExplorePreview() {
    AnpFuelTheme {
        MasterExploreContent(
            uiState = MasterExploreUiState(isLoading = false),
            onQuestionChanged = {},
            onAsk = {},
            onSuggestion = {},
            onChooseAlternative = {},
            onLoadMore = {},
            onOpenOccurrences = {},
            onDismissError = {},
            onNavigateBack = {},
        )
    }
}
