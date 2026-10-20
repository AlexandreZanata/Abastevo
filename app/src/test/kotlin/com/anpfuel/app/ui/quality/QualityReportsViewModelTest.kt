package com.anpfuel.app.ui.quality

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.QualityReportGateway
import io.mockk.*
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.*
import org.junit.jupiter.api.Assertions.*

@OptIn(ExperimentalCoroutinesApi::class)
class QualityReportsViewModelTest {
    private val dispatcher=StandardTestDispatcher()
    private val gateway=mockk<QualityReportGateway>()
    private val station="eeeeeeee-5555-4555-8555-000000000001"
    private val revision="eeeeeeee-6666-4666-8666-000000000001"
    private var current:String?="synthetic-owner"
    @BeforeEach fun before() {
        Dispatchers.setMain(dispatcher)
        every {gateway.currentOwner()} answers {current}
        coEvery {gateway.owner()} coAnswers {current ?: throw QualityReportException(QualityReportFailure.AUTH_REQUIRED)}
        coEvery {gateway.pending(any())} returns null
        coEvery {gateway.list(any(),any())} returns QualityReportPage(emptyList(),null)
        coEvery {gateway.execute(any(),any())} coAnswers {secondArg<QualityReportCommand>().let {QualityReportReceipt(it.reportId,it.expectedRevision+1,"PENDING_REVIEW")}}
    }
    @AfterEach fun after(){Dispatchers.resetMain()}
    private fun vm(link:Boolean=false)=QualityReportsViewModel(SavedStateHandle(mapOf("stationId" to station,"fuelProduct" to "ETHANOL","revisionId" to if(link)revision else "","sampleRef" to if(link)"sample:exact/one" else "")),gateway)
    private suspend fun ready(vm:QualityReportsViewModel,scope:TestScope) {vm.resume();scope.advanceUntilIdle();vm.day("2020-01-01");vm.text("Synthetic claim")}

