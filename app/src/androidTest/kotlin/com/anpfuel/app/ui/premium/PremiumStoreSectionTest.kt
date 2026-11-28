package com.anpfuel.app.ui.premium

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.StoreOffer
import com.anpfuel.domain.premium.StoreProduct
import com.anpfuel.domain.premium.StoreProvider
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class PremiumStoreSectionTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun unavailableStoreShowsRetryWithoutPurchaseDetails() {
        val offer = StoreOffer(
            provider = StoreProvider.PLAY,
            products = listOf(StoreProduct("abastevo_premium", listOf("monthly"))),
            accountBinding = "a".repeat(64),
            managementUrl = "https://play.google.com/store/account/subscriptions?package=com.abastevo",
        )
        val status = PremiumStatus(
            policyVersion = 1,
            state = PremiumState.NONE,
            reason = "no-purchase",
            capabilities = emptySet(),
            validUntil = null,
            retainedDataAccess = true,
            paymentsConfigured = true,
            managementUrl = PremiumStatus.MANAGEMENT_URL,
            storeOffers = listOf(offer),
        )
        var retries = 0
        compose.setContent {
            AnpFuelTheme {
                PremiumStoreSection(
                    state = PremiumUiState(signedOut = false, status = status, plansUnavailable = true),
                    onSubscribe = { error("Subscribe cannot be called without a store plan") },
                    onRetryPlans = { retries++ },
                )
            }
        }

        val resources = InstrumentationRegistry.getInstrumentation().targetContext.resources
        compose.onNodeWithText(resources.getString(R.string.premium_store_plans_unavailable)).assertIsDisplayed()
        compose.onNodeWithText(resources.getString(R.string.premium_store_plans_retry_hint)).assertIsDisplayed()
        compose.onNodeWithText(resources.getString(R.string.premium_store_billing_note)).assertDoesNotExist()
        compose.onNode(hasText(resources.getString(R.string.premium_store_subscribe)) and hasClickAction())
            .assertDoesNotExist()
        compose.onNodeWithText(resources.getString(R.string.action_retry)).performClick()
        compose.runOnIdle { assertEquals(1, retries) }
    }
}
