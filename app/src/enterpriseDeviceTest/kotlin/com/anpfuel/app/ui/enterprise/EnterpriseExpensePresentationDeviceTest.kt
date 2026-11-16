package com.anpfuel.app.ui.enterprise

import android.content.res.Configuration
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.ui.enterprise.expense.ExpensePanel
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.attachment.ReceiptDraft
import com.anpfuel.application.enterprise.expense.ExpenseForm
import com.anpfuel.application.enterprise.expense.ExpensePayer
import com.anpfuel.application.enterprise.expense.ExpenseViewState
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.attachment.*
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.expense.*
import com.anpfuel.domain.enterprise.fleet.*
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import java.time.Instant
import java.util.Locale
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** SCR-EXP-01 presentation (EMP-EXP-004) over real expense views built from real operations and receipts. */
@RunWith(AndroidJUnit4::class)
class EnterpriseExpensePresentationDeviceTest {
    @get:Rule val compose = createComposeRule()
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val partition = EnterprisePartition.of(id(1), "https://example.invalid", id(2))
    private val context = EnterpriseContextSummary(partition, id(3), "Synthetic company A", id(5))
    private val operation: EnterpriseOperation = ExpenseOperations.submit(ExpenseEnvelope(SyncBinding.of(id(1), "https://example.invalid", id(2)), id(3), id(4),
        id(100), TimeDeclaration(EnterpriseTime.parseInstant("2026-11-15T12:00:00Z"), "UTC", 0)), id(110), id(20), "TOLL", "1250", "1000", "EMPLOYEE", null)
    private fun ack(result: String = "ACCEPTED", code: String? = null) = OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${operation.operationId}",""" +
        """"payload_digest":"${operation.payloadDigest}","result":"$result","code":${code?.let { "\"$it\"" } ?: "null"},"entity_revision":${if (result == "ACCEPTED") "1" else "null"},""" +
        """"received_at":"2026-11-15T16:00:00Z","review_item_id":null,"warnings":[]}""").toByteArray(), operation)
    private fun view(ack: OperationAcknowledgement?) = ExpensePresentation.view(EnterpriseQueueEntry(operation, acknowledgement = ack))
    private fun receipt(state: AttachmentState, accepted: Boolean) = AttachmentPresentation.view(EnterpriseAttachment(id(700), id(701), id(702), operation.operationId,
        AttachmentTarget(DocumentTargetKind.EXPENSE, id(110)), "ab".repeat(32), 2048, Instant.parse("2026-11-15T12:01:00Z"), state, null,
        if (state == AttachmentState.WAITING_FOR_RECORD) null else 1), if (accepted) ack() else null)
    private fun state(ack: OperationAcknowledgement? = null, receipt: AttachmentView? = null, open: Boolean = true, saveFailed: Boolean = false,
        captureInvalid: Boolean = false) =
        ExpenseViewState(generation = 3, epoch = 0, context = context, loaded = true, expenses = listOf(view(ack)), openExpenseId = if (open) id(110) else null,
            attachments = receipt?.let { mapOf(operation.operationId to listOf(it)) } ?: emptyMap(), saveFailed = saveFailed, captureInvalid = captureInvalid)
    private val assets = run {
        val request = ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(id(70), id(71), "synthetic-reference-access", id(3), id(4), null))
        val asset = """{"id":"${id(20)}","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"Caminhão sintético","fleet_number":"FROTA-12","plate":"ABC1D23","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[],"assignment":{"kind":"PRIMARY","valid_to":null}}"""
        val raw = """{"schema_version":1,"request_id":"${id(70)}","membership_id":"${id(3)}","enrollment_id":"${id(4)}","mode":"RESET","reason":"NEW_CURSOR","from_revision":null,"revision":1,"confirmed_at":"2026-11-15T12:00:00Z","cursor":"c-1","entity_types":["asset"],"changes":[{"revision":1,"entity_type":"asset","entity_id":"${id(20)}","op":"UPSERT","asset":$asset}]}"""
        val cache = ReferenceCache.apply(null, request, ReferenceFeedResponse.parse(raw.toByteArray(), request, null), EnterpriseDeviceClock("boot-1", 1_000, System.currentTimeMillis()))
        MyAssetsViewState(generation = 9, epoch = 0, context = context, loaded = true, snapshot = MyAssets.present(cache, EnterpriseDeviceClock("boot-1", 2_000, System.currentTimeMillis())))
    }
    private class Calls { val saved = mutableListOf<Pair<ExpenseForm, ReceiptDraft?>>(); var sends = 0 }

    private fun render(initial: ExpenseViewState, connected: Boolean = true): Pair<Calls, (ExpenseViewState) -> Unit> {
        val calls = Calls()
        var current by mutableStateOf(initial)
        val app = InstrumentationRegistry.getInstrumentation().targetContext
        val configuration = Configuration(app.resources.configuration).apply { setLocale(Locale.forLanguageTag("pt-BR")) }
        val localized = app.createConfigurationContext(configuration)
        compose.setContent {
            CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                MaterialTheme {
                    ExpensePanel(current, connected, assets, {}, {}, {}, {}, { id, warning -> MyAssets.choose(partition.organizationId, assets.snapshot!!, id, warning) },
                        { _, form, draft -> calls.saved += form to draft }, { calls.sends++ })
                }
            }
        }
        return calls to { next -> current = next }
    }

    @Test fun anOfflineExpenseIsSavedNotAcceptedAndSendingNeedsAConnection() {
        val (calls, _) = render(state(), connected = false)
        compose.onNodeWithTag("enterprise-expense-offline").assertExists()
        compose.onNodeWithTag("enterprise-expense-sync", useUnmergedTree = true).onChildren().assertAny(hasText("Salvo neste aparelho", substring = true))
        compose.onNodeWithTag("enterprise-expense-review").assertTextEquals("Revisão: ainda não enviada")
        compose.onAllNodesWithText("Aceito pela empresa", substring = true).assertCountEquals(0)
        compose.onAllNodesWithText("Aprovad", substring = true).assertCountEquals(0)
        compose.onNodeWithTag("enterprise-expense-detail-paid").assertTextEquals("Valor pago: R$ 12,50")
        compose.onNodeWithTag("enterprise-expense-detail-company").assertTextEquals("Valor para a empresa: R$ 10,00")
        compose.onNodeWithTag("enterprise-expense-send").assertIsNotEnabled()
        assertEquals(0, calls.sends)
    }

    @Test fun acceptanceFollowsTheReceiptRefusalSaysWhyAndTheReceiptPhotoKeepsItsOwnProgress() {
        val (_, update) = render(state(ack = ack(), receipt = receipt(AttachmentState.DECLARED, accepted = true)))
        compose.onNodeWithTag("enterprise-expense-sync", useUnmergedTree = true).onChildren().assertAny(hasText("Aceito pela empresa", substring = true))
        compose.onNodeWithTag("enterprise-expense-review").assertTextEquals("Revisão: aguardando aprovação")
        compose.onNodeWithTag("enterprise-expense-receipt-0", useUnmergedTree = true).onChildren().assertAny(hasText("Foto 1: Enviando"))
        compose.onAllNodesWithText("Arquivo guardado pela empresa", substring = true).assertCountEquals(0)
        compose.onAllNodesWithText("válid", substring = true, ignoreCase = true).assertCountEquals(0)
        update(state(ack = ack("REJECTED", "FORBIDDEN")))
        compose.onNodeWithTag("enterprise-expense-review").assertTextEquals("Revisão: não aceita")
        compose.onAllNodesWithText("Aceito pela empresa", substring = true).assertCountEquals(0)
    }

    @Test fun theFormNeedsEveryFieldSavesExactEntriesAndRefusalsSaveNothing() {
        val (calls, update) = render(state(open = false))
        compose.onNodeWithTag("enterprise-expense-new").performClick()
        compose.onNodeWithTag("enterprise-asset-picker-row-${id(20)}").performClick()
        compose.onNodeWithTag("enterprise-expense-chosen").assertTextContains("Caminhão sintético", substring = true)
        compose.onNodeWithTag("enterprise-expense-save").performScrollTo().assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-expense-category-FUEL").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-expense-category-TOLL").performScrollTo().performClick()
        compose.onNodeWithTag("enterprise-expense-paid").performScrollTo().performTextInput("12,50")
        compose.onNodeWithTag("enterprise-expense-save").performScrollTo().assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-expense-company").performScrollTo().performTextInput("10")
        // No payer is assumed.
        compose.onNodeWithTag("enterprise-expense-save").performScrollTo().assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-expense-payer-EMPLOYEE").performScrollTo().performClick()
        compose.onNodeWithTag("enterprise-expense-save").performScrollTo().assertIsEnabled().performClick()
        val (form, draft) = calls.saved.single()
        assertEquals(listOf("TOLL", "12,50", "10"), listOf(form.category, form.paidTotal, form.companyAmount))
        assertEquals(ExpensePayer.EMPLOYEE, form.payer)
        assertNull(draft)
        update(state(open = false, captureInvalid = true))
        compose.onNodeWithTag("enterprise-expense-feedback").assertTextContains("Nada foi salvo", substring = true)
        update(state(open = false, saveFailed = true))
        compose.onNodeWithTag("enterprise-expense-feedback").assertTextEquals("Nada foi salvo. Tente de novo.")
    }
}
