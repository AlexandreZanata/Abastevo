package com.anpfuel.app.ui.enterprise

import android.view.WindowManager
import android.view.inputmethod.InputMethodManager
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.ComposeTimeoutException
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.geometry.Rect
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.hilt.work.HiltWorkerFactory
import androidx.lifecycle.Lifecycle
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.work.Configuration
import androidx.work.WorkManager
import com.anpfuel.app.MainActivity
import com.anpfuel.app.R
import com.anpfuel.app.locale.AppLocaleHolder
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.entity.SurveyWeekEntity
import com.anpfuel.data.local.preferences.TermsAcceptancePrefsStore
import com.anpfuel.data.local.preferences.UserPreferencesDataStore
import com.anpfuel.domain.legal.PublishedLegalDocuments
import com.anpfuel.domain.legal.TermsAcceptanceRule
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek
import dagger.hilt.android.testing.HiltAndroidRule
import dagger.hilt.android.testing.HiltAndroidTest
import javax.inject.Inject
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TestWatcher
import org.junit.runner.Description
import org.junit.runner.RunWith

@Target(AnnotationTarget.FUNCTION)
@Retention(AnnotationRetention.RUNTIME)
annotation class EnterpriseGuestAccount

/**
 * Actual MainActivity/Hilt/navigation/Keystore on the connected physical phone.
 * Only account and enterprise server outcomes are synthetic. The opt-in package
 * and test-only PREVIEW/blocking network bindings protect installed user apps.
 */
@HiltAndroidTest
@RunWith(AndroidJUnit4::class)
class EnterpriseInvitationDeviceTest {
    private var inputDiagnostic = "not-entered"
    @Inject lateinit var scenario: EnterpriseDeviceScenario
    @Inject lateinit var auth: AuthFlow
    @Inject lateinit var sessions: AuthSessionStore
    @Inject lateinit var preferences: UserPreferencesDataStore
    @Inject lateinit var database: AnpFuelDatabase
    @Inject lateinit var workerFactory: HiltWorkerFactory

    @get:Rule(order = 0) val hilt = HiltAndroidRule(this)
    @get:Rule(order = 1) val seed = object : TestWatcher() {
        override fun starting(description: Description) {
            hilt.inject()
            val context = InstrumentationRegistry.getInstrumentation().targetContext
            check(context.packageName == "com.anpfuel.app.enterprisevalidation") {
                "Enterprise device tests require the isolated validation package"
            }
            AppLocaleHolder.localeTag = "pt-BR"
            check(TermsAcceptancePrefsStore(context).save(TermsAcceptanceRule.accept(
                PublishedLegalDocuments.CURRENT, ticked = true, nowMillis = System.currentTimeMillis(),
            )))
            runBlocking {
                val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
                preferences.write(UserPreferences(
                    onboardingCompleted = true,
                    locationPromptCompleted = true,
                    autoDownloadLatestWeek = false,
                    activeSurveyWeek = week,
                    preferredState = BrazilianState.PARANA,
                    preferredMunicipality = "Curitiba",
                    localeTag = "pt-BR",
                    localeUserSelected = true,
                ))
                database.surveyWeekDao().insert(SurveyWeekEntity(
                    DomainId.forSurveyWeek(week).value, week.startDate.toString(),
                    week.endDate.toString(), System.currentTimeMillis(), null,
                ))
            }
            check(auth.logout() is AuthApiResult.Ok)
            if (description.getAnnotation(EnterpriseGuestAccount::class.java) == null) {
                check(auth.loginWithKey(EnterpriseDeviceScenario.ACCOUNT_KEY) is AuthApiResult.Ok)
            }
            try {
                WorkManager.getInstance(context)
            } catch (_: IllegalStateException) {
                WorkManager.initialize(context, Configuration.Builder().setWorkerFactory(workerFactory).build())
            }
        }
    }
    @get:Rule(order = 2) val compose = createAndroidComposeRule<MainActivity>()

