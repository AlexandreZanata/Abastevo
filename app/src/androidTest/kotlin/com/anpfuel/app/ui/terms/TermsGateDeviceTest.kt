package com.anpfuel.app.ui.terms

import androidx.activity.ComponentActivity
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.legal.LegalDocuments
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** First-use terms screen contract (B-BR-TA01); source case until the authorized device batch runs. */
@RunWith(AndroidJUnit4::class)
class TermsGateDeviceTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()
    private val documents = LegalDocuments("2026-10", "1.0")

    @Test fun acceptNeedsATickAndLinksOrDeclineNeverAccept() {
        var ticked by mutableStateOf(false)
        var accepts = 0
        var declines = 0
        val opened = mutableListOf<String>()
        compose.setContent {
            AnpFuelTheme(dynamicColor = false) {
                TermsAcceptanceScreen(
                    documents = documents, changed = true, ticked = ticked, failed = false,
                    onTick = { ticked = it }, onAccept = { accepts++ }, onDecline = { declines++ },
                    onOpen = { opened += it },
                )
            }
        }
        compose.onNodeWithTag("terms-changed").assertIsDisplayed()
        compose.onNodeWithTag("terms-accept").performScrollTo().assertIsNotEnabled()
        compose.onNodeWithTag("terms-read-terms").performScrollTo().performClick()
        compose.onNodeWithTag("terms-read-privacy").performScrollTo().performClick()
        assertEquals(listOf(TermsDocumentLinks.TERMS, TermsDocumentLinks.PRIVACY), opened)
        compose.onNodeWithTag("terms-accept").performScrollTo().assertIsNotEnabled()
        compose.onNodeWithTag("terms-decline").performScrollTo().performClick()
        assertEquals(1, declines)
        assertEquals(0, accepts)
        compose.onNodeWithTag("terms-check").performScrollTo().performClick()
        compose.onNodeWithTag("terms-accept").performScrollTo().assertIsEnabled().performClick()
        assertEquals(1, accepts)
    }
}
