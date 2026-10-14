package com.anpfuel.app.community

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.port.CommunityHelpfulnessFlagProvider
import com.anpfuel.application.usecase.community.HelpfulnessVoteOutcome
import com.anpfuel.application.usecase.community.SubmitHelpfulnessVoteUseCase
import com.anpfuel.domain.repository.CommunityHelpfulnessRejectKind
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Shared signed helpfulness, independent confirmation and structured report controller.
 * UI changes only after server outcomes. Main serialization plus target/owner/origin generations
 * prevent late responses from rendering or releasing another account/revision operation.
 */
sealed interface CommunityHelpfulnessUiState {
    data object Disabled : CommunityHelpfulnessUiState
    data object Guest : CommunityHelpfulnessUiState
    data object Idle : CommunityHelpfulnessUiState
    data object Loading : CommunityHelpfulnessUiState
    data class Content(
        val observationId: String,
        val decisionSequence: Long,
        val submissionId: String,
        val helpful: Long,
        val notHelpful: Long,
        val total: Long,
        val helpfulBasisPoints: Long?,
        val myChoice: String?,
        val underReview: Boolean,
    ) : CommunityHelpfulnessUiState
    data object Submitting : CommunityHelpfulnessUiState
    data class Rejected(
        val kind: CommunityHelpfulnessRejectKind,
        val message: String,
    ) : CommunityHelpfulnessUiState
}