    private fun enter() {
        val label = compose.activity.getString(R.string.nav_profile)
        val tab = hasContentDescription(label) and SemanticsMatcher.expectValue(SemanticsProperties.Role, Role.Tab)
        compose.waitUntil(30_000L) {
            compose.onAllNodes(tab).fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNode(tab).performClick()
        awaitTag("enterprise-invitation-entry")
        compose.onNodeWithTag("enterprise-invitation-entry").performScrollTo().performClick()
        awaitTag("enterprise-invitation-screen")
        compose.onNodeWithTag("enterprise-invitation-screen").assertIsDisplayed()
        assertSecureWindow()
    }

    private fun preview(code: String = EnterpriseDeviceScenario.CODE) {
        val before = inputContext()
        compose.onNodeWithTag("enterprise-invitation-code").performTextReplacement(code)
        inputDiagnostic = "before=$before; after=${inputContext()}"
        submitStableField("enterprise-invitation-code", "enterprise-invitation-preview", code)
        awaitTag("enterprise-invitation-consent")
        compose.onNodeWithText(EnterpriseDeviceScenario.COMPANY_NAME).assertIsDisplayed()
    }

    private fun accept() {
        compose.onNodeWithTag("enterprise-invitation-consent").performScrollTo().performClick()
        compose.onNodeWithTag("enterprise-invitation-accept").performScrollTo().performClick()
    }

    private fun result(resource: Int) {
        awaitTag("enterprise-invitation-result")
        compose.onNodeWithTag("enterprise-invitation-result")
            .assertTextContains(compose.activity.getString(resource)).assertIsDisplayed()
    }

    private fun assertSecureWindow() = compose.runOnIdle {
        assertTrue(compose.activity.window.attributes.flags and WindowManager.LayoutParams.FLAG_SECURE != 0)
    }

    @Test
    fun pendingRequiresConsentAndServerAcknowledgement() {
        enter()
        scenario.decisionHold = CompletableDeferred()
        preview()
        compose.onNodeWithTag("enterprise-invitation-consent").performScrollTo().assertIsOff()
        compose.onNodeWithTag("enterprise-invitation-accept").assertIsNotEnabled()
        assertEquals(0, scenario.accepts.get())
        accept()
        compose.waitUntil(10_000L) { scenario.accepts.get() == 1 }
        compose.onNodeWithTag("enterprise-invitation-result").assertDoesNotExist()
        scenario.decisionHold!!.complete(Unit)
        result(R.string.enterprise_invitation_pending)
        compose.onNodeWithText(compose.activity.getString(R.string.enterprise_invitation_active)).assertDoesNotExist()
        assertTrue(sessions.load()?.accountId == EnterpriseDeviceScenario.ACCOUNT)
    }

    @Test
    fun anActiveServerAcknowledgementIsShownSeparately() {
        scenario.pendingConfirmation = false
        enter()
        preview()
        compose.onNodeWithTag("enterprise-invitation-consent").performScrollTo().assertIsOff()
        accept()
        result(R.string.enterprise_invitation_active)
        assertEquals(1, scenario.accepts.get())
    }

    @Test
    fun offlineInvalidAndDeclineNeverGrantLocalAccess() {
        scenario.connected.value = false
        enter()
        compose.onNodeWithTag("enterprise-invitation-offline").assertIsDisplayed()
        compose.onNodeWithTag("enterprise-invitation-code").performTextInput(EnterpriseDeviceScenario.CODE)
        compose.onNodeWithTag("enterprise-invitation-preview").performScrollTo().assertIsNotEnabled()
        assertEquals(0, scenario.previews.get())
        scenario.connected.value = true
        scenario.invalidCode = true
        compose.onNodeWithTag("enterprise-invitation-code").performTextReplacement("invalid-invitation")
        compose.onNodeWithTag("enterprise-invitation-preview").performScrollTo().performClick()
        compose.waitUntil(10_000L) { scenario.previews.get() == 1 }
        compose.onNodeWithTag("enterprise-invitation-consent").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-invitation-result").assertDoesNotExist()
        compose.onNodeWithText(EnterpriseDeviceScenario.COMPANY_NAME).assertDoesNotExist()
        scenario.invalidCode = false
        preview()
        compose.onNodeWithTag("enterprise-invitation-consent").performScrollTo().assertIsOff()
        compose.onNodeWithTag("enterprise-invitation-decline").performScrollTo().performClick()
        result(R.string.enterprise_invitation_declined)
        assertEquals(0, scenario.accepts.get())
        assertEquals(1, scenario.declines.get())
        assertTrue(sessions.load()?.accountId == EnterpriseDeviceScenario.ACCOUNT)
    }

    @Test
    @EnterpriseGuestAccount
    fun guestSignInAndExplicitFreshReauthenticationUseExistingKey() {
        enter()
        compose.onNodeWithTag("enterprise-invitation-preview").assertIsNotEnabled()
        signIn()
        scenario.requireFresh = true
        compose.onNodeWithTag("enterprise-invitation-code").performTextReplacement(EnterpriseDeviceScenario.CODE)
        compose.onNodeWithTag("enterprise-invitation-preview").performScrollTo().performClick()
        compose.onNodeWithTag("enterprise-invitation-sign-in").performScrollTo().assertIsDisplayed()
        signIn()
        preview()
        assertEquals(2, scenario.logins.get())
        assertTrue(sessions.load()?.accountId == EnterpriseDeviceScenario.ACCOUNT)
    }

    private fun signIn() {
        compose.onNodeWithTag("enterprise-invitation-sign-in").performScrollTo().performClick()
        awaitTag("account-key-entry")
        assertSecureWindow()
        compose.onNodeWithTag("account-key-entry").performScrollTo().performTextReplacement(EnterpriseDeviceScenario.ACCOUNT_KEY)
        submitStableField("account-key-entry", "account-key-submit", EnterpriseDeviceScenario.ACCOUNT_KEY)
        awaitTag("enterprise-invitation-screen")
        compose.onNodeWithTag("enterprise-invitation-screen").assertIsDisplayed()
    }

    @Test
    fun anUncertainAcknowledgementRetriesTheSameDecisionAndOperation() {
        enter()
        preview()
        scenario.unavailable = true
        accept()
        compose.waitUntil(10_000L) { scenario.accepts.get() == 1 }
        compose.onNodeWithTag("enterprise-invitation-result").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-invitation-decline").assertIsNotEnabled()
        scenario.unavailable = false
        compose.onNodeWithTag("enterprise-invitation-accept").performScrollTo().performClick()
        result(R.string.enterprise_invitation_pending)
        assertEquals(2, scenario.accepts.get())
        assertEquals(1, scenario.operations.distinct().size)
        assertEquals(0, scenario.declines.get())
    }

    @Test
    fun backgroundingClearsPrivateInputAndDiscardsALateAcknowledgement() {
        enter()
        compose.onNodeWithTag("enterprise-invitation-code").performTextInput(EnterpriseDeviceScenario.CODE)
        assertMaskedInput("enterprise-invitation-code", EnterpriseDeviceScenario.CODE)
        compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
        compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        assertEmptyCode()
        scenario.decisionHold = CompletableDeferred()
        preview()
        accept()
        compose.waitUntil(10_000L) { scenario.accepts.get() == 1 }
        compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
        compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        scenario.decisionHold!!.complete(Unit)
        compose.waitForIdle()
        compose.onNodeWithTag("enterprise-invitation-result").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-invitation-consent").assertDoesNotExist()
        compose.onNodeWithText(EnterpriseDeviceScenario.COMPANY_NAME).assertDoesNotExist()
        assertEmptyCode()
        assertFalse(scenario.operations.isEmpty())
    }

    private fun assertEmptyCode() {
        awaitTag("enterprise-invitation-code")
        compose.onNodeWithTag("enterprise-invitation-code").assert(
            SemanticsMatcher.expectValue(SemanticsProperties.EditableText, AnnotatedString("")),
        )
    }

    private fun assertMaskedInput(field: String, value: String) {
        // Compose1.7 exposes visually transformed EditableText; the fake ports
        // separately require the exact raw fixture before acknowledging it.
        val displayed = PasswordVisualTransformation().filter(AnnotatedString(value)).text
        try {
            compose.onNodeWithTag(field).assert(SemanticsMatcher.expectValue(SemanticsProperties.EditableText, displayed))
        } catch (failure: AssertionError) {
            throw AssertionError("Synthetic input lifecycle: $inputDiagnostic; current=${inputContext()}", failure)
        }
    }

    private fun inputContext(): String = compose.runOnIdle {
        val activity = compose.activity
        "epoch=${activity.enterpriseFence.epoch},background=${activity.enterpriseFence.backgroundEpoch}," +
            "lifecycle=${activity.lifecycle.currentState},focus=${activity.window.decorView.hasWindowFocus()}"
    }

    private fun submitStableField(field: String, action: String, value: String) {
        assertMaskedInput(field, value)
        compose.onNodeWithTag(action).assertIsEnabled()
        compose.runOnIdle {
            val activity = compose.activity
            val decor = activity.window.decorView
            ViewCompat.getWindowInsetsController(decor)?.hide(WindowInsetsCompat.Type.ime())
            activity.getSystemService(InputMethodManager::class.java)
                ?.hideSoftInputFromWindow(decor.windowToken, 0)
        }
        compose.waitForIdle()
        compose.onNodeWithTag(action).performScrollTo()
        compose.waitForIdle()
        assertMaskedInput(field, value)
        compose.onNodeWithTag(action).assertIsDisplayed().assertIsEnabled()
        var previous: Rect? = null
        var stableFrames = 0
        compose.waitUntil(5_000L) {
            val bounds = compose.onNodeWithTag(action).fetchSemanticsNode().boundsInRoot
            val imeHidden = compose.runOnIdle {
                ViewCompat.getRootWindowInsets(compose.activity.window.decorView)
                    ?.isVisible(WindowInsetsCompat.Type.ime()) != true
            }
            stableFrames = if (imeHidden && previous == bounds) stableFrames + 1 else 0
            previous = bounds
            stableFrames >= 2
        }
        assertMaskedInput(field, value)
        compose.onNodeWithTag(action).assertIsDisplayed().assertIsEnabled().performClick()
    }

    private fun awaitTag(tag: String) {
        try {
            compose.waitUntil(30_000L) { compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty() }
        } catch (failure: ComposeTimeoutException) {
            // Diagnose only this isolated synthetic app. Never print input,
            // hierarchy text, device identifiers, screenshots or logcat.
            val tags = listOf("enterprise-invitation-screen", "enterprise-invitation-code", "enterprise-invitation-preview",
                "enterprise-invitation-consent", "enterprise-invitation-accept", "enterprise-invitation-result",
                "enterprise-invitation-sign-in", "account-key-entry", "account-key-submit")
            val nodes = tags.associateWith { key -> compose.onAllNodes(hasTestTag(key)).fetchSemanticsNodes() }
            val present = nodes.filterValues { it.isNotEmpty() }.keys
            val lengths = nodes.mapValues { (_, values) -> values.firstOrNull()?.config?.getOrNull(SemanticsProperties.EditableText)?.length }
            val disabled = nodes.filterValues { it.isNotEmpty() }.mapValues { (_, values) -> SemanticsProperties.Disabled in values.first().config }
            val errors = listOf(
                "auth_input" to R.string.auth_error_input, "auth_key" to R.string.auth_error_key,
                "auth_offline" to R.string.auth_error_offline, "auth_storage" to R.string.auth_error_secure_storage,
                "auth_unknown" to R.string.auth_error_unknown, "invitation_stale" to R.string.enterprise_invitation_stale,
                "invitation_fresh" to R.string.enterprise_invitation_fresh_required,
                "invitation_invalid" to R.string.enterprise_invitation_invalid,
                "invitation_unavailable" to R.string.enterprise_invitation_unavailable,
            ).filter { (_, id) -> compose.onAllNodes(androidx.compose.ui.test.hasText(compose.activity.getString(id))).fetchSemanticsNodes().isNotEmpty() }.map { it.first }
            throw AssertionError("Missing tag=$tag; synthetic calls: login=${scenario.logins.get()},preview=${scenario.previews.get()},accept=${scenario.accepts.get()}; sessionPresent=${sessions.load() != null}; connected=${scenario.connected.value}; present=$present; lengths=$lengths; disabled=$disabled; errors=$errors", failure)
        }
    }
}
