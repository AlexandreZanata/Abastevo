package com.anpfuel.app.ui.quality

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.app.BuildConfig
import com.anpfuel.application.usecase.quality.ReadQuality
import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.*
import io.mockk.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.*
import org.junit.jupiter.api.Assertions.*

@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
class QualityFlagsTest {
    private val dispatcher=StandardTestDispatcher()
    private val station="eeeeeeee-5555-4555-8555-000000000001"
    @BeforeEach fun before(){Dispatchers.setMain(dispatcher)}
    @AfterEach fun after(){Dispatchers.resetMain()}
    @Test fun `disabled public entry performs no source reads but owner history and withdrawal survive`()=runTest(dispatcher) {
        if(BuildConfig.QUALITY_UI_ENABLED)return@runTest
        val public=mockk<QualityGateway>()
        val vm=QualityViewModel(SavedStateHandle(mapOf("stationId" to station)),ReadQuality(public),public)
        vm.selectFuel(com.anpfuel.domain.valueobject.FuelProduct.ETHANOL);vm.loadRevisions();vm.refresh()
        advanceUntilIdle();assertTrue(vm.state.value.unavailable)
        coVerify(exactly=0){public.summary(any(),any(),any())}
        coVerify(exactly=0){public.revisions(any())}
        val privateGateway=mockk<QualityReportGateway>()
        val report=QualityReport(station,1,"EXPERIENCE",station,"ETHANOL","2020-01-01","OTHER","Synthetic claim","","","REVIEWED","USER_CLAIM","")
        every {privateGateway.currentOwner()} returns "synthetic-owner"
        coEvery {privateGateway.owner()} returns "synthetic-owner"
        coEvery {privateGateway.pending(any())} returns null
        coEvery {privateGateway.list(any(),any())} returns QualityReportPage(listOf(report),null)
        val history=QualityReportsViewModel(SavedStateHandle(mapOf("stationId" to "own")),privateGateway)
        history.resume();advanceUntilIdle();assertEquals("synthetic-owner",history.state.value.owner)
        history.select(report);history.request("WITHDRAW")
        assertEquals("WITHDRAW",history.state.value.confirmation?.action)
        coVerify(exactly=0){privateGateway.execute(any(),any())}
    }
}