@HiltViewModel
class CommunityHelpfulnessViewModel @Inject constructor(
    private val useCase: SubmitHelpfulnessVoteUseCase,
    private val flagProvider: CommunityHelpfulnessFlagProvider,
    private val proof: com.anpfuel.data.remote.PhotoProofTransport? = null,
) : ViewModel() {
    internal var ioDispatcher: kotlinx.coroutines.CoroutineDispatcher = kotlinx.coroutines.Dispatchers.IO
    private data class Target(val observationId:String,val decisionSequence:Long,val clientSubmissionId:String,val scope:String?,val accountId:String,val ownerScope:String?)
    private var target:Target?=null
    private var generation=0L
    private var submitting=false
    private var pendingChoice:String?=null
    private val actionIds=mutableMapOf<String,String>()
    private val _state=MutableStateFlow<CommunityHelpfulnessUiState>(if(flagProvider.isEnabled()) CommunityHelpfulnessUiState.Idle else CommunityHelpfulnessUiState.Disabled)
    val state=_state.asStateFlow()
    private val _actionReceived=MutableStateFlow(false)
    val actionReceived=_actionReceived.asStateFlow()
    fun signedAccount():String = proof?.ownerScope()?.takeIf { it.startsWith("account:") }.orEmpty()
    private fun scopeStamp():String? = proof?.let { it.origin+"|"+it.ownerScope() }

    fun prepare(observationId:String,decisionSequence:Long,accountId:String) {
        if(!flagProvider.isEnabled() || accountId.isBlank()) {
            generation++;target=null;submitting=false;pendingChoice=null;actionIds.clear();_actionReceived.value=false
            _state.value=if(flagProvider.isEnabled()) CommunityHelpfulnessUiState.Guest else CommunityHelpfulnessUiState.Disabled
            return
        }
        if(target?.let { it.observationId==observationId && it.decisionSequence==decisionSequence && it.scope==scopeStamp() && it.accountId==accountId }==true) { refresh();return }
        generation++;submitting=false;pendingChoice=null;actionIds.clear();_actionReceived.value=false
        target=Target(observationId,decisionSequence,UUID.randomUUID().toString(),scopeStamp(),accountId,proof?.ownerScope())
        refresh()
    }
    fun clear() { generation++;target=null;submitting=false;_state.value=CommunityHelpfulnessUiState.Idle;_actionReceived.value=false }
    private fun perform(loading:Boolean=false,onAccepted:()->Unit={},action:suspend (Target)->CommunityHelpfulnessUiState) {
        val current=target ?: return
        if(proof!=null && current.scope!=scopeStamp()) {clear();_state.value=CommunityHelpfulnessUiState.Guest;return}
        if(submitting) return
        val ticket=++generation
        submitting=true
        _state.value=if(loading) CommunityHelpfulnessUiState.Loading else CommunityHelpfulnessUiState.Submitting
        viewModelScope.launch {
            try {
                val result=kotlinx.coroutines.withContext(ioDispatcher) { action(current) }
                if(ticket!=generation || current!=target) return@launch
                if(proof!=null && current.scope!=scopeStamp()) {clear();_state.value=CommunityHelpfulnessUiState.Guest;return@launch}
                _state.value=result
                if(result is CommunityHelpfulnessUiState.Content) {target=current.copy(clientSubmissionId=result.submissionId);onAccepted()}
            } catch(cancelled:kotlinx.coroutines.CancellationException) {throw cancelled}
            catch(_:Exception) {if(ticket==generation) _state.value=CommunityHelpfulnessUiState.Rejected(CommunityHelpfulnessRejectKind.TRANSPORT,"community action unavailable")}
            finally {if(ticket==generation) submitting=false}
        }
    }
    fun refresh()=perform(loading=true) { current ->
        when(val outcome=useCase.tally(SubmitHelpfulnessVoteUseCase.TallyRequest(current.observationId,current.decisionSequence,current.ownerScope))) {
            HelpfulnessVoteOutcome.Disabled -> CommunityHelpfulnessUiState.Disabled
            is HelpfulnessVoteOutcome.Tallied -> content(current,outcome.tally.helpful,outcome.tally.notHelpful,outcome.tally.total,outcome.tally.helpfulBasisPoints,outcome.tally.myChoice,outcome.tally.underReview)
            is HelpfulnessVoteOutcome.Rejected -> CommunityHelpfulnessUiState.Rejected(outcome.kind,outcome.message)
            else -> CommunityHelpfulnessUiState.Rejected(CommunityHelpfulnessRejectKind.TRANSPORT,"community action unavailable")
        }
    }
    fun vote(choiceWire:String) {
        if(submitting) return
        if(pendingChoice!=null && pendingChoice!=choiceWire) target=target?.copy(clientSubmissionId=UUID.randomUUID().toString())
        pendingChoice=choiceWire
        perform {current ->
            when(val outcome=useCase.cast(SubmitHelpfulnessVoteUseCase.CastRequest(current.observationId,current.clientSubmissionId,choiceWire,current.decisionSequence,current.ownerScope))) {
                HelpfulnessVoteOutcome.Disabled -> CommunityHelpfulnessUiState.Disabled
                is HelpfulnessVoteOutcome.Voted -> content(current,outcome.receipt.helpful,outcome.receipt.notHelpful,outcome.receipt.total,outcome.receipt.helpfulBasisPoints,outcome.receipt.myChoice,false)
                is HelpfulnessVoteOutcome.Rejected -> CommunityHelpfulnessUiState.Rejected(outcome.kind,outcome.message)
                else -> CommunityHelpfulnessUiState.Rejected(CommunityHelpfulnessRejectKind.TRANSPORT,"community action unavailable")
            }
        }
    }
    fun remove()=perform {current ->
        when(val outcome=useCase.remove(SubmitHelpfulnessVoteUseCase.RemoveRequest(current.observationId,current.decisionSequence,current.ownerScope))) {
            HelpfulnessVoteOutcome.Disabled -> CommunityHelpfulnessUiState.Disabled
            is HelpfulnessVoteOutcome.Removed -> content(current.copy(clientSubmissionId=UUID.randomUUID().toString()),outcome.tally.helpful,outcome.tally.notHelpful,outcome.tally.total,outcome.tally.helpfulBasisPoints,outcome.tally.myChoice,outcome.tally.underReview)
            is HelpfulnessVoteOutcome.Rejected -> CommunityHelpfulnessUiState.Rejected(outcome.kind,outcome.message)
            else -> CommunityHelpfulnessUiState.Rejected(CommunityHelpfulnessRejectKind.TRANSPORT,"community action unavailable")
        }
    }
    fun report(reason:String) {
        require(reason in setOf("PRICE_CHANGED","WRONG_STATION","WRONG_PRODUCT","WRONG_CONDITION","EVIDENCE_MISMATCH","OTHER"))
        publicAction("disputes",reason)
    }
    fun confirmPrice()=publicAction("confirmations",null)
    private fun publicAction(route:String,reason:String?) {
        val transport=proof ?: return
        val before=_state.value as? CommunityHelpfulnessUiState.Content ?: return
        val id=actionIds.getOrPut(route+reason) {UUID.randomUUID().toString()}
        _actionReceived.value=false
        perform(onAccepted={_actionReceived.value=true}) {current ->
            val doc=org.json.JSONObject().put("client_submission_id",id)
            if(reason!=null) doc.put("reason",reason)
            val scope=current.ownerScope ?: throw java.io.IOException("community owner unavailable")
            transport.post("/v1/observations/${current.observationId}/$route",id,doc,scope)
            before
        }
    }
    private fun content(current:Target,helpful:Long,notHelpful:Long,total:Long,basisPoints:Long?,myChoice:String?,underReview:Boolean)=
        CommunityHelpfulnessUiState.Content(current.observationId,current.decisionSequence,current.clientSubmissionId,helpful,notHelpful,total,basisPoints,myChoice,underReview)
}
