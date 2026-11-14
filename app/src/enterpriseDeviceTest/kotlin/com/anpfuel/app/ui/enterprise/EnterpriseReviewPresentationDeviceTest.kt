package com.anpfuel.app.ui.enterprise

import android.content.res.Configuration
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.sync.EnterpriseSyncViewState
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.*
import java.time.Instant
import java.util.Locale
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Sanitized review outcome evidence only; these fixtures are not runtime producers. */
@RunWith(AndroidJUnit4::class)
class EnterpriseReviewPresentationDeviceTest {
    @get:Rule val compose = createComposeRule()
    private val context = EnterpriseContextSummary(EnterprisePartition.of(
        "10000000-0000-4000-8000-000000000001", "https://example.invalid", "10000000-0000-4000-8000-000000000002"),
        "10000000-0000-4000-8000-000000000003", "Synthetic company A")
    private val time = Instant.parse("2026-11-13T12:00:00Z")
    private val digest = "sha256:" + "a".repeat(64)
    private fun id(index: Int) = "10000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private fun item(index: Int, outcome: String?, attachments: Int = 0, dismissed: Boolean = false): EnterpriseSyncItem {
        val identity = EnterpriseOperationIdentity(id(index), digest)
        val resolution = outcome?.let {
            EnterpriseSyncResolution.from(ReviewResolution.create(ReviewResolutionSnapshot(identity.operationId, digest, id(900 + index),
                id(950 + index), 2, it, if (it == ReviewResolution.REFUSED) "Synthetic reviewer refusal" else "",
                "2026-11-13T12:05:00Z", if (it == ReviewResolution.ACCEPTED) 3 else null)))
        }
        return EnterpriseSyncItem(identity, "validation.presentation", 1, TimeDeclaration(time, "UTC", 0), QueueRetryState(),
            EnterpriseSyncReceipt(SyncResult.ACCEPTED_FOR_REVIEW, "PERMISSION_CHANGED", time.plusSeconds(60), emptyList(), null,
                "sha256:" + "b".repeat(64)), attachments, dismissed, resolution)
    }
    private fun state(items: List<EnterpriseSyncItem> = emptyList(), detail: EnterpriseSyncDetail? = null) =
        EnterpriseSyncViewState(generation = 4, epoch = 0, context = context, items = items, detail = detail)
    private fun content(state: EnterpriseSyncViewState, onDismiss: (EnterpriseOperationIdentity, String) -> Unit = { _, _ -> }) {
        val application = InstrumentationRegistry.getInstrumentation().targetContext
        val configuration = Configuration(application.resources.configuration).apply { setLocale(Locale.ENGLISH) }
        val localized = application.createConfigurationContext(configuration)
        compose.setContent {
            CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                MaterialTheme { EnterpriseSyncPanel(state, true, {}, {}, {}, {}, {}, {}, onDismiss) }
            }
        }
    }

    @Test fun listShowsEachActualReviewOutcomeTruthfully() {
        val held = item(4, null); val accepted = item(5, ReviewResolution.ACCEPTED, attachments = 1); val refused = item(6, ReviewResolution.REFUSED)
        content(state(listOf(held, accepted, refused)))
        for ((row, labels) in listOf(
            held to listOf("Needs review", "No company decision has been received yet", "The permission changed before receipt"),
            accepted to listOf("Accepted after company review", "Declared attachments: 1 · upload unconfirmed"),
            refused to listOf("Refused after company review"),
        )) {
            val tag = "enterprise-sync-item-${row.identity.operationId}"
            compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag(tag))
            labels.forEach { compose.onNodeWithTag(tag).assertTextContains(it, substring = true) }
        }
        compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag("enterprise-sync-item-${accepted.identity.operationId}"))
        compose.onNodeWithTag("enterprise-sync-item-${accepted.identity.operationId}")
            .assert(SemanticsMatcher.expectValue(androidx.compose.ui.semantics.SemanticsProperties.StateDescription, "Accepted after company review"))
        compose.onNodeWithText("Synthetic reviewer refusal", substring = true).assertDoesNotExist()
        compose.onNodeWithText(id(905)).assertDoesNotExist()
        compose.onNodeWithText("Upload complete").assertDoesNotExist()
    }

    @Test fun acceptedDecisionDetailKeepsOriginalReceiptAndOffersNoHiding() {
        val accepted = item(5, ReviewResolution.ACCEPTED)
        content(state(detail = EnterpriseSyncDetail(accepted, emptyList())))
        compose.onNodeWithText("Accepted after company review").assertIsDisplayed()
        compose.onNodeWithTag("enterprise-sync-review-decision").performScrollTo().assertIsDisplayed()
            .assertTextContains("Company review decision", substring = true)
            .assertTextContains("No reason was given with this acceptance", substring = true)
            .assertTextContains("The original review receipt remains unchanged.", substring = true)
            .assertTextContains("Decision recorded by the server:", substring = true)
        compose.onNodeWithText("Confirmed server receipt:", substring = true).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Confirmed review decision:", substring = true).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("The permission changed before receipt").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("enterprise-sync-dismiss").assertDoesNotExist()
    }

    @Test fun refusedDecisionHidingIsBoundToTheResolutionDigest() {
        val refused = item(6, ReviewResolution.REFUSED)
        var dismissed: Pair<EnterpriseOperationIdentity, String>? = null
        content(state(detail = EnterpriseSyncDetail(refused, emptyList())), onDismiss = { identity, value -> dismissed = identity to value })
        compose.onNodeWithTag("enterprise-sync-review-decision").performScrollTo()
            .assertTextContains("Reviewer’s reason: Synthetic reviewer refusal", substring = true)
        compose.onNodeWithTag("enterprise-sync-dismiss").performScrollTo().performClick()
        compose.runOnIdle { assertNull(dismissed) }
        compose.onNodeWithTag("enterprise-sync-dismiss-confirm").performClick()
        compose.runOnIdle {
            assertEquals(refused.identity, dismissed?.first)
            assertEquals(refused.resolution!!.resolutionDigest, dismissed?.second)
            assertNotEquals(refused.receipt!!.receiptDigest, dismissed?.second)
        }
    }

    @Test fun unresolvedDetailCannotBeHiddenAndAllCatalogsTranslateTheOutcomes() {
        val held = item(4, null)
        content(state(detail = EnterpriseSyncDetail(held, emptyList())))
        compose.onNodeWithText("No company decision has been received yet").assertIsDisplayed()
        compose.onNodeWithTag("enterprise-sync-dismiss").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-sync-review-decision").assertDoesNotExist()
        val application = InstrumentationRegistry.getInstrumentation().targetContext
        val keys = listOf(R.string.enterprise_sync_review_accepted, R.string.enterprise_sync_review_refused,
            R.string.enterprise_sync_review_unresolved, R.string.enterprise_sync_review_decision_title,
            R.string.enterprise_sync_review_reason, R.string.enterprise_sync_review_no_reason,
            R.string.enterprise_sync_review_decided_at, R.string.enterprise_sync_review_original_kept,
            R.string.enterprise_sync_review_history)
        fun texts(locale: Locale) = application.createConfigurationContext(
            Configuration(application.resources.configuration).apply { setLocale(locale) }).resources.let { r -> keys.map(r::getString) }
        val english = texts(Locale.ENGLISH)
        for (tag in listOf("pt-BR", "es", "fr", "de", "ja", "ru", "zh-CN")) {
            val localized = texts(Locale.forLanguageTag(tag))
            assertEquals(keys.size, localized.toSet().size)
            for (index in keys.indices) assertNotEquals("$tag must translate review text $index", english[index], localized[index])
            assertTrue(localized.filterIndexed { index, _ -> index in setOf(4, 6, 8) }.all { it.contains("%1\$s") })
        }
    }
}
