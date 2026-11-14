package com.anpfuel.app.ui.enterprise

import android.content.res.Configuration
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.sync.EnterpriseSyncViewState
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.*
import java.time.Instant
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Presentation fixtures contain sanitized evidence only; they are not runtime producers. */
@RunWith(AndroidJUnit4::class)
class EnterpriseSyncPresentationDeviceTest {
    @get:Rule val compose = createComposeRule()
    private val context = EnterpriseContextSummary(EnterprisePartition.of(
        "10000000-0000-4000-8000-000000000001", "https://example.invalid", "10000000-0000-4000-8000-000000000002"),
        "10000000-0000-4000-8000-000000000003", "Synthetic company A")
    private val time = Instant.parse("2026-11-13T12:00:00Z")
    private fun item(index: Int, result: SyncResult? = null, attachments: Int = 0, code: String? = null): EnterpriseSyncItem {
        val id = "10000000-0000-4000-8000-" + index.toString().padStart(12, '0')
        return EnterpriseSyncItem(EnterpriseOperationIdentity(id, "sha256:" + "a".repeat(64)), "validation.presentation", 1,
            TimeDeclaration(time, "UTC", 0), QueueRetryState(), result?.let {
                EnterpriseSyncReceipt(it, code, time.plusSeconds(60), emptyList(), if (it == SyncResult.ACCEPTED) 2 else null,
                    "sha256:" + "b".repeat(64))
            }, attachments)
    }
    private fun state(items: List<EnterpriseSyncItem> = emptyList(), detail: EnterpriseSyncDetail? = null,
        active: Set<EnterpriseOperationIdentity> = emptySet(), error: EnterpriseAccessError? = null) =
        EnterpriseSyncViewState(generation = 4, epoch = 0, context = context, items = items, detail = detail, active = active, error = error)

