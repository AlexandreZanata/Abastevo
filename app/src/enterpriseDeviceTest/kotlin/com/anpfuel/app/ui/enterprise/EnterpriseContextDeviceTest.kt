package com.anpfuel.app.ui.enterprise

import android.content.Context
import android.view.KeyEvent
import android.view.WindowManager
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
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
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TestWatcher
import org.junit.runner.Description
import org.junit.runner.RunWith

/** The physical runner must retain only this isolated app's data before this method. */
@Target(AnnotationTarget.FUNCTION)
@Retention(AnnotationRetention.RUNTIME)
annotation class EnterpriseRetainSyntheticSession

@HiltAndroidTest
@RunWith(AndroidJUnit4::class)
class EnterpriseContextDeviceTest {
    @Inject lateinit var scenario: EnterpriseDeviceScenario
    @Inject lateinit var contexts: EnterpriseContextDeviceScenario
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
            check(context.packageName == "com.anpfuel.app.enterprisevalidation")
            AppLocaleHolder.localeTag = "pt-BR"
            if (description.getAnnotation(EnterpriseRetainSyntheticSession::class.java) != null) {
                // A fresh process must use the existing Keystore/session, not a fixture login.
                val fixture = context.getSharedPreferences("enterprise-device-validation", Context.MODE_PRIVATE)
                check(fixture.getBoolean("company-selection-prepared", false)) { "Cold-process preparation is required" }
                check(fixture.edit().remove("company-selection-prepared").commit())
                val retained = checkNotNull(sessions.load()) { "Cold-process preparation is required" }
                check(retained.accountId == EnterpriseDeviceScenario.ACCOUNT)
                scenario.familyId = retained.familyId
            } else {
                check(TermsAcceptancePrefsStore(context).save(TermsAcceptanceRule.accept(
                    PublishedLegalDocuments.CURRENT, ticked = true, nowMillis = System.currentTimeMillis(),
                )))
                runBlocking {
                    val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
                    preferences.write(UserPreferences(
                        onboardingCompleted = true, locationPromptCompleted = true,
                        autoDownloadLatestWeek = false, activeSurveyWeek = week,
                        preferredState = BrazilianState.PARANA, preferredMunicipality = "Curitiba",
                        localeTag = "pt-BR", localeUserSelected = true,
                    ))
                    database.surveyWeekDao().insert(SurveyWeekEntity(
                        DomainId.forSurveyWeek(week).value, week.startDate.toString(),
                        week.endDate.toString(), System.currentTimeMillis(), null,
                    ))
                }
                check(auth.logout() is AuthApiResult.Ok)
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
        compose.waitUntil(30_000L) { compose.onAllNodes(tab).fetchSemanticsNodes().isNotEmpty() }
        compose.onNode(tab).performClick()
        await("enterprise-context-entry")
        compose.onNodeWithTag("enterprise-context-entry").performScrollTo().performClick()
        await("enterprise-context-screen")
        assertSecureWindow()
    }

    private fun option(organization: String) = "enterprise-context-option-$organization"

    private fun select(organization: String, name: String) {
        await(option(organization))
        compose.onNodeWithTag(option(organization)).performScrollTo().assertIsEnabled().performClick()
        home(name)
        awaitAbsent("enterprise-context-unconfirmed")
    }

    private fun home(name: String) {
        await("enterprise-home")
        compose.waitUntil(30_000L) {
            compose.onAllNodes(hasTestTag("enterprise-context-name") and hasText(name, substring = true))
                .fetchSemanticsNodes().isNotEmpty()
        }
        awaitAbsent("enterprise-context-chooser")
        compose.onNodeWithTag("enterprise-context-header-name").assertTextContains(name, substring = true).assertIsDisplayed()
        compose.onNodeWithTag("enterprise-context-name").performScrollTo().assertTextContains(name, substring = true).assertIsDisplayed()
    }

    private fun switcher() {
        compose.onNodeWithTag("enterprise-context-switch").performScrollTo().performClick()
        await("enterprise-context-chooser")
    }

