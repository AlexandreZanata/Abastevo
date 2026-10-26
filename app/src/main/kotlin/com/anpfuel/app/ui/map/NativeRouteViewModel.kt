package com.anpfuel.app.ui.map

import android.content.Context
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.domain.routing.*
import com.anpfuel.maps.engine.MapEngine
import com.anpfuel.maps.engine.MapSearchResult
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

enum class RouteInputSlot { ORIGIN, DESTINATION, STOP }
data class NativeRouteUi(
    val origin: RoutePoint? = null, val destination: RoutePoint? = null,
    val stops: List<RoutePoint> = emptyList(), val avoid: Set<RoadAvoidance> = emptySet(),
    val building: Boolean = false, val preview: NativeRoutePreview? = null,
    val error: String? = null, val missingMaps: List<String> = emptyList(),
    val searching: Boolean = false, val results: List<MapSearchResult> = emptyList(),
)

@HiltViewModel
class NativeRouteViewModel @Inject constructor(private val engine: MapEngine, private val saved: SavedStateHandle): ViewModel() {
    private val state = MutableStateFlow(NativeRouteUi())
    val ui = state.asStateFlow()
    private var generation = 0L
    private var searchGeneration = 0L
    private var deadline: Job? = null
    private var searchDeadline: Job? = null
    init {
        val coordinates = saved.get<ArrayList<String>>("route-coordinates").orEmpty()
        val labels = saved.get<ArrayList<String>>("route-labels").orEmpty()
        val points = coordinates.mapIndexedNotNull { index, raw -> runCatching {
            val parts=raw.split(','); RoutePoint(parts[0].toDouble(),parts[1].toDouble(),labels.getOrElse(index){""})
        }.getOrNull() }
        val rawAvoid = saved.get<ArrayList<String>>("route-avoid").orEmpty()
        val avoid = rawAvoid.mapNotNull { raw -> RoadAvoidance.entries.firstOrNull { it.name == raw } }
        if (points.size >= 2 && avoid.size == rawAvoid.size) runCatching { NativeRouteRequest(points).validate() }.onSuccess {
            state.value=NativeRouteUi(origin=points.first(),destination=points.last(),stops=points.drop(1).dropLast(1),avoid=avoid.toSet())
        }
    }
    fun point(slot:RouteInputSlot, point:RoutePoint) {
        invalidate()
        state.update { when(slot) {
            RouteInputSlot.ORIGIN -> it.copy(origin=point)
            RouteInputSlot.DESTINATION -> it.copy(destination=point)
            RouteInputSlot.STOP -> if (it.stops.size < 100) it.copy(stops=it.stops+point) else it.copy(error="too-many-stops")
        }}
        persist()
    }
    fun mapPoint(slot:RouteInputSlot, label:String) {
        val center=engine.currentCenter() ?: return fail("not-ready")
        point(slot,RoutePoint(center.latitude,center.longitude,label))
    }
    fun manual(slot:RouteInputSlot, latitude:String, longitude:String, label:String) {
        val p=runCatching { RoutePoint(latitude.trim().toDouble(),longitude.trim().toDouble(),label).also {
            require(it.latitude.isFinite() && it.longitude.isFinite() && it.latitude in -90.0..90.0 && it.longitude in -180.0..180.0)
        }}.getOrNull() ?: return fail("invalid")
        point(slot,p)
    }
    fun removeStop(index:Int) { invalidate(); state.update { it.copy(stops=it.stops.filterIndexed { i,_ -> i != index }) };persist() }
    fun moveStop(index:Int, offset:Int) {
        val list=state.value.stops.toMutableList();val other=index+offset
        if(index !in list.indices || other !in list.indices) return
        invalidate();val item=list.removeAt(index);list.add(other,item);state.update { it.copy(stops=list) };persist()
    }
    fun avoidance(option:RoadAvoidance) { invalidate();state.update { it.copy(avoid=if(option in it.avoid) it.avoid-option else it.avoid+option) };persist() }
    fun search(context:Context, query:String) {
        stopSearch(); val token=searchGeneration
        if(query.trim().length !in 2..256) { state.update { it.copy(results=emptyList(),searching=false) };return }
        state.update { it.copy(searching=true,results=emptyList()) }
        searchDeadline=viewModelScope.launch { delay(30_000); if(token==searchGeneration && state.value.searching) {stopSearch();fail("timeout")} }
        if (!engine.search(context,query.trim()) { results -> if(token==searchGeneration) {searchDeadline?.cancel();state.update { it.copy(searching=false,results=results.take(50)) }} }) {
            searchDeadline?.cancel();state.update { it.copy(searching=false,error="not-ready") }
        }
    }
    fun request(language:String):NativeRouteRequest? {
        val current=state.value
        return runCatching { NativeRouteRequest(listOf(checkNotNull(current.origin))+current.stops+checkNotNull(current.destination),current.avoid,language).also { it.validate() } }.getOrNull()
    }
    fun build(language:String) {
        invalidate()
        val request=request(language) ?: return fail("invalid")
        val token=generation
        state.update { it.copy(building=true) }
        deadline=viewModelScope.launch { delay(90_000);if(token==generation && state.value.building) { invalidate(); fail("timeout") } }
        val accepted=engine.buildRoute(request) { result ->
            if(token!=generation) return@buildRoute
            deadline?.cancel()
            when(result) {
                is NativeRouteResult.Ready -> state.update { it.copy(building=false,preview=result.preview,error=null,missingMaps=emptyList()) }
                is NativeRouteResult.Failed -> state.update { it.copy(building=false,preview=null,error="native-${result.code}",missingMaps=result.missingMaps) }
            }
        }
        if(!accepted) { deadline?.cancel(); state.update { it.copy(building=false,error="not-ready") } }
    }
    fun restorePlan(points: List<RoutePoint>, avoid: Set<RoadAvoidance>) {
        val cleaned = points.distinctBy { it.latitude to it.longitude }
        if (cleaned.size < 2) return
        invalidate()
        state.value = NativeRouteUi(
            origin = cleaned.first(),
            destination = cleaned.last(),
            stops = cleaned.drop(1).dropLast(1),
            avoid = avoid,
        )
        persist()
    }
    fun fail(reason:String) { state.update { it.copy(error=reason) } }
    fun stopSearch() { ++searchGeneration;searchDeadline?.cancel();engine.cancelSearch();state.update { it.copy(searching=false,results=emptyList()) } }
    fun cancel() { invalidate();stopSearch() }
    private fun invalidate() { ++generation;deadline?.cancel();engine.cancelRoute();state.update { it.copy(building=false,preview=null,error=null,missingMaps=emptyList()) } }
    private fun persist() {
        val s=state.value;val points=if(s.origin!=null && s.destination!=null) listOf(s.origin)+s.stops+s.destination else emptyList()
        saved["route-avoid"]=ArrayList(s.avoid.map { it.name })
        saved["route-coordinates"]=ArrayList(points.map { "${it.latitude},${it.longitude}" })
        saved["route-labels"]=ArrayList(points.map { it.label })
    }
    override fun onCleared() { cancel();super.onCleared() }
}
