package com.anpfuel.app.ui.quality

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.quality.ReadQuality
import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.QualityGateway
import com.anpfuel.domain.valueobject.FuelProduct
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class QualityUiState(
    val fuel: FuelProduct = FuelProduct.GASOLINE_REGULAR,
    val selection: QualitySelection? = null,
    val snapshot: QualitySnapshot? = null,
    val loading: Boolean = false,
    val unavailable: Boolean = false,
    val stale: Boolean = false,
    val revisions: List<QualityRevision> = emptyList(),
    val catalogCursor: String? = null,
    val catalogLoading: Boolean = false,
    val catalogFailed: Boolean = false,
    val sample: QualitySample? = null,
    val provenance: QualityProvenance? = null,
    val detailLoading: Boolean = false,
    val detailFailed: Boolean = false,
)

@HiltViewModel
class QualityViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val read: ReadQuality,
    private val gateway: QualityGateway,
) : ViewModel() {
    private val initialFuel = runCatching { FuelProduct.valueOf(savedStateHandle["fuelProduct"] ?: "GASOLINE_REGULAR") }.getOrNull()
    private val stationId: String = savedStateHandle.get<String>("stationId").orEmpty()
    private val initial = initialFuel?.takeIf {com.anpfuel.app.BuildConfig.QUALITY_UI_ENABLED}?.let { runCatching { QualitySelection(stationId,WireFuelMapper.toWire(it)) }.getOrNull() }
    private val mutable=MutableStateFlow(QualityUiState(fuel=initialFuel ?: FuelProduct.GASOLINE_REGULAR,selection=initial,unavailable=initial==null))
    val state=mutable.asStateFlow()
    private var generation=0L
    private var catalogGeneration=0L
    private var detailGeneration=0L
    private var readJob:Job?=null
    private var detailJob:Job?=null
    private var catalogJob:Job?=null

    init { if(initial!=null) { refresh(); loadRevisions() } }

    fun refresh() { mutable.value.selection?.let { load(it,null,false) } }
    fun selectFuel(fuel:FuelProduct) {
        if(!com.anpfuel.app.BuildConfig.QUALITY_UI_ENABLED)return
        val selection=runCatching { QualitySelection(stationId,WireFuelMapper.toWire(fuel),mutable.value.selection?.revisionId) }.getOrNull() ?: return
        mutable.update {it.copy(fuel=fuel,selection=selection,snapshot=null,sample=null,provenance=null)}
        load(selection,null,true)
    }
    fun selectRevision(id:String?) {
        if(id!=null && mutable.value.revisions.none {it.id==id && it.selectable}) return
        val selection=mutable.value.selection?.copy(revisionId=id) ?: return
        mutable.update {it.copy(selection=selection,snapshot=null,sample=null,provenance=null)}
        load(selection,null,true)
    }
    fun nextPage() {
        val current=mutable.value
        if(current.loading) return
        val revision=current.snapshot?.summary?.lease?.revisionId ?: return
        val cursor=current.snapshot.assays.nextCursor ?: return
        val selection=current.selection?.copy(revisionId=revision) ?: return
        load(selection,cursor,false)
    }

    private fun load(selection:QualitySelection,cursor:String?,reset:Boolean) {
        val own=++generation
        detailGeneration++
        readJob?.cancel(); detailJob?.cancel()
        val previous=if(cursor!=null) mutable.value.snapshot?.assays?.items?.lastOrNull() else null
        mutable.update {it.copy(loading=true,unavailable=false,snapshot=if(reset)null else it.snapshot,stale=false,sample=null,provenance=null,detailLoading=false,detailFailed=false)}
        readJob=viewModelScope.launch {
            try {
                val result=read(selection,cursor)
                if(previous!=null && result.assays.items.isNotEmpty()) {
                    val next=result.assays.items.first()
                    check(next.observedOn+next.id>previous.observedOn+previous.id)
                }
                if(generation==own) mutable.update {it.copy(loading=false,snapshot=result,unavailable=false,stale=result.summary.lease.stale)}
            } catch(e:CancellationException) {throw e}
            catch(e:Exception) {
                if(generation==own) mutable.update {
                    val retained=if(e is QualityReadUnavailable && e.disabled)null else it.snapshot
                    it.copy(loading=false,unavailable=true,stale=retained!=null,snapshot=retained)
                }
            }
        }
    }

    fun loadRevisions(next:Boolean=false) {
        if(!com.anpfuel.app.BuildConfig.QUALITY_UI_ENABLED)return
        val cursor=if(next)mutable.value.catalogCursor ?: return else null
        val own=++catalogGeneration
        catalogJob?.cancel()
        mutable.update {it.copy(catalogLoading=true,catalogFailed=false)}
        catalogJob=viewModelScope.launch {
            try {
                val page=gateway.revisions(cursor)
                if(catalogGeneration==own) mutable.update {it.copy(revisions=page.items,catalogCursor=page.nextCursor,catalogLoading=false)}
            }catch(e:CancellationException){throw e}
            catch(_:Exception){if(catalogGeneration==own)mutable.update {it.copy(catalogLoading=false,catalogFailed=true)}}
        }
    }

    fun openSample(ref:String) {
        val snapshot=mutable.value.snapshot ?: return
        if(snapshot.assays.items.none {it.sampleRef==ref}) return
        val revision=snapshot.summary.lease.revisionId ?: return
        val fuel=mutable.value.selection?.fuel
        detail(run={ gateway.sample(ref,revision).also {result ->
            check(result.sampleRef==ref && result.lease.revisionId==revision && result.assays.all {it.fuel==fuel})
        }},accept={result -> mutable.update {it.copy(sample=result)}})
    }
    fun openSources() {
        val revision=mutable.value.snapshot?.summary?.lease?.revisionId ?: return
        detail(run={gateway.provenance(revision).also {check(it.lease.revisionId==revision)}},accept={result -> mutable.update {it.copy(provenance=result)}})
    }
    private fun <T> detail(run:suspend ()->T,accept:(T)->Unit) {
        val own=generation
        val ticket=++detailGeneration
        detailJob?.cancel()
        mutable.update {it.copy(detailLoading=true,detailFailed=false,sample=null,provenance=null)}
        detailJob=viewModelScope.launch {
            try {val result=run(); if(generation==own && detailGeneration==ticket){accept(result);mutable.update {it.copy(detailLoading=false)}}}
            catch(e:CancellationException){throw e}
            catch(_:Exception){if(generation==own && detailGeneration==ticket)mutable.update {it.copy(detailLoading=false,detailFailed=true)}}
        }
    }
    fun dismissDetail(){detailGeneration++;detailJob?.cancel();mutable.update {it.copy(sample=null,provenance=null,detailLoading=false,detailFailed=false)}}
}