    private fun reconfirm() {
        compose.onNodeWithTag("enterprise-context-reconfirm").performScrollTo().assertIsEnabled().performClick()
        awaitAbsent("enterprise-context-unconfirmed")
    }

    @Test
    fun twoCompaniesUseCurrentSourceAndPersonalReturnIsExplicit() {
        contexts.paginated = true
        enter()
        await(option(EnterpriseContextDeviceScenario.ORGANIZATION_A))
        compose.onNodeWithTag(option(EnterpriseContextDeviceScenario.ORGANIZATION_B)).assertDoesNotExist()
        select(EnterpriseContextDeviceScenario.ORGANIZATION_A, EnterpriseContextDeviceScenario.COMPANY_A)
        switcher()
        compose.onNodeWithTag("enterprise-context-refresh").performScrollTo().performClick()
        compose.waitUntil(10_000L) { contexts.lists.get() == 2 && contexts.completedReads.get() == 2 }
        compose.waitForIdle()
        compose.onNodeWithTag("enterprise-context-header-name")
            .assertTextContains(EnterpriseContextDeviceScenario.COMPANY_A, substring = true)
        if (compose.onAllNodes(hasTestTag("enterprise-context-chooser")).fetchSemanticsNodes().isEmpty()) switcher()
        compose.onNodeWithTag("enterprise-context-next").performScrollTo().performClick()
        await(option(EnterpriseContextDeviceScenario.ORGANIZATION_B))
        compose.onNodeWithTag(option(EnterpriseContextDeviceScenario.ORGANIZATION_A)).assertDoesNotExist()
        assertEquals(3, contexts.lists.get())
        select(EnterpriseContextDeviceScenario.ORGANIZATION_B, EnterpriseContextDeviceScenario.COMPANY_B)
        switcher()
        InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
        awaitAbsent("enterprise-context-chooser")
        home(EnterpriseContextDeviceScenario.COMPANY_B)
        assertEquals(3, contexts.reads.get())
        InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
        await("enterprise-context-chooser")
        compose.onNodeWithTag("enterprise-context-header-name")
            .assertTextContains(EnterpriseContextDeviceScenario.COMPANY_B, substring = true)
        compose.onNodeWithTag("enterprise-context-personal").performScrollTo().performClick()
        awaitAbsent("enterprise-context-screen")
        compose.onNodeWithTag("enterprise-home").assertDoesNotExist()
        assertEquals(EnterpriseDeviceScenario.ACCOUNT, sessions.load()?.accountId)
    }

    @Test
    fun offlineAndBackgroundKeepOnlyVisiblyUnconfirmedMetadata() {
        enter()
        select(EnterpriseContextDeviceScenario.ORGANIZATION_A, EnterpriseContextDeviceScenario.COMPANY_A)
        scenario.connected.value = false
        await("enterprise-context-unconfirmed")
        home(EnterpriseContextDeviceScenario.COMPANY_A)
        compose.onNodeWithTag("enterprise-context-reconfirm").assertIsNotEnabled()
        switcher()
        compose.onNodeWithTag(option(EnterpriseContextDeviceScenario.ORGANIZATION_A)).performScrollTo().assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-context-refresh").performScrollTo().assertIsNotEnabled()
        InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
        awaitAbsent("enterprise-context-chooser")
        assertEquals(1, contexts.reads.get())
        scenario.connected.value = true
        compose.waitForIdle()
        await("enterprise-context-unconfirmed")
        reconfirm()
        compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
        compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        compose.runOnIdle { check(compose.activity.lifecycle.currentState == Lifecycle.State.RESUMED) }
        await("enterprise-context-unconfirmed")
        home(EnterpriseContextDeviceScenario.COMPANY_A)
        reconfirm()
        assertEquals(3, contexts.reads.get())
    }

