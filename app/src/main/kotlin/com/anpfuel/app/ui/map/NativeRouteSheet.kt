package com.anpfuel.app.ui.map

import android.Manifest
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.locale.AppLocaleHolder
import com.anpfuel.domain.routing.*

@OptIn(ExperimentalMaterial3Api::class,ExperimentalLayoutApi::class)
@Composable
fun NativeRouteSheet(viewModel:NativeRouteViewModel, onDismiss:()->Unit, onDownloads:()->Unit,
    guidance: @Composable (NativeRoutePreview, NativeRouteRequest) -> Unit = { _,_ -> }) {
    val state by viewModel.ui.collectAsStateWithLifecycle()
    val context=LocalContext.current
    var target by remember { mutableStateOf(RouteInputSlot.DESTINATION) }
    var query by remember { mutableStateOf("") }
    var latitude by remember { mutableStateOf("") }
    var longitude by remember { mutableStateOf("") }
    var cancelFix by remember { mutableStateOf<(() -> Unit)?>(null) }
    var fixing by remember { mutableStateOf(false) }
    var fixGeneration by remember { mutableLongStateOf(0L) }
    fun cancelPosition() { ++fixGeneration;cancelFix?.invoke();cancelFix=null;fixing=false }
    val currentLabel=stringResource(R.string.map22_current)
    val centerLabel=stringResource(R.string.map22_center)
    val upLabel=stringResource(R.string.map22_move_up)
    val downLabel=stringResource(R.string.map22_move_down)
    val manualLabel=stringResource(R.string.map22_coordinates)
    fun fix() {
        cancelPosition();fixing=true
        val token=fixGeneration
        val selectedTarget=target
        cancelFix=requestRouteFix(context,currentLabel) { point ->
            if(token!=fixGeneration) return@requestRouteFix
            fixing=false
            if(point!=null) viewModel.point(selectedTarget,point) else viewModel.fail("no-fix")
        }
    }
    val permission=rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { grants ->
        if(grants.values.any { it }) fix() else viewModel.fail("no-permission")
    }
    DisposableEffect(viewModel) { onDispose { cancelPosition();viewModel.stopSearch() } }
    ModalBottomSheet(onDismissRequest={if(state.building) viewModel.cancel(); onDismiss()}) {
        Column(Modifier.fillMaxWidth().heightIn(max=620.dp).verticalScroll(rememberScrollState()).padding(16.dp),verticalArrangement=Arrangement.spacedBy(10.dp)) {
            Text(stringResource(R.string.map22_title),style=MaterialTheme.typography.titleLarge)
            Text(stringResource(R.string.map22_approximate),style=MaterialTheme.typography.bodySmall)
            Text("${stringResource(R.string.map22_origin)}: ${state.origin?.label ?: stringResource(R.string.map22_unselected)}")
            state.stops.forEachIndexed { index, stop ->
                Row(horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                    Text("${index+1} · ${stop.label}",Modifier.weight(1f))
                    TextButton(onClick={viewModel.moveStop(index,-1)},enabled=index>0,modifier=Modifier.semantics {contentDescription=upLabel}) { Text("↑") }
                    TextButton(onClick={viewModel.moveStop(index,1)},enabled=index<state.stops.lastIndex,modifier=Modifier.semantics {contentDescription=downLabel}) { Text("↓") }
                    TextButton(onClick={viewModel.removeStop(index)}) { Text(stringResource(R.string.action_cancel)) }
                }
            }
            Text("${stringResource(R.string.map22_destination)}: ${state.destination?.label ?: stringResource(R.string.map22_unselected)}")
            FlowRow(horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                RouteInputSlot.entries.forEach { slot -> FilterChip(selected=target==slot,onClick={cancelPosition();target=slot},label={Text(stringResource(when(slot){
                    RouteInputSlot.ORIGIN->R.string.map22_origin;RouteInputSlot.DESTINATION->R.string.map22_destination;RouteInputSlot.STOP->R.string.map22_stop
                }))}) }
            }
            Row {
                TextButton(onClick={cancelPosition();viewModel.mapPoint(target,centerLabel)}) { Text(centerLabel) }
                TextButton(onClick={permission.launch(arrayOf(Manifest.permission.ACCESS_FINE_LOCATION,Manifest.permission.ACCESS_COARSE_LOCATION))},enabled=!fixing) { Text(currentLabel) }
            }
            if(fixing) LinearProgressIndicator(Modifier.fillMaxWidth())
            OutlinedTextField(query,{query=it},label={Text(stringResource(R.string.map22_search_hint))},modifier=Modifier.fillMaxWidth())
            Button(onClick={viewModel.search(context,query)},enabled=!state.searching) {Text(stringResource(R.string.map_search))}
            if(state.searching) LinearProgressIndicator(Modifier.fillMaxWidth())
            state.results.forEach { result -> TextButton(onClick={cancelPosition();viewModel.point(target,RoutePoint(result.latitude,result.longitude,result.name))}) {Text(result.name)} }
            Row(horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(latitude,{latitude=it},label={Text(stringResource(R.string.map22_latitude))},modifier=Modifier.weight(1f))
                OutlinedTextField(longitude,{longitude=it},label={Text(stringResource(R.string.map22_longitude))},modifier=Modifier.weight(1f))
            }
            TextButton(onClick={cancelPosition();viewModel.manual(target,latitude,longitude,manualLabel)}) { Text(manualLabel) }
            Text(stringResource(R.string.map22_avoid))
            FlowRow(horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                RoadAvoidance.entries.forEach { option -> FilterChip(selected=option in state.avoid,onClick={viewModel.avoidance(option)},label={Text(stringResource(when(option){
                    RoadAvoidance.TOLL->R.string.map22_tolls;RoadAvoidance.MOTORWAY->R.string.map22_motorways;RoadAvoidance.FERRY->R.string.map22_ferries;RoadAvoidance.UNPAVED->R.string.map22_unpaved
                }))}) }
            }
            state.error?.let {
                Text(stringResource(if(it=="no-fix"||it=="no-permission") R.string.map22_fix_error else R.string.map22_error),color=MaterialTheme.colorScheme.error,modifier=Modifier.testTag("native-route-error"))
                if(state.missingMaps.isNotEmpty()) Text(state.missingMaps.joinToString("\n"))
                Button(onClick=onDownloads) { Text(stringResource(R.string.map_downloads)) }
            }
            if(state.building) LinearProgressIndicator(Modifier.fillMaxWidth())
            Row(horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                Button(onClick={viewModel.build(java.util.Locale.forLanguageTag(AppLocaleHolder.localeTag).language)},enabled=!state.building,modifier=Modifier.testTag("native-route-build")) {Text(stringResource(R.string.map22_preview))}
                OutlinedButton(onClick=viewModel::cancel) {Text(stringResource(R.string.map_cancel))}
            }
            state.preview?.let { preview ->
                Text(stringResource(R.string.map22_estimate,java.text.NumberFormat.getNumberInstance().format(preview.distanceMeters),preview.seconds),modifier=Modifier.testTag("native-route-preview"))
                if(preview.warnings) Text(stringResource(R.string.map22_warnings))
                preview.maneuvers.forEach { step -> Text(step.instruction) }
                Button(onClick=onDismiss) {Text(stringResource(R.string.map22_show))}
                viewModel.request(java.util.Locale.forLanguageTag(AppLocaleHolder.localeTag).language)?.let { guidance(preview,it) }
            }
        }
    }
}
