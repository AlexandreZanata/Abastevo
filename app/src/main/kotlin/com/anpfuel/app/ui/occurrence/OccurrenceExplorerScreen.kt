package com.anpfuel.app.ui.occurrence

import androidx.compose.foundation.border
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
import com.anpfuel.app.ui.theme.AnpContainerDefaults

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OccurrenceExplorerScreen(
    onBack: () -> Unit,
    onDetail: (String) -> Unit = {},
    viewModel: OccurrenceExplorerViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val list = rememberLazyListState()
    var showSheet by rememberSaveable { mutableStateOf(false) }
    var timeline by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(state.items.firstOrNull()?.id) { if (state.items.isNotEmpty()) list.scrollToItem(0) }
    if (showSheet) {
        ModalBottomSheet(onDismissRequest = { showSheet = false }) {
            OccurrenceFilterSheet(
                filter = state.filter,
                onApply = { filter -> viewModel.selectFilter(filter); showSheet = false },
                onClear = { viewModel.clearFilters(); showSheet = false },
            )
        }
    }
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
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(onClick = { showSheet = true }) { Text(stringResource(R.string.occurrence_filter_type)) }
                    FilterChip(selected = timeline, onClick = { timeline = !timeline }, label = { Text(stringResource(R.string.occurrence_timeline)) })
                    if (!state.filter.isEmpty()) {
                        OutlinedButton(onClick = viewModel::clearFilters) { Text(stringResource(R.string.occurrence_clear)) }
                    }
                }
            }
            if (state.items.isEmpty() && !state.loading && !state.unavailable) {
                item { Text(stringResource(R.string.occurrence_empty)) }
            }
            if (timeline) {
                timelineSections(state.items).forEach { section ->
                    item {
                        Text(
                            stringResource(if (section.companyKind) R.string.occurrence_company_section else R.string.occurrence_global_section),
                            style = MaterialTheme.typography.titleMedium,
                            modifier = Modifier.semantics { heading() },
                        )
                    }
                    items(section.items, key = { it.id }) { record ->
                        OccurrenceCard(record, onDetail)
                    }
                }
            } else {
                items(state.items, key = { it.id }) { record ->
                    OccurrenceCard(record, onDetail)
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

@Composable
private fun OccurrenceCard(record: com.anpfuel.domain.occurrence.OccurrenceRecord, onDetail: (String) -> Unit) {
    ElevatedCard(
        onClick = { onDetail(record.id) },
        modifier = Modifier
            .fillMaxWidth()
            .border(
                border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerLow),
                shape = CardDefaults.shape,
            ),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(stringResource(R.string.occurrence_action_label), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
            Text(record.result, style = MaterialTheme.typography.bodyMedium, maxLines = 2)
            Text(stringResource(R.string.occurrence_document_day, record.documentDay), style = MaterialTheme.typography.bodySmall)
            Text(record.municipality + " · " + record.uf, style = MaterialTheme.typography.bodySmall)
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun OccurrenceFilterSheet(
    filter: OccurrenceFilter,
    onApply: (OccurrenceFilter) -> Unit,
    onClear: () -> Unit,
) {
    var uf by rememberSaveable { mutableStateOf(filter.uf) }
    var segment by rememberSaveable { mutableStateOf(filter.segment) }
    var kind by rememberSaveable { mutableStateOf(filter.subjectKind) }
    Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(stringResource(R.string.occurrence_filter_type), style = MaterialTheme.typography.titleLarge, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.occurrence_filter_uf))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChip(selected = uf == null, onClick = { uf = null }, label = { Text("BR") })
            listOf("SP", "RJ", "DF").forEach { code ->
                FilterChip(selected = uf == code, onClick = { uf = code }, label = { Text(code) })
            }
        }
        Text(stringResource(R.string.occurrence_filter_segment))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChip(selected = segment == null, onClick = { segment = null }, label = { Text(stringResource(R.string.occurrence_all)) })
            listOf("Revenda de Combustíveis", "Revenda de GLP").forEach { option ->
                FilterChip(selected = segment == option, onClick = { segment = option }, label = { Text(option) })
            }
        }
        Text(stringResource(R.string.occurrence_filter_kind))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChip(selected = kind == null, onClick = { kind = null }, label = { Text(stringResource(R.string.occurrence_all)) })
            listOf("company-shaped", "personal-shaped", "other").forEach { option ->
                FilterChip(selected = kind == option, onClick = { kind = option }, label = { Text(option) })
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = onClear, modifier = Modifier.weight(1f)) { Text(stringResource(R.string.occurrence_clear)) }
            Button(onClick = { onApply(OccurrenceFilter(uf = uf, segment = segment, subjectKind = kind, fromDay = filter.fromDay, toDay = filter.toDay)) }, modifier = Modifier.weight(1f)) { Text(stringResource(R.string.occurrence_apply)) }
        }
    }
}
