package com.anpfuel.app.ui.quality

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.domain.quality.QualityAssay
import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale

@OptIn(ExperimentalMaterial3Api::class,ExperimentalLayoutApi::class)
@Composable
fun QualityScreen(onBack:()->Unit,viewModel:QualityViewModel=hiltViewModel(),onReports:(String,FuelProduct,String?,String?)->Unit={_,_,_,_->}) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val locale=LocalConfiguration.current.locales[0]
    val uri=LocalUriHandler.current
    var showPeriods by rememberSaveable {mutableStateOf(false)}
    val list=rememberLazyListState()
    val snapshot=state.snapshot
    LaunchedEffect(snapshot?.assays?.items?.firstOrNull()?.id,state.fuel,snapshot?.summary?.lease?.revisionId) {list.scrollToItem(0)}
    AnpScaffold(topBar={AnpTopAppBar(title={Text(stringResource(R.string.quality_title))},onNavigateUp=onBack)}) {padding ->
        LazyColumn(state=list,modifier=Modifier.fillMaxSize().padding(padding),contentPadding=PaddingValues(20.dp),verticalArrangement=Arrangement.spacedBy(16.dp)) {
            item {
                Text(stringResource(R.string.quality_note),style=MaterialTheme.typography.bodyMedium)
                state.selection?.let {selection ->
                    OutlinedButton(onClick={onReports(selection.stationId,state.fuel,null,null)}){Text(stringResource(R.string.quality_reports_entry))}
                }
                if(state.loading || state.detailLoading) LinearProgressIndicator(Modifier.fillMaxWidth())
                if(state.unavailable) {
                    Text(stringResource(R.string.quality_unavailable))
                    OutlinedButton(onClick=viewModel::refresh,enabled=!state.loading){Text(stringResource(R.string.action_retry))}
                }
                if(state.stale || snapshot?.summary?.lease?.stale==true) Text(stringResource(R.string.quality_stale),color=MaterialTheme.colorScheme.error)
                if(state.detailFailed) Text(stringResource(R.string.quality_source_unavailable))
            }
            item {
                FlowRow(horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                    FuelProduct.entries.forEach {fuel -> FilterChip(selected=state.fuel==fuel,onClick={viewModel.selectFuel(fuel)},label={Text(stringResource(FuelProductI18n.toStringRes(fuel)))})}
                }
            }
            item {
                Text(stringResource(R.string.quality_score_unavailable),style=MaterialTheme.typography.bodyMedium)
                snapshot?.summary?.let {summary ->
                    val lease=summary.lease
                    if(!lease.available) Text(stringResource(R.string.quality_no_source))
                    else {
                        Text(stringResource(R.string.quality_edition,lease.edition,formatQualityDate(lease.publishedAt.orEmpty(),locale)))
                        if(!lease.current) Text(stringResource(R.string.quality_historical))
                        if(!summary.dimensionsAvailable) Text(stringResource(R.string.quality_pending_dimensions))
                        summary.dimensions.forEach {dim ->
                            Text(stringResource(R.string.quality_samples,dim.samples,dim.visits),style=MaterialTheme.typography.titleMedium)
                            Text(stringResource(R.string.quality_verdicts,dim.conforming,dim.nonconforming,dim.unresolved))
                            Text(stringResource(R.string.quality_sample_range,formatQualityDate(dim.oldestObserved,locale),formatQualityDate(dim.newestObserved,locale)))
                        }
                        summary.cells.forEach {cell ->
                            Text(stringResource(when(cell.conformity){"CONFORMING"->R.string.quality_conforming;"NONCONFORMING"->R.string.quality_nonconforming;else->R.string.quality_unknown}))
                            Text(stringResource(R.string.quality_assay_count,cell.count,formatQualityDate(cell.lastObserved,locale)))
                        }
                        TextButton(onClick=viewModel::openSources,enabled=!state.detailLoading){Text(stringResource(R.string.quality_source))}
                    }
                }
                Text(stringResource(R.string.quality_aspect_note),style=MaterialTheme.typography.bodySmall)
            }
            item {
                OutlinedButton(onClick={showPeriods=!showPeriods},modifier=Modifier.fillMaxWidth()){Text(stringResource(R.string.quality_periods))}
                if(showPeriods) {
                    TextButton(onClick={viewModel.selectRevision(null);showPeriods=false},enabled=!state.loading){Text(stringResource(R.string.quality_current))}
                    if(state.catalogFailed) {
                        Text(stringResource(R.string.quality_revisions_unavailable))
                        TextButton(onClick={viewModel.loadRevisions()}){Text(stringResource(R.string.action_retry))}
                    }
                    if(state.catalogLoading) LinearProgressIndicator(Modifier.fillMaxWidth())
                    state.revisions.forEach {revision ->
                        OutlinedButton(onClick={viewModel.selectRevision(revision.id);showPeriods=false},enabled=revision.selectable && !state.loading,modifier=Modifier.fillMaxWidth()) {
                            Text(revision.edition+if(revision.current)" • "+stringResource(R.string.quality_current) else "")
                        }
                    }
                    if(state.catalogCursor!=null) TextButton(onClick={viewModel.loadRevisions(true)},enabled=!state.catalogLoading){Text(stringResource(R.string.quality_more_periods))}
                }
            }
            if(snapshot!=null && snapshot.summary.lease.available) {
                if(snapshot.assays.items.isEmpty()) item {Text(stringResource(R.string.quality_empty))}
                items(snapshot.assays.items,key={it.id}) {row ->
                    OutlinedCard(onClick={viewModel.openSample(row.sampleRef)},enabled=!state.detailLoading,modifier=Modifier.fillMaxWidth()) {
                        Column(Modifier.padding(16.dp),verticalArrangement=Arrangement.spacedBy(6.dp)) {QualityAssayText(row,locale)}
                    }
                }
                item {
                    if(snapshot.assays.nextCursor!=null) Button(onClick=viewModel::nextPage,enabled=!state.loading,modifier=Modifier.fillMaxWidth()){Text(stringResource(R.string.quality_more_assays))}
                    OutlinedButton(onClick=viewModel::refresh,enabled=!state.loading,modifier=Modifier.fillMaxWidth()){Text(stringResource(R.string.quality_first_page))}
                }
            }
        }
    }
    state.sample?.let {sample ->
        AlertDialog(onDismissRequest=viewModel::dismissDetail,title={Text(stringResource(R.string.quality_sample_title))},text={
            LazyColumn(Modifier.heightIn(max=440.dp),verticalArrangement=Arrangement.spacedBy(16.dp)) {
                item {
                    if(sample.lease.stale)Text(stringResource(R.string.quality_stale))
                    Text(stringResource(R.string.quality_aspect_note))
                    state.selection?.let {selection ->
                        TextButton(onClick={onReports(selection.stationId,state.fuel,sample.lease.revisionId,sample.sampleRef)}){Text(stringResource(R.string.quality_reports_link))}
                    }
                }
                items(sample.assays,key={it.id}) {row -> QualityAssayText(row,locale)}
            }
        },confirmButton={TextButton(onClick=viewModel::dismissDetail){Text(stringResource(R.string.action_back))}})
    }
    state.provenance?.let {provenance ->
        AlertDialog(onDismissRequest=viewModel::dismissDetail,title={Text(stringResource(R.string.quality_source))},text={
            LazyColumn(Modifier.heightIn(max=440.dp),verticalArrangement=Arrangement.spacedBy(16.dp)) {
                if(provenance.lease.stale)item {Text(stringResource(R.string.quality_stale))}
                items(provenance.resources) {resource ->
                    Column(verticalArrangement=Arrangement.spacedBy(8.dp)) {
                        if(resource.member.isNotEmpty()) Text(stringResource(R.string.quality_provenance,resource.member))
                        Text(stringResource(R.string.quality_hash,resource.sha256),style=MaterialTheme.typography.bodySmall)
                        TextButton(onClick={runCatching {uri.openUri(resource.url)}}){Text(stringResource(R.string.quality_source))}
                    }
                }
            }
        },confirmButton={TextButton(onClick=viewModel::dismissDetail){Text(stringResource(R.string.action_back))}})
    }
}

@Composable
private fun QualityAssayText(row:QualityAssay,locale:Locale) {
    Text(row.assay,style=MaterialTheme.typography.titleSmall,modifier=Modifier.semantics {heading()})
    Text(stringResource(R.string.quality_observed,formatQualityDate(row.sourceObservedAt ?: row.observedOn,locale)))
    val result=(row.result+" "+row.unit).trim()
    Text(result.ifEmpty {stringResource(R.string.quality_blank_result)})
    Text(stringResource(when(row.conformity){"CONFORMING"->R.string.quality_conforming;"NONCONFORMING"->R.string.quality_nonconforming;else->R.string.quality_unknown}))
}

private fun formatQualityDate(raw:String,locale:Locale):String=runCatching {
    when {
        raw.length==10 -> LocalDate.parse(raw).format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM).withLocale(locale))
        raw.length==19 && raw[10]==' ' -> LocalDateTime.parse(raw,DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss")).format(DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM).withLocale(locale))
        else -> Instant.parse(raw).atZone(ZoneId.systemDefault()).format(DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM).withLocale(locale))
    }
}.getOrDefault(raw)
