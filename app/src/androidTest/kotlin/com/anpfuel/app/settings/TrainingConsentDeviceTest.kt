package com.anpfuel.app.settings

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
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
}