    private fun content(state: EnterpriseSyncViewState, connected: Boolean = true,
        onDetail: (EnterpriseOperationIdentity) -> Unit = {}, onDismiss: (EnterpriseOperationIdentity, String) -> Unit = { _, _ -> }) {
        val application = InstrumentationRegistry.getInstrumentation().targetContext
        val configuration = Configuration(application.resources.configuration).apply { setLocale(Locale.ENGLISH) }
        val localized = application.createConfigurationContext(configuration)
        compose.setContent {
            CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                MaterialTheme {
                    EnterpriseSyncPanel(state, connected, {}, {}, {}, {}, onDetail, {}, onDismiss)
                }
            }
        }
    }

    @Test fun retainedReceiptsRemainTextuallyDistinctAndOnlyMatchingActualActivityShowsSending() {
        val saved = item(4)
        val accepted = item(5, SyncResult.ACCEPTED)
        val review = item(6, SyncResult.ACCEPTED_FOR_REVIEW, code = "ORPHAN_REFERENCE")
        val refused = item(7, SyncResult.REJECTED, code = "MALFORMED")
        content(state(listOf(saved, accepted, review, refused), active = setOf(saved.identity, accepted.identity)))
        for ((status, label) in listOf(
            EnterpriseSyncStatus.SAVED_ON_DEVICE to "Saved on this device; not yet confirmed",
            EnterpriseSyncStatus.NEEDS_REVIEW to "Needs review",
            EnterpriseSyncStatus.REJECTED to "Not accepted",
            EnterpriseSyncStatus.ACCEPTED to "Accepted by the company",
        )) {
            val tag = "enterprise-sync-group-${status.name}"
            compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag(tag))
            compose.onNodeWithTag(tag).assertTextContains(label).assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
        }
        compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag("enterprise-sync-item-${saved.identity.operationId}"))
        compose.onNodeWithTag("enterprise-sync-item-${saved.identity.operationId}").assertTextContains("Sending", substring = true)
        compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag("enterprise-sync-item-${accepted.identity.operationId}"))
        compose.onNodeWithTag("enterprise-sync-item-${accepted.identity.operationId}").assertTextContains("Accepted by the company", substring = true)
        compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag("enterprise-sync-item-${review.identity.operationId}"))
        compose.onNodeWithTag("enterprise-sync-item-${review.identity.operationId}").assertTextContains("Needs review", substring = true)
            .assertTextContains("A related record was not found", substring = true)
        compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag("enterprise-sync-item-${refused.identity.operationId}"))
        compose.onNodeWithTag("enterprise-sync-item-${refused.identity.operationId}").assertTextContains("Not accepted", substring = true)
            .assertTextContains("The record format could not be accepted", substring = true)
        compose.onNodeWithTag("enterprise-sync-manual-attention").assertDoesNotExist()
        compose.onNodeWithText("Approved").assertDoesNotExist()
        compose.onNodeWithText("ORPHAN_REFERENCE").assertDoesNotExist()
        compose.onNodeWithText("validation.presentation").assertDoesNotExist()
    }

    @Test fun refusedVisibilityRequiresAnExplicitDigestBoundConfirmation() {
        val refused = item(7, SyncResult.REJECTED, code = "MALFORMED")
        var dismissed: Pair<EnterpriseOperationIdentity, String>? = null
        content(state(detail = EnterpriseSyncDetail(refused, emptyList())), onDismiss = { identity, receipt -> dismissed = identity to receipt })
        compose.onNodeWithTag("enterprise-sync-dismiss").performScrollTo().performClick()
        compose.runOnIdle { assertEquals(null, dismissed) }
        compose.onNodeWithTag("enterprise-sync-dismiss-confirm").performClick()
        compose.runOnIdle { assertTrue(dismissed?.first == refused.identity && dismissed?.second == refused.receipt!!.receiptDigest) }
        compose.onNodeWithText("Delete record").assertDoesNotExist()
    }

    @Test fun acceptedOperationDoesNotInventAnAttachmentUploadOrSaveTimestamp() {
        val accepted = item(5, SyncResult.ACCEPTED, attachments = 1)
        val attachment = EnterpriseSyncAttachment("10000000-0000-4000-8000-000000000008", 8, "c".repeat(64),
            "image/jpeg", EnterpriseAttachmentEvidence.MISSING)
        content(state(detail = EnterpriseSyncDetail(accepted, listOf(attachment))))
        compose.onNodeWithText("Accepted by the company").assertIsDisplayed()
        compose.onNodeWithText("Upload is not confirmed").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Missing on this device").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Save time was not recorded").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Upload complete").assertDoesNotExist()
        compose.onNodeWithText("Retry upload").assertDoesNotExist()
    }

    @Test fun unavailablePrivateStatusNeverRendersRowsAndNetworkCheckingStaysDisabledOffline() {
        val saved = item(4)
        content(state(listOf(saved), error = EnterpriseAccessError.UNAVAILABLE), connected = false)
        compose.onNodeWithText("Status unavailable").assertIsDisplayed()
        compose.onNodeWithTag("enterprise-sync-item-${saved.identity.operationId}").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-sync-check").assertIsNotEnabled()
        compose.onNodeWithText("Everything sent").assertDoesNotExist()
    }

    @Test fun homePressureAndLegacyWarningDoNotClaimGlobalCompletion() {
        val storage = EnterpriseQueueStats(0, 0, 1, 8000, 100)
        val totals = EnterpriseSyncTotals(0, 0, 0, 0, 0, 0, 0, 0, 0, storage, null, null)
        val application = InstrumentationRegistry.getInstrumentation().targetContext
        val configuration = Configuration(application.resources.configuration).apply { setLocale(Locale.ENGLISH) }
        val localized = application.createConfigurationContext(configuration)
        var opened = false
        compose.setContent {
            CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                MaterialTheme { EnterpriseSyncHomeStatus(totals, { opened = true }) }
            }
        }
        compose.onNodeWithTag("enterprise-sync-storage-pressure").assertIsDisplayed()
        compose.onNodeWithTag("enterprise-sync-legacy").assertIsDisplayed()
        compose.onNodeWithTag("enterprise-sync-open").performClick()
        compose.runOnIdle { assertTrue(opened) }
        compose.onNodeWithText("Everything sent").assertDoesNotExist()
    }

    @Test fun actualManualAttentionIsInformativeAndKeepsReadableSavedRows() {
        val saved = item(4)
        content(state(listOf(saved)).copy(manualAttention = 2, manualWaitingMillis = 15_000))
        compose.onNodeWithTag("enterprise-sync-manual-attention").assertIsDisplayed()
        compose.onNodeWithTag("enterprise-sync-manual-wait").assertIsDisplayed()
        compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag("enterprise-sync-item-${saved.identity.operationId}"))
        compose.onNodeWithTag("enterprise-sync-item-${saved.identity.operationId}").assertIsDisplayed()
        compose.onNodeWithText("Status unavailable").assertDoesNotExist()
        compose.onNodeWithText("Everything sent").assertDoesNotExist()
    }
}
