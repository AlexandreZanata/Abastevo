package com.anpfuel.app.ui.quality

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Density
import androidx.lifecycle.SavedStateHandle
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.application.usecase.quality.ReadQuality
import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.QualityGateway
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Compiled source cases; actual device execution is separately recorded. */
@RunWith(AndroidJUnit4::class)
class QualityUiDeviceTest {
    @get:Rule val compose=createComposeRule()
    private val station="eeeeeeee-5555-4555-8555-000000000001"
    private val revision="eeeeeeee-6666-4666-8666-000000000001"
    private val lease=QualityLease("pmqc","pmqc-assay-v1","2026-09",revision,true,true,"2026-10-18T00:00:00Z","published",true)
    private val row=QualityAssay("eeeeeeee-6666-4666-8666-000000000002","sample","Synthetic assay","2026-09-01","ETHANOL","LII","","FUTURE_STATUS")
    private val gateway=object:QualityGateway {
        override suspend fun summary(stationId:String,fuel:String,revisionId:String?)=QualitySummary(station,lease)
        override suspend fun assays(stationId:String,fuel:String,revisionId:String,cursor:String?)=QualityPage(listOf(row.copy(fuel=fuel)),null,lease)
        override suspend fun revisions(cursor:String?)=QualityPage<QualityRevision>(emptyList(),null)
        override suspend fun sample(sampleRef:String,revisionId:String)=QualitySample(sampleRef,listOf(row),lease)
        override suspend fun provenance(revisionId:String)=QualityProvenance(lease,emptyList())
    }
    private fun label(id:Int)=InstrumentationRegistry.getInstrumentation().targetContext.getString(id)
    private fun show(font:Float=1f):QualityViewModel {
        val vm=QualityViewModel(SavedStateHandle(mapOf("stationId" to station,"fuelProduct" to "ETHANOL")),ReadQuality(gateway),gateway)
        compose.setContent {AnpFuelTheme {
            val density=LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density,font)) {QualityScreen({},vm)}
        }}
        compose.waitUntil(5000){vm.state.value.snapshot!=null}
        return vm
    }
    @Test fun guestSeesDatedEvidenceAndUnavailableScoreAtDoubleFont() {
        show(2f)
        compose.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(label(R.string.quality_score_unavailable)))
        compose.onNodeWithText(label(R.string.quality_score_unavailable)).assertIsDisplayed()
        compose.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(label(R.string.quality_first_page)))
        compose.onNodeWithText(label(R.string.quality_first_page)).assertIsEnabled()
    }
    @Test fun unknownAssayNeverBecomesAnApprovalAndSampleRemainsReachable() {
        show()
        compose.onNode(hasScrollToNodeAction()).performScrollToNode(hasText("Synthetic assay"))
        compose.onNodeWithText(label(R.string.quality_unknown)).assertIsDisplayed()
        compose.onNodeWithText("Synthetic assay").performClick()
        compose.onNodeWithText(label(R.string.quality_sample_title)).assertIsDisplayed()
        compose.onNodeWithText(label(R.string.action_back)).performClick()
        compose.onNodeWithText(label(R.string.quality_sample_title)).assertDoesNotExist()
    }
}
