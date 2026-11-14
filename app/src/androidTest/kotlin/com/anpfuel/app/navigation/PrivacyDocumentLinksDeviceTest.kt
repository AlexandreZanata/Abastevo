package com.anpfuel.app.navigation

import androidx.activity.ComponentActivity
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.Density
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.privacy.PrivacyDocumentLinks
import com.anpfuel.app.ui.privacy.PrivacyDocumentLinksCard
import com.anpfuel.app.ui.theme.AnpFuelTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class PrivacyDocumentLinksDeviceTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()

    private fun verify(large: Boolean) {
        val opened = mutableListOf<String>()
        compose.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, if (large) 2f else 1f)) {
                AnpFuelTheme(darkTheme = large, dynamicColor = false) {
                    PrivacyDocumentLinksCard(onOpen = { opened += it })
                }
            }
        }
        compose.runOnIdle { assertTrue("Opening privacy must not submit anything", opened.isEmpty()) }
        compose.onNodeWithTag("privacy-full-policy").assertIsDisplayed().performClick()
        compose.runOnIdle { assertEquals(listOf(PrivacyDocumentLinks.POLICY), opened) }
        compose.onNodeWithTag("privacy-external-deletion").assertIsDisplayed().performClick()
        compose.runOnIdle {
            assertEquals(listOf(
                "https://abastevo.com.br/privacidade/",
                "https://abastevo.com.br/excluir-conta/",
            ), opened)
        }
    }

    @Test fun missingBrowserShowsTheAddressAndAllowsAnotherAttempt() {
        var attempts = 0
        compose.setContent {
            AnpFuelTheme(darkTheme = false, dynamicColor = false) {
                PrivacyDocumentLinksCard(onOpen = {
                    attempts++
                    if (attempts < 3) throw IllegalArgumentException("No browser in isolated test")
                })
            }
        }
        compose.onNodeWithTag("privacy-full-policy").performClick()
        compose.onNodeWithTag("privacy-document-error")
            .assertIsDisplayed().assertTextContains(PrivacyDocumentLinks.POLICY, substring = true)
        compose.onNodeWithTag("privacy-external-deletion").performClick()
        compose.onNodeWithTag("privacy-document-error")
            .assertIsDisplayed().assertTextContains(PrivacyDocumentLinks.DELETION, substring = true)
        compose.onNodeWithTag("privacy-external-deletion").performClick()
        compose.onNodeWithTag("privacy-document-error").assertDoesNotExist()
        compose.runOnIdle { assertEquals(3, attempts) }
    }

    @Test fun publicDocumentsRequireExplicitActionsAndUseExactStaticDestinations() = verify(large = false)
    @Test fun bothActionsRemainAccessibleWithLargeTextAndDarkTheme() = verify(large = true)
}
