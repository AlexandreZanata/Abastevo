package com.anpfuel.app.ui.quality

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.usecase.quality.ReadQuality
import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.QualityGateway
import com.anpfuel.domain.valueobject.FuelProduct
import io.mockk.*
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.*
import org.junit.jupiter.api.Assertions.*

@OptIn(ExperimentalCoroutinesApi::class)
class QualityViewModelTest {
    private val dispatcher=StandardTestDispatcher()
    private val gateway=mockk<QualityGateway>()
    private val station="eeeeeeee-5555-4555-8555-000000000001"
    private val revision="eeeeeeee-6666-4666-8666-000000000001"
    private val lease=QualityLease("pmqc","pmqc-assay-v1","2026-09",revision,true,false)
    private val row=QualityAssay("eeeeeeee-6666-4666-8666-000000000002","sample","Aspecto","2026-09-01","ETHANOL","LII","","CONFORMING")
    @BeforeEach fun before() {
        Dispatchers.setMain(dispatcher)
        coEvery {gateway.summary(station,any(),any())} returns QualitySummary(station,lease)
        coEvery {gateway.assays(station,any(),revision,any())} coAnswers {QualityPage(listOf(row.copy(fuel=secondArg())),null,lease)}
        coEvery {gateway.revisions(any())} returns QualityPage(emptyList(),null)
    }
    @AfterEach fun after(){Dispatchers.resetMain()}
    private fun vm(id:String=station)=QualityViewModel(SavedStateHandle(mapOf("stationId" to id,"fuelProduct" to "ETHANOL")),ReadQuality(gateway),gateway)

    @Test fun `late old fuel results cannot replace current selection`()=runTest(dispatcher) {
        val entered=CompletableDeferred<Unit>();val release=CompletableDeferred<Unit>()
        coEvery {gateway.summary(station,"ETHANOL",null)} coAnswers {
            entered.complete(Unit);withContext(NonCancellable){release.await()};QualitySummary(station,lease)
        }
        val vm=vm();runCurrent();assertTrue(entered.isCompleted)
        vm.selectFuel(FuelProduct.DIESEL_S10);runCurrent()
        release.complete(Unit);advanceUntilIdle()
        assertEquals(FuelProduct.DIESEL_S10,vm.state.value.fuel)
        assertEquals("DIESEL_S10",vm.state.value.snapshot?.assays?.items?.single()?.fuel)
    }

    @Test fun `late sample detail is discarded after fuel switch`()=runTest(dispatcher) {
        val vm=vm();advanceUntilIdle()
        val entered=CompletableDeferred<Unit>();val release=CompletableDeferred<Unit>()
        coEvery {gateway.sample("sample",revision)} coAnswers {
            entered.complete(Unit);withContext(NonCancellable){release.await()};QualitySample("sample",listOf(row),lease)
        }
        vm.openSample("sample");runCurrent();assertTrue(entered.isCompleted)
        vm.selectFuel(FuelProduct.DIESEL_S10);runCurrent()
        release.complete(Unit);advanceUntilIdle()
        assertNull(vm.state.value.sample)
        assertFalse(vm.state.value.detailLoading)
        assertEquals(FuelProduct.DIESEL_S10,vm.state.value.fuel)
    }

    @Test fun `disabled backend clears formerly visible public facts`()=runTest(dispatcher) {
        val vm=vm();advanceUntilIdle();assertNotNull(vm.state.value.snapshot)
        coEvery {gateway.summary(station,"ETHANOL",null)} throws QualityReadUnavailable(disabled=true)
        vm.refresh();advanceUntilIdle()
        assertNull(vm.state.value.snapshot);assertTrue(vm.state.value.unavailable)
        assertFalse(vm.state.value.stale)
    }

    @Test fun `invalid station never reaches the gateway`()=runTest(dispatcher) {
        val vm=vm("../other");advanceUntilIdle()
        assertTrue(vm.state.value.unavailable)
        coVerify(exactly=0){gateway.summary(any(),any(),any())}
        coVerify(exactly=0){gateway.revisions(any())}
    }
}
