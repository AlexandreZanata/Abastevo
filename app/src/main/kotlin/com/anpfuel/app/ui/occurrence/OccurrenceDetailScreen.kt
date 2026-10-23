package com.anpfuel.app.ui.occurrence

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
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
import com.anpfuel.domain.occurrence.OccurrenceRecord
import com.anpfuel.domain.repository.OccurrenceGateway
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class OccurrenceDetailState(
    val record: OccurrenceRecord? = null,
    val loading: Boolean = false,
    val unavailable: Boolean = false,
    val stale: Boolean = false,
)

@HiltViewModel
class OccurrenceDetailViewModel @Inject constructor(
    private val gateway: OccurrenceGateway,
) : ViewModel() {
    private val mutable = MutableStateFlow(OccurrenceDetailState())
    val state = mutable.asStateFlow()
    private var job: Job? = null
    private var generation = 0L
    private var currentId: String? = null

    fun load(id: String) {
        val myGeneration = ++generation
        job?.cancel()
        if (currentId != id) mutable.value = OccurrenceDetailState()
        currentId = id
        job = viewModelScope.launch {
            mutable.update { it.copy(loading = true, unavailable = false) }
            try {
                val (record, lease) = gateway.record(id)
                if (myGeneration != generation) return@launch
                check(record.id == id)
                mutable.update { it.copy(record = record, loading = false, stale = lease.stale) }
            } catch (e: CancellationException) { throw e }
            catch (_: Exception) {
                if (myGeneration != generation) return@launch
                mutable.update { it.copy(loading = false, unavailable = it.record == null, stale = it.record != null) }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OccurrenceDetailScreen(
    recordId: String,
    onBack: () -> Unit,
    viewModel: OccurrenceDetailViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    LaunchedEffect(recordId) { viewModel.load(recordId) }
    AnpScaffold(topBar = { AnpTopAppBar(title = { Text(stringResource(R.string.occurrence_detail_title)) }, onNavigateUp = onBack) }) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (state.loading) LinearProgressIndicator(Modifier.fillMaxWidth())
            if (state.unavailable) {
                Text(stringResource(R.string.occurrence_unavailable))
                return@Column
            }
            state.record?.let { record ->
                Text(stringResource(R.string.occurrence_action_label), style = MaterialTheme.typography.titleLarge, modifier = Modifier.semantics { heading() })
                Text(record.result, style = MaterialTheme.typography.bodyLarge)
                Text(stringResource(R.string.occurrence_document_day, record.documentDay), style = MaterialTheme.typography.bodyMedium)
                Text(record.procedure, style = MaterialTheme.typography.bodyMedium)
                Text(record.municipality + " · " + record.uf, style = MaterialTheme.typography.bodyMedium)
                Text(record.segment, style = MaterialTheme.typography.bodyMedium)
                Text(record.documentNumber, style = MaterialTheme.typography.bodySmall)
                if (state.stale) Text(stringResource(R.string.occurrence_stale), color = MaterialTheme.colorScheme.error)
                Text(stringResource(R.string.occurrence_no_guilt), style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}
