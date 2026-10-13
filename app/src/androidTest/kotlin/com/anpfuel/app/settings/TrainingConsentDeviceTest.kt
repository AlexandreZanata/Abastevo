package com.anpfuel.app.settings

import android.graphics.Rect
import android.os.Build
import android.view.View
import android.view.ViewGroup
import android.view.inspector.WindowInspector
import androidx.activity.ComponentActivity
import androidx.compose.ui.window.DialogWindowProvider
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.preferences.TrainingTermsSource
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.application.usecase.privacy.TrainingConsentCaller
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.repository.TrainingTermsDocument
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Native review/confirmation contract; no network, real photo, or account mutation. */
@RunWith(AndroidJUnit4::class)
class TrainingConsentDeviceTest {
    @get:Rule val compose=createAndroidComposeRule<ComponentActivity>()
    private val document=TrainingTermsDocument("training-terms-v1","a".repeat(64),"Synthetic complete offline document.")
    private val caller=TrainingConsentCaller("synthetic",ContributionScope("account:synthetic:key","https://test.invalid"))
    private fun text(id:Int)=compose.activity.getString(id)

    @Test fun readingAloneNeverConsentsAndOwnerChangeClearsTheCheckbox() {
        val host=mutableStateOf(ConsentHostState(true,caller,document))
        var grants=0
        compose.setContent {AnpFuelTheme(dynamicColor=false) {
            Column(Modifier.verticalScroll(rememberScrollState())) {
                TrainingConsentContent(host.value,TrainingConsentUiState.Content(false,null,null,null,0,false,false),
                    {enabled,version,digest -> if(enabled) {grants++;assertEquals(document.version,version);assertEquals(document.digest,digest)} },{}, {})
            }
        }}
        compose.onNodeWithText(text(R.string.privacy_confirm_preference)).performScrollTo().assertIsNotEnabled()
        compose.onNodeWithText(text(R.string.privacy_read_terms)).performScrollTo().performClick()
        compose.onNodeWithText(document.text).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.privacy_finish_reading)).performScrollTo().performClick()
        assertEquals(0,grants)
        compose.onNodeWithText(text(R.string.privacy_confirm_preference)).performScrollTo().assertIsNotEnabled()
        compose.onNodeWithText(text(R.string.privacy_consent_check)).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.privacy_confirm_preference)).performScrollTo().assertIsEnabled()
        compose.runOnIdle {host.value=host.value.copy(caller=caller.copy(accountId="other",scope=caller.scope!!.copy(ownerScope="account:other:key")))}
        compose.onNodeWithText(text(R.string.privacy_confirm_preference)).performScrollTo().assertIsNotEnabled()
        assertEquals(0,grants)
        compose.onNodeWithText(text(R.string.privacy_read_terms)).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.privacy_finish_reading)).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.privacy_consent_check)).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.privacy_confirm_preference)).performScrollTo().performClick()
        compose.runOnIdle {assertEquals(1,grants)}
        compose.onNodeWithText(text(R.string.privacy_confirm_preference)).assertIsNotEnabled()
    }

    @Test fun offlineRevocationAndRightsNeedNeitherTermsNorAcceptance() {
        var revoked=false
        compose.setContent {AnpFuelTheme(dynamicColor=false) {
            Column(Modifier.verticalScroll(rememberScrollState())) {
                TrainingConsentContent(ConsentHostState(true,caller,null),TrainingConsentUiState.Offline,
                    {enabled,_,_ ->assertFalse(enabled);revoked=true},{},{})
            }
        }}
        compose.onNodeWithText(text(R.string.privacy_revoke_delete)).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.privacy_rights_heading)).performScrollTo().assertIsDisplayed()
        compose.runOnIdle {assertTrue(revoked)}
        compose.onNodeWithText(text(R.string.privacy_confirm_preference)).assertDoesNotExist()
    }
    @Test fun completeBundledTermsAreReadableWithoutGrantingPermission() {
        val bundled=TrainingTermsSource(ApplicationProvider.getApplicationContext()).read()
        assertEquals("training-terms-v1",bundled.version)
        assertEquals("4a9763192620d267311f49fd43c9f4502144fa3bf78edc04b4112982e87bf8ce",bundled.digest)
        assertTrue(bundled.text.contains("8. Segurança e alterações"))
        val host=mutableStateOf(ConsentHostState(true,caller,bundled))
        var decisions=0
        compose.setContent {AnpFuelTheme(dynamicColor=false) {
            Column(Modifier.verticalScroll(rememberScrollState())) {
                TrainingConsentContent(host.value,TrainingConsentUiState.Offline,
                    {_,_,_ -> decisions++},{}, {})
            }
        }}
        compose.onNodeWithText(text(R.string.privacy_read_terms)).performScrollTo().performClick()
        compose.onNodeWithText(bundled.text).assertExists()
        val finish=compose.onNodeWithText(text(R.string.privacy_finish_reading)).performScrollTo().assertIsDisplayed()
        val bounds=finish.fetchSemanticsNode()
        assertEquals("The complete reading action must fit above system navigation",bounds.size.height.toFloat(),bounds.boundsInWindow.height,1f)
        if(Build.VERSION.SDK_INT>=29) {
            val usable=Rect()
            val offset=IntArray(2)
            compose.runOnUiThread {
                val dialog=WindowInspector.getGlobalWindowViews().single {containsDialogWindow(it)}
                dialog.getWindowVisibleDisplayFrame(usable)
                dialog.getLocationOnScreen(offset)
            }
            val bottom=bounds.boundsInWindow.bottom+offset[1]
            assertTrue("Reading action overlaps system navigation: $bottom vs $usable",bottom<=usable.bottom)
        }
        finish.performClick()
        compose.onNodeWithText(text(R.string.privacy_consent_check)).performScrollTo().assertIsOff()
        compose.onNodeWithText(text(R.string.privacy_confirm_preference)).performScrollTo().assertIsNotEnabled()
        compose.runOnIdle {assertEquals(0,decisions);host.value=host.value.copy(caller=null)}
        compose.onNodeWithText(text(R.string.privacy_read_terms)).performScrollTo().performClick()
        compose.onNodeWithText(bundled.text).assertExists()
        compose.onNodeWithContentDescription(text(R.string.action_back)).performClick()
        compose.onNodeWithText(text(R.string.privacy_rights_heading)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(text(R.string.privacy_confirm_preference)).assertDoesNotExist()
        compose.runOnIdle {assertEquals(0,decisions)}
    }

    private fun containsDialogWindow(view:View):Boolean {
        if(view is DialogWindowProvider) return true
        if(view is ViewGroup) for(i in 0 until view.childCount) {
            if(containsDialogWindow(view.getChildAt(i))) return true
        }
        return false
    }

}
