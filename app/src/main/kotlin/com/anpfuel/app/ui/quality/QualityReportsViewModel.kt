package com.anpfuel.app.ui.quality

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.QualityReportGateway
import com.anpfuel.domain.valueobject.FuelProduct
import dagger.hilt.android.lifecycle.HiltViewModel
import java.time.LocalDate
import java.time.ZoneOffset
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class QualityReportsState(
    val owner:String?=null,val visible:Boolean=false,val busy:Boolean=false,
    val reports:List<QualityReport> = emptyList(),val nextAfterId:String?=null,
    val selected:QualityReport?=null,val day:String="",val symptom:String="OTHER",val text:String="",
    val confirmation:QualityReportCommand?=null,val pending:QualityReportCommand?=null,
    val receipt:QualityReportReceipt?=null,val error:QualityReportFailure?=null,
)

/** Private content is memory-only, never written to SavedState or navigation arguments. */
@HiltViewModel
class QualityReportsViewModel @Inject constructor(
    saved:SavedStateHandle,private val gateway:QualityReportGateway,
):ViewModel() {
    val stationId=saved.get<String>("stationId").orEmpty()
    val fuel=runCatching {WireFuelMapper.toWire(FuelProduct.valueOf(saved.get<String>("fuelProduct") ?: "GASOLINE_REGULAR"))}.getOrDefault("")
    private val revision=saved.get<String>("revisionId").orEmpty()
    private val sample=saved.get<String>("sampleRef").orEmpty()
    val linkageReview=revision.isNotEmpty() && sample.isNotEmpty()
    private val valid=qualityReportId(stationId) && fuel in QUALITY_FUELS &&
        (revision.isEmpty()==sample.isEmpty()) && (revision.isEmpty() || qualityReportId(revision)) && sample.toByteArray().size<=128
    private val mutable=MutableStateFlow(QualityReportsState(day=today()))
    val state=mutable.asStateFlow()
    private var generation=0L
    private var job:Job?=null

    fun resume() {
        val ticket=++generation
        job?.cancel()
        mutable.update {it.copy(visible=false,busy=true,confirmation=null)}
        job=viewModelScope.launch {
            try {
                if(!valid || !com.anpfuel.app.BuildConfig.QUALITY_UI_ENABLED)throw QualityReportException(QualityReportFailure.INVALID)
                val owner=gateway.owner()
                if(ticket!=generation)return@launch
                if(mutable.value.owner!=owner)mutable.value=QualityReportsState(owner=owner,day=today(),busy=true)
                mutable.update {it.copy(visible=true,error=null)}
                val page=gateway.list(owner)
                if(current(ticket,owner))mutable.update {it.copy(reports=page.items,nextAfterId=page.nextAfterId,busy=false)}
            }catch(e:CancellationException){throw e}
            catch(e:Exception){if(ticket==generation)failure(e)}
        }
    }
    fun pause() {generation++;job?.cancel();mutable.update {it.copy(visible=false,busy=false,confirmation=null)}}
    private fun owner():String? {
        val owner=mutable.value.owner
        if(owner==null || owner!=gateway.currentOwner()) {clear(QualityReportFailure.AUTH_REQUIRED);return null}
        return owner
    }
    private fun current(ticket:Long,owner:String):Boolean {
        if(ticket!=generation)return false
        if(owner!=gateway.currentOwner()){clear(QualityReportFailure.OWNER_CHANGED);return false}
        return true
    }
    private fun clear(error:QualityReportFailure) {generation++;mutable.value=QualityReportsState(visible=true,day=today(),error=error)}
    private fun failure(e:Exception) {
        val kind=(e as? QualityReportException)?.kind ?: QualityReportFailure.UNAVAILABLE
        if(kind==QualityReportFailure.OWNER_CHANGED || kind==QualityReportFailure.AUTH_REQUIRED)clear(kind)
        else mutable.update {it.copy(busy=false,visible=true,error=kind,confirmation=null,pending=if(kind==QualityReportFailure.UNAVAILABLE)it.pending else null)}
    }
    fun text(value:String) {if(editable())mutable.update {it.copy(text=value.take(2001),error=null)}}
    fun day(value:String) {if(editable())mutable.update {it.copy(day=value.take(10),error=null)}}
    fun symptom(value:String) {if(editable() && value in setOf("STARTING","PERFORMANCE","CONSUMPTION","OTHER"))mutable.update {it.copy(symptom=value,error=null)}}
    private fun editable()=mutable.value.let {it.visible && !it.busy && it.pending==null && it.confirmation==null}
    fun newReport() {if(editable() && owner()!=null)mutable.update {it.copy(selected=null,text="",day=today(),symptom="OTHER",receipt=null,error=null)}}
    fun select(report:QualityReport) {
        if(!editable() || owner()==null || mutable.value.reports.none {it.id==report.id && it.revision==report.revision})return
        mutable.update {it.copy(selected=report,text=report.text,day=report.occurredOn,symptom=report.symptom,receipt=null,error=null)}
    }
    fun request(action:String) {
        if(!editable() || owner()==null)return
        val s=mutable.value
        val report=s.selected
        val id=UUID.randomUUID().toString()
        val command=if(action=="WITHDRAW" || action=="APPEAL") {
            if(report==null)return
            QualityReportCommand(id,report.id,action,report.revision,text=if(action=="APPEAL")s.text else "")
        } else {
            if(action !in setOf("SUBMIT","EDIT") || (action=="EDIT")!=(report!=null))return
            QualityReportCommand(id,report?.id ?: UUID.randomUUID().toString(),action,report?.revision ?: 0,
                report?.kind ?: if(linkageReview)"LINK_REVIEW" else "EXPERIENCE",report?.stationId ?: stationId,report?.fuel ?: fuel,
                s.day,s.symptom,s.text,report?.officialRevisionId ?: revision,report?.sampleRef ?: sample)
        }
        try {command.validate(LocalDate.now(ZoneOffset.UTC))}catch(_:Exception){mutable.update {it.copy(error=QualityReportFailure.INVALID)};return}
        mutable.update {it.copy(confirmation=command,error=null)}
    }
    fun dismissConfirmation(){mutable.update {it.copy(confirmation=null)}}
    fun confirm() {
        val command=mutable.value.confirmation ?: return
        if(mutable.value.busy)return
        mutable.update {it.copy(confirmation=null,pending=command)}
        send(command)
    }
    fun retry() {mutable.value.pending?.let {if(!mutable.value.busy)send(it)}}
    private fun send(command:QualityReportCommand) {
        val owner=owner() ?: return
        val ticket=++generation
        job?.cancel()
        mutable.update {it.copy(busy=true,error=null)}
        job=viewModelScope.launch {
            try {
                val receipt=gateway.execute(owner,command)
                if(!current(ticket,owner))return@launch
                mutable.update {it.copy(pending=null,receipt=receipt,selected=null,text="",day=today(),symptom="OTHER")}
                val page=gateway.list(owner)
                if(current(ticket,owner))mutable.update {it.copy(busy=false,reports=page.items,nextAfterId=page.nextAfterId)}
            }catch(e:CancellationException){throw e}
            catch(e:Exception){if(ticket==generation)failure(e)}
        }
    }
    fun nextPage() {
        val s=mutable.value
        if(s.busy || s.pending!=null || !s.visible)return
        val cursor=s.nextAfterId ?: return
        val owner=owner() ?: return
        val ticket=++generation
        mutable.update {it.copy(busy=true,error=null,selected=null,text="")}
        job=viewModelScope.launch {
            try {val page=gateway.list(owner,cursor);if(current(ticket,owner))mutable.update {it.copy(busy=false,reports=page.items,nextAfterId=page.nextAfterId)}}
            catch(e:CancellationException){throw e}
            catch(e:Exception){if(ticket==generation)failure(e)}
        }
    }
    private fun today()=LocalDate.now(ZoneOffset.UTC).toString()
}
