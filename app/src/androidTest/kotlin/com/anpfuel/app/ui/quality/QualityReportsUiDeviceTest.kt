package com.anpfuel.app.ui.quality

import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.lifecycle.SavedStateHandle
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.QualityReportGateway
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Source cases only until the separately authorized device batch executes. */
@RunWith(AndroidJUnit4::class)
class QualityReportsUiDeviceTest {
    @get:Rule val compose=createComposeRule()
    private val station="eeeeeeee-5555-4555-8555-000000000001"
    private var signedIn=false
    private var commands=0
    private val gateway=object:QualityReportGateway {
        override fun currentOwner()=if(signedIn)"synthetic-owner" else null
        override suspend fun owner()=currentOwner() ?: throw QualityReportException(QualityReportFailure.AUTH_REQUIRED)
        override suspend fun list(owner:String,afterId:String?)=QualityReportPage(emptyList(),null)
        override suspend fun get(owner:String,reportId:String):QualityReport=error("unused")
        override suspend fun execute(owner:String,command:QualityReportCommand):QualityReportReceipt {commands++;return QualityReportReceipt(command.reportId,1,"PENDING_REVIEW")}
    }
    private fun label(id:Int)=InstrumentationRegistry.getInstrumentation().targetContext.getString(id)
    private fun show(signIn:()->Unit={}):QualityReportsViewModel {
        val vm=QualityReportsViewModel(SavedStateHandle(mapOf("stationId" to station,"fuelProduct" to "ETHANOL")),gateway)
        compose.setContent {AnpFuelTheme {QualityReportsScreen({},signIn,vm)}}
        compose.runOnIdle {vm.resume()}
        compose.waitUntil(5000){vm.state.value.visible && !vm.state.value.busy}
        return vm
    }
    @Test fun guestHasFreeLoginAndNoPrivateForm() {
        var logins=0;show {logins++}
        compose.onNodeWithText(label(R.string.quality_reports_login)).performClick()
        compose.onNodeWithText(label(R.string.quality_reports_text)).assertDoesNotExist()
        assertEquals(1,logins);assertEquals(0,commands)
    }
    @Test fun submitRequiresDeliberateConfirmationAndGuidanceDoesNotSubmit() {
        signedIn=true
        val vm=show()
        compose.runOnIdle {vm.day("2020-01-01");vm.text("Synthetic claim")}
        compose.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(label(R.string.quality_reports_submit)))
        compose.onNodeWithText(label(R.string.quality_reports_submit)).performClick()
        compose.onNodeWithText(label(R.string.quality_reports_confirm_submit)).assertIsDisplayed()
        assertEquals(0,commands)
        compose.onNodeWithText(label(R.string.action_cancel)).performClick()
        compose.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(label(R.string.quality_reports_anp)))
        compose.onNodeWithText(label(R.string.quality_reports_anp)).performClick()
        compose.onNodeWithText(label(R.string.quality_reports_anp_handoff)).assertIsDisplayed()
        assertEquals(0,commands)
    }
}
