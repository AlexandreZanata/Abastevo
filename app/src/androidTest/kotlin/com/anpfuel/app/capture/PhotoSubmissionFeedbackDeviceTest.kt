package com.anpfuel.app.capture

import androidx.activity.ComponentActivity
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Real native feedback without network, account changes or photo submission. */
@RunWith(AndroidJUnit4::class)
class PhotoSubmissionFeedbackDeviceTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()

    @Test fun queuedRetryReceivedAndReturnAreVisibleWithoutScrolling() {
        val status = mutableStateOf<CaptureOcrViewModel.SubmitState>(CaptureOcrViewModel.SubmitState.Queued(false,4))
        var returned = false
        compose.setContent { AnpFuelTheme(dynamicColor=false) {
            PhotoSubmissionFeedback(status.value) { returned = true }
        } }
        compose.onNodeWithText(compose.activity.getString(R.string.capture_saved_title)).assertIsDisplayed()
        compose.onNodeWithText(compose.activity.getString(R.string.capture_sent_count,4)).assertIsDisplayed()
        assertFalse(returned)
        compose.runOnIdle { status.value = CaptureOcrViewModel.SubmitState.Queued(false,4,true) }
        compose.onNodeWithText(compose.activity.getString(R.string.capture_queue_retrying)).assertIsDisplayed()
        compose.runOnIdle { status.value = CaptureOcrViewModel.SubmitState.Sent(4,true) }
        compose.onNodeWithText(compose.activity.getString(R.string.capture_sent_title)).assertIsDisplayed()
        compose.onNodeWithText(compose.activity.getString(R.string.capture_sent_pending_validation)).assertIsDisplayed()
        compose.onNodeWithText(compose.activity.getString(R.string.capture_back_community)).performClick()
        compose.runOnIdle { assertTrue(returned) }
    }

    @Test fun refusedSubsetNeverDisplaysAllPricesSent() {
        compose.setContent { AnpFuelTheme(dynamicColor=false) {
            PhotoSubmissionFeedback(CaptureOcrViewModel.SubmitState.Partial(1,"contribution.rejected")) { }
        } }
        compose.onNodeWithText(compose.activity.getString(R.string.capture_partial_title)).assertIsDisplayed()
        compose.onNodeWithText(compose.activity.getString(R.string.capture_partial,1,"contribution.rejected")).assertIsDisplayed()
    }
}