    @Test
    fun aLateSelectionCannotRestoreAuthorityAndDefinitiveRefusalClearsNames() {
        enter()
        select(EnterpriseContextDeviceScenario.ORGANIZATION_A, EnterpriseContextDeviceScenario.COMPANY_A)
        switcher()
        contexts.readHold = CompletableDeferred()
        compose.onNodeWithTag(option(EnterpriseContextDeviceScenario.ORGANIZATION_B)).performScrollTo().performClick()
        compose.waitUntil(10_000L) { contexts.reads.get() == 2 }
        compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
        compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        contexts.readHold!!.complete(Unit)
        compose.waitUntil(10_000L) { contexts.completedReads.get() == 2 }
        compose.waitForIdle()
        InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
        awaitAbsent("enterprise-context-chooser")
        home(EnterpriseContextDeviceScenario.COMPANY_A)
        await("enterprise-context-unconfirmed")
        contexts.denied = true
        compose.onNodeWithTag("enterprise-context-reconfirm").performScrollTo().performClick()
        awaitAbsent("enterprise-home")
        await("enterprise-context-chooser")
        compose.onNodeWithTag("enterprise-context-header-name").assertDoesNotExist()
        compose.onNodeWithText(EnterpriseContextDeviceScenario.COMPANY_A).assertDoesNotExist()
        compose.onNodeWithText(EnterpriseContextDeviceScenario.COMPANY_B).assertDoesNotExist()
        compose.onNodeWithTag(option(EnterpriseContextDeviceScenario.ORGANIZATION_A)).assertDoesNotExist()
        compose.onNodeWithTag(option(EnterpriseContextDeviceScenario.ORGANIZATION_B)).assertDoesNotExist()
        compose.onNodeWithText(compose.activity.getString(R.string.enterprise_context_denied)).assertIsDisplayed()
    }

    @Test
    fun activityRecreationRetainsOnlyUnconfirmedCompanyMetadata() {
        enter()
        select(EnterpriseContextDeviceScenario.ORGANIZATION_A, EnterpriseContextDeviceScenario.COMPANY_A)
        compose.activityRule.scenario.recreate()
        await("enterprise-context-screen")
        assertSecureWindow()
        home(EnterpriseContextDeviceScenario.COMPANY_A)
        await("enterprise-context-unconfirmed")
        reconfirm()
    }

    @Test
    fun prepareCompanySelectionForTheFollowingFreshProcess() {
        enter()
        select(EnterpriseContextDeviceScenario.ORGANIZATION_A, EnterpriseContextDeviceScenario.COMPANY_A)
        assertEquals(EnterpriseDeviceScenario.ACCOUNT, sessions.load()?.accountId)
        // Only a fixture boolean is persisted; company metadata remains owned by the product flow.
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        check(context.getSharedPreferences("enterprise-device-validation", Context.MODE_PRIVATE)
            .edit().putBoolean("company-selection-prepared", true).commit())
    }

    @Test
    @EnterpriseRetainSyntheticSession
    fun aFreshProcessStartsPersonalWithoutReplacingTheExistingAccount() {
        contexts.listHold = CompletableDeferred()
        enter()
        compose.waitUntil(10_000L) { contexts.lists.get() == 1 }
        compose.onNodeWithTag("enterprise-home").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-context-name").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-context-header-name").assertDoesNotExist()
        compose.onNodeWithText(EnterpriseContextDeviceScenario.COMPANY_A).assertDoesNotExist()
        compose.onNodeWithText(EnterpriseContextDeviceScenario.COMPANY_B).assertDoesNotExist()
        assertEquals(0, scenario.logins.get())
        assertEquals(EnterpriseDeviceScenario.ACCOUNT, sessions.load()?.accountId)
        contexts.listHold!!.complete(Unit)
        await(option(EnterpriseContextDeviceScenario.ORGANIZATION_A))
        compose.onNodeWithTag("enterprise-home").assertDoesNotExist()
        assertEquals(0, contexts.reads.get())
    }

    private fun assertSecureWindow() = compose.runOnIdle {
        assertTrue(compose.activity.window.attributes.flags and WindowManager.LayoutParams.FLAG_SECURE != 0)
    }

    private fun await(tag: String) {
        compose.waitUntil(30_000L) { compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty() }
    }

    private fun awaitAbsent(tag: String) {
        compose.waitUntil(30_000L) { compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isEmpty() }
    }
}
