package com.anpfuel.app.ui.search

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.AppErrorMapper
import com.anpfuel.app.mapper.DataAvailabilityI18n
import com.anpfuel.app.ui.components.AnpAttributionFooter
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.model.MunicipalitySearchResult

@Composable
fun SearchScreen(
    onNavigateBack: (() -> Unit)? = null,
    onNavigateToHome: () -> Unit,
    onNavigateToLocationPicker: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: SearchViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    LaunchedEffect(viewModel) {
        viewModel.navigation.collect { destination ->
            when (destination) {
                SearchNavigation.ToHome -> onNavigateToHome()
                SearchNavigation.ToLocationPicker -> onNavigateToLocationPicker()
            }
        }
    }

    SearchContent(
        uiState = uiState,
        onNavigateBack = onNavigateBack,
        onQueryChange = viewModel::onQueryChange,
        onResultSelected = viewModel::onResultSelected,
        onBrowseByState = viewModel::onBrowseByState,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun SearchContent(
    uiState: SearchUiState,
    onNavigateBack: (() -> Unit)? = null,
    onQueryChange: (String) -> Unit,
    onResultSelected: (MunicipalitySearchResult) -> Unit,
    onBrowseByState: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                onNavigateUp = onNavigateBack,
                title = { Text(text = stringResource(R.string.nav_search)) },
            )
        },
        bottomBar = { AnpAttributionFooter() },
    ) { innerPadding ->
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding),
        ) {
            Column(modifier = Modifier.fillMaxSize()) {
                OutlinedTextField(
                    value = uiState.query,
                    onValueChange = onQueryChange,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 8.dp),
                    label = { Text(text = stringResource(R.string.search_municipality_hint)) },
                    singleLine = true,
                    enabled = !uiState.isSaving,
                )

                if (uiState.showMinCharsHint) {
                    Text(
                        text = stringResource(R.string.search_min_chars_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                    )
                }

                when {
                    uiState.errorMessage != null -> {
                        ErrorState(
                            message = uiState.errorMessage,
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }

                    uiState.isSearching -> {
                        LoadingState(modifier = Modifier.fillMaxWidth())
                    }

                    uiState.showNoResults -> {
                        Column(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalAlignment = Alignment.CenterHorizontally,
                        ) {
                            EmptyState(
                                message = stringResource(
                                    AppErrorMapper.toStringRes(
                                        uiState.error ?: com.anpfuel.application.error.AppError.SearchNoResults,
                                    ),
                                ),
                                modifier = Modifier.fillMaxWidth(),
                            )
                            TextButton(onClick = onBrowseByState) {
                                Text(text = stringResource(R.string.search_browse_by_state))
                            }
                        }
                    }

                    uiState.results.isNotEmpty() -> {
                        LazyColumn(modifier = Modifier.fillMaxSize()) {
                            items(
                                items = uiState.results,
                                key = { "${it.state.name}:${it.municipality}" },
                            ) { result ->
                                val subtitleRes = DataAvailabilityI18n.toSearchSubtitleStringRes(result.dataAvailability)
                                ListItem(
                                    headlineContent = { Text(stringResource(R.string.search_result_format, result.municipality, result.state.abbreviation)) },
                                    supportingContent = if (subtitleRes != null) { { Text(stringResource(subtitleRes), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) } } else null,
                                    modifier = Modifier.fillMaxWidth().clickable { onResultSelected(result) },
                                )
                                HorizontalDivider()
                            }
                        }
                    }
                }
            }

            if (uiState.isSaving) {
                LoadingState(modifier = Modifier.align(Alignment.Center))
            }
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun SearchScreenPreview() {
    AnpFuelTheme {
        SearchContent(
            uiState = SearchUiState(
                query = "CU",
                showMinCharsHint = true,
            ),
            onNavigateBack = {},
            onQueryChange = {},
            onResultSelected = {},
            onBrowseByState = {},
        )
    }
}