    @Test fun `guest never requests owner data or commands`()=runTest(dispatcher) {
        current=null
        val vm=vm();vm.resume();advanceUntilIdle();vm.request("SUBMIT");vm.confirm()
        assertNull(vm.state.value.owner);assertEquals(QualityReportFailure.AUTH_REQUIRED,vm.state.value.error)
        coVerify(exactly=0){gateway.list(any(),any())};coVerify(exactly=0){gateway.execute(any(),any())}
    }
    @Test fun `double tap and edit during unresolved command cannot fork intent`()=runTest(dispatcher) {
        val entered=CompletableDeferred<Unit>();val release=CompletableDeferred<Unit>()
        val commands=mutableListOf<QualityReportCommand>()
        coEvery {gateway.execute(any(),capture(commands))} coAnswers {
            val c=secondArg<QualityReportCommand>();entered.complete(Unit);release.await();QualityReportReceipt(c.reportId,1,"PENDING_REVIEW")
        }
        val vm=vm();ready(vm,this);vm.request("SUBMIT");vm.confirm();vm.confirm();runCurrent()
        assertTrue(entered.isCompleted)
        vm.text("Replacement should not be submitted");vm.request("SUBMIT");vm.retry()
        assertEquals("Synthetic claim",vm.state.value.text);assertEquals(1,commands.size)
        release.complete(Unit);advanceUntilIdle()
        assertNotNull(vm.state.value.receipt);assertNull(vm.state.value.pending)
    }
    @Test fun `uncertain outcome retries the same stable command with fresh gateway authority`()=runTest(dispatcher) {
        val commands=mutableListOf<QualityReportCommand>()
        coEvery {gateway.execute(any(),capture(commands))} coAnswers {
            if(commands.size==1)throw QualityReportException(QualityReportFailure.UNAVAILABLE)
            val c=secondArg<QualityReportCommand>();QualityReportReceipt(c.reportId,1,"PENDING_REVIEW")
        }
        val vm=vm();ready(vm,this);vm.request("SUBMIT");vm.confirm();advanceUntilIdle()
        assertNotNull(vm.state.value.pending);assertNull(vm.state.value.receipt)
        vm.retry();advanceUntilIdle()
        assertEquals(2,commands.size);assertEquals(commands[0],commands[1]);assertNotNull(vm.state.value.receipt)
    }
    @Test fun `changed owner during accepted response clears private receipt and draft`()=runTest(dispatcher) {
        val release=CompletableDeferred<Unit>()
        coEvery {gateway.execute(any(),any())} coAnswers {
            val c=secondArg<QualityReportCommand>();withContext(NonCancellable){release.await()};QualityReportReceipt(c.reportId,1,"PENDING_REVIEW")
        }
        val vm=vm();ready(vm,this);vm.request("SUBMIT");vm.confirm();runCurrent()
        current="other-owner";release.complete(Unit);advanceUntilIdle()
        assertNull(vm.state.value.owner);assertNull(vm.state.value.receipt);assertNull(vm.state.value.pending)
        assertTrue(vm.state.value.text.isEmpty());assertTrue(vm.state.value.reports.isEmpty())
    }
    @Test fun `stopped screen hides private content and stale owner read cannot return after resume`()=runTest(dispatcher) {
        val release=CompletableDeferred<Unit>()
        coEvery {gateway.list("synthetic-owner",any())} coAnswers {
            withContext(NonCancellable){release.await()};QualityReportPage(listOf(report()),null)
        }
        val vm=vm();vm.resume();runCurrent();vm.pause();assertFalse(vm.state.value.visible)
        current="other-owner";vm.resume();runCurrent();release.complete(Unit);advanceUntilIdle()
        assertEquals("other-owner",vm.state.value.owner);assertTrue(vm.state.value.reports.isEmpty())
    }
    @Test fun `link review retains exact displayed sample revision and fuel through confirmation`()=runTest(dispatcher) {
        val vm=vm(true);ready(vm,this);vm.request("SUBMIT")
        val c=vm.state.value.confirmation!!
        assertEquals("LINK_REVIEW",c.kind);assertEquals(revision,c.officialRevisionId)
        assertEquals("sample:exact/one",c.sampleRef);assertEquals(station,c.stationId);assertEquals("ETHANOL",c.fuel)
        vm.dismissConfirmation();assertNull(vm.state.value.confirmation)
        coVerify(exactly=0){gateway.execute(any(),any())}
    }
    @Test fun `edit withdrawal and appeal pin owner report revision without rewriting its station`()=runTest(dispatcher) {
        val r=report().copy(status="REVIEWED",revision=3)
        coEvery {gateway.list(any(),any())} returns QualityReportPage(listOf(r),null)
        val vm=vm();ready(vm,this);vm.select(r);vm.text("Synthetic appeal")
        for(action in listOf("EDIT","WITHDRAW","APPEAL")) {
            vm.request(action);val c=vm.state.value.confirmation!!
            assertEquals(r.id,c.reportId);assertEquals(3,c.expectedRevision)
            if(action=="WITHDRAW")assertTrue(c.text.isEmpty())
            if(action=="APPEAL")assertTrue(c.stationId.isEmpty())
            vm.dismissConfirmation()
        }
    }
    private fun report()=QualityReport(revision,1,"EXPERIENCE",station,"ETHANOL","2020-01-01","OTHER","Synthetic claim","","","PENDING_REVIEW","USER_CLAIM","")
    @Test fun `recreated screen restores only the current owners frozen pending intent`()=runTest(dispatcher) {
        val pending=QualityReportCommand(revision,revision,"SUBMIT",0,"EXPERIENCE",station,"ETHANOL","2020-01-01","OTHER","Synthetic queued claim")
        coEvery {gateway.pending("synthetic-owner")} returns pending
        val restored=vm();restored.resume();advanceUntilIdle()
        assertEquals(pending,restored.state.value.pending)
        assertEquals("Synthetic queued claim",restored.state.value.text)
        restored.text("must not fork");assertEquals("Synthetic queued claim",restored.state.value.text)
        current="other-owner";restored.resume();advanceUntilIdle()
        assertNull(restored.state.value.pending);assertTrue(restored.state.value.text.isEmpty())
        coVerify(exactly=0){gateway.execute(any(),any())}
    }

    @Test fun `account history retains owner reads without a station and cannot submit a fake station`()=runTest(dispatcher) {
        val history=QualityReportsViewModel(SavedStateHandle(mapOf("stationId" to "own")),gateway)
        history.resume();advanceUntilIdle()
        assertTrue(history.historyOnly);assertEquals("synthetic-owner",history.state.value.owner)
        history.day("2020-01-01");history.text("Synthetic claim");history.request("SUBMIT")
        assertNull(history.state.value.confirmation)
        coVerify(exactly=1){gateway.list("synthetic-owner",null)}
        coVerify(exactly=0){gateway.execute(any(),any())}
    }

}
