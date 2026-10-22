package com.anpfuel.app.ui.occurrence

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
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

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OccurrenceExplorerScreen(
    onBack: () -> Unit,
    onDetail: (String) -> Unit = {},
    viewModel: OccurrenceExplorerViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val list = rememberLazyListState()
    var showFilters by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(state.items.firstOrNull()?.id) { if (state.items.isNotEmpty()) list.scrollToItem(0) }
    AnpScaffold(topBar = { AnpTopAppBar(title = { Text(stringResource(R.string.occurrence_title)) }, onNavigateUp = onBack) }) { padding ->
        LazyColumn(state = list, modifier = Modifier.fillMaxSize().padding(padding), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            item {
                Text(stringResource(R.string.occurrence_no_guilt), style = MaterialTheme.typography.bodyMedium)
                if (state.loading) LinearProgressIndicator(Modifier.fillMaxWidth())
                if (state.unavailable) {
                    Text(stringResource(R.string.occurrence_unavailable))
                    OutlinedButton(onClick = viewModel::refresh, enabled = !state.loading) { Text(stringResource(R.string.occurrence_retry)) }
                }
                if (state.stale) Text(stringResource(R.string.occurrence_stale), color = MaterialTheme.colorScheme.error)
                state.coverage?.let { coverage ->
                    Text(stringResource(R.string.occurrence_coverage, coverage.observations, coverage.cases), style = MaterialTheme.typography.titleMedium)
                }
            }
            item {
                OutlinedButton(onClick = { showFilters = !showFilters }) { Text(stringResource(R.string.occurrence_filter_type)) }
                if (showFilters) {
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        FilterChip(selected = state.filter.uf == null, onClick = { viewModel.selectFilter(state.filter.copy(uf = null)) }, label = { Text("BR") })
                        listOf("SP", "RJ", "DF").forEach { uf ->
                            FilterChip(selected = state.filter.uf == uf, onClick = { viewModel.selectFilter(state.filter.copy(uf = uf)) }, label = { Text(uf) })
                        }
                    }
                }
            }
            if (state.items.isEmpty() && !state.loading && !state.unavailable) {
                item { Text(stringResource(R.string.occurrence_empty)) }
            }
            items(state.items, key = { it.id }) { record ->
                ElevatedCard(onClick = { onDetail(record.id) }, modifier = Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(stringResource(R.string.occurrence_action_label), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
                        Text(record.result, style = MaterialTheme.typography.bodyMedium, maxLines = 2)
                        Text(stringResource(R.string.occurrence_document_day, record.documentDay), style = MaterialTheme.typography.bodySmall)
                        Text(record.municipality + " · " + record.uf, style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
            if (state.cursor != null) {
                item {
                    OutlinedButton(onClick = viewModel::nextPage, enabled = !state.loadingMore, modifier = Modifier.fillMaxWidth()) {
                        if (state.loadingMore) CircularProgressIndicator(Modifier.size(16.dp)) else Text(stringResource(R.string.occurrence_retry))
                    }
                }
            }
        }
    }
}
