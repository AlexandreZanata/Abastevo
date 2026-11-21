package com.anpfuel.app.ui.enterprise

import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
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
import com.anpfuel.app.ui.enterprise.attachment.ReceiptCaptureUi
import com.anpfuel.app.ui.enterprise.attachment.ReceiptPickers
import com.anpfuel.app.ui.enterprise.fueling.FuelingPanel
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.attachment.ReceiptCaptureState
import com.anpfuel.application.enterprise.attachment.ReceiptDraft
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.application.enterprise.fueling.FuelingForm
import com.anpfuel.application.enterprise.fueling.FuelingViewState
import com.anpfuel.application.enterprise.fueling.FuelingWarnings
import com.anpfuel.data.local.ocr.MlKitImagePriceOcr
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.attachment.*
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.fleet.*
import com.anpfuel.domain.enterprise.fueling.*
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import java.io.ByteArrayOutputStream
import java.time.Instant
import java.util.Locale
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** SCR-FUEL-01/02 presentation (EMP-FUEL-011) over real fueling views built from real operations and receipts. */
@RunWith(AndroidJUnit4::class)
class EnterpriseFuelingPresentationDeviceTest {
    @get:Rule val compose = createComposeRule()
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val partition = EnterprisePartition.of(id(1), "https://example.invalid", id(2))
    private val context = EnterpriseContextSummary(partition, id(3), "Synthetic company A", id(5))
    private val operation: EnterpriseOperation = FuelingOperations.submit(FuelingEnvelope(SyncBinding.of(id(1), "https://example.invalid", id(2)), id(3), id(4),
        id(100), TimeDeclaration(EnterpriseTime.parseInstant("2026-11-15T12:00:00Z"), "UTC", 0)), id(110),
        FuelingSubmitInput(id(20), id(5), "DIESEL_S10", "L", "150000", "97000", null, "6490", null, "FULL", "COMPANY", "FUEL_CARD",
            FuelingReadingInput(id(60), id(61), id(62), "ODOMETER", "184600000")))
    private fun ack() = OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}",""" +
        """"result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-15T16:00:00Z","review_item_id":null,"warnings":[]}""").toByteArray(), operation)
    private fun view(accepted: Boolean) = FuelingPresentation.view(EnterpriseQueueEntry(operation, acknowledgement = if (accepted) ack() else null))
    private fun receipt(state: AttachmentState, accepted: Boolean) = AttachmentPresentation.view(EnterpriseAttachment(id(700), id(701), id(702), operation.operationId,
        AttachmentTarget(DocumentTargetKind.FUELING, id(110)), "ab".repeat(32), 2048, Instant.parse("2026-11-15T12:01:00Z"), state, null,
        if (state == AttachmentState.WAITING_FOR_RECORD) null else 1), if (accepted) ack() else null)
    private fun state(accepted: Boolean = false, receipt: AttachmentView? = null, open: Boolean = true, saveFailed: Boolean = false) =
        FuelingViewState(generation = 3, epoch = 0, context = context, loaded = true, fuelings = listOf(view(accepted)), openFuelingId = if (open) id(110) else null,
            attachments = receipt?.let { mapOf(operation.operationId to listOf(it)) } ?: emptyMap(), saveFailed = saveFailed)
    private val assets = run {
        val request = ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(id(70), id(71), "synthetic-reference-access", id(3), id(4), null))
        val asset = """{"id":"${id(20)}","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"Caminhão sintético","fleet_number":"FROTA-12","plate":"ABC1D23","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[{"id":"${id(60)}","kind":"ODOMETER","modulus":null,"segment_id":"${id(61)}","installed_at":"2026-11-01T00:00:00Z"}],"assignment":{"kind":"PRIMARY","valid_to":null}}"""
        val raw = """{"schema_version":1,"request_id":"${id(70)}","membership_id":"${id(3)}","enrollment_id":"${id(4)}","mode":"RESET","reason":"NEW_CURSOR","from_revision":null,"revision":1,"confirmed_at":"2026-11-15T12:00:00Z","cursor":"c-1","entity_types":["asset"],"changes":[{"revision":1,"entity_type":"asset","entity_id":"${id(20)}","op":"UPSERT","asset":$asset}]}"""
        val cache = ReferenceCache.apply(null, request, ReferenceFeedResponse.parse(raw.toByteArray(), request, null), EnterpriseDeviceClock("boot-1", 1_000, System.currentTimeMillis()))
        MyAssetsViewState(generation = 9, epoch = 0, context = context, loaded = true, snapshot = MyAssets.present(cache, EnterpriseDeviceClock("boot-1", 2_000, System.currentTimeMillis())))
    }
    private class Calls { val saved = mutableListOf<Pair<FuelingForm, ReceiptDraft?>>(); var sends = 0 }

    private fun render(initial: FuelingViewState, connected: Boolean = true,
        readMoney: (suspend (ReceiptDraft) -> ReceiptMoneyReading?)? = null): Pair<Calls, (FuelingViewState) -> Unit> {
        val calls = Calls()
        var current by mutableStateOf(initial)
        var draft by mutableStateOf<ReceiptDraft?>(null)
        val pickers = object : ReceiptPickers {
            override fun camera() = library()
            override fun library() {
                val bitmap = Bitmap.createBitmap(48, 48, Bitmap.Config.ARGB_8888)
                val bytes = ByteArrayOutputStream().use { output ->
                    try { check(bitmap.compress(Bitmap.CompressFormat.JPEG, 90, output)); output.toByteArray() }
                    finally { bitmap.recycle() }
                }
                val original = (ReceiptIntake.inspect(bytes, ReceiptSource.IMPORT) as ReceiptInspection.Accepted).original
                draft = ReceiptDraft(original, LegibilityHint.NOT_CHECKED)
            }
        }
        val app = InstrumentationRegistry.getInstrumentation().targetContext
        val configuration = Configuration(app.resources.configuration).apply { setLocale(Locale.forLanguageTag("pt-BR")) }
        val localized = app.createConfigurationContext(configuration)
        compose.setContent {
            CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                MaterialTheme {
                    FuelingPanel(current, connected, assets, {}, {}, {}, {}, { id, warning -> MyAssets.choose(partition.organizationId, assets.snapshot!!, id, warning) },
                        { _, form, draft -> calls.saved += form to draft },
                        { form -> FuelingWarnings(form.paidTotal == "970,00", emptyList()) }, { calls.sends++ },
                        capture = if (readMoney == null) null else ReceiptCaptureUi(ReceiptCaptureState(draft), pickers, { draft = null }, readMoney))
                }
            }
        }
        return calls to { next -> current = next }
    }

    @Test fun anOfflineFuelingIsSavedNotAcceptedAndSendingNeedsAConnection() {
        val (calls, _) = render(state(), connected = false)
        compose.onNodeWithTag("enterprise-fueling-offline").assertTextContains("Sem internet", substring = true)
        compose.onNodeWithTag("enterprise-fueling-sync", useUnmergedTree = true).onChildren().assertAny(hasText("Salvo neste aparelho", substring = true))
        compose.onNodeWithTag("enterprise-fueling-review").assertTextEquals("Revisão: ainda não enviado")
        compose.onAllNodesWithText("Aceito pela empresa", substring = true).assertCountEquals(0)
        compose.onNodeWithTag("enterprise-fueling-detail-paid").assertTextEquals("Total pago: R$ 970")
        compose.onNodeWithTag("enterprise-fueling-send").assertIsNotEnabled()
        assertEquals(0, calls.sends)
    }

    @Test fun acceptanceFollowsTheReceiptAndTheReceiptPhotoKeepsItsOwnProgress() {
        render(state(accepted = true, receipt = receipt(AttachmentState.DECLARED, accepted = true)))
        compose.onNodeWithTag("enterprise-fueling-sync", useUnmergedTree = true).onChildren().assertAny(hasText("Aceito pela empresa", substring = true))
        compose.onNodeWithTag("enterprise-fueling-review").assertTextEquals("Revisão: aguardando aprovação")
        compose.onNodeWithTag("enterprise-fueling-receipt-0", useUnmergedTree = true).onChildren().assertAny(hasText("Foto 1: Enviando"))
        compose.onAllNodesWithText("Arquivo guardado pela empresa", substring = true).assertCountEquals(0)
    }

    @Test fun aPlanChangeReceiptKeepsTheFuelingAndShowsReview() {
        val raw = """{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"ACCEPTED_FOR_REVIEW","code":"PLAN_CHANGED","entity_revision":null,"received_at":"2026-11-15T16:00:00Z","review_item_id":"${id(108)}","warnings":[]}"""
        val receipt = OperationAcknowledgement.parse(raw.toByteArray(), operation)
        val held = FuelingPresentation.view(EnterpriseQueueEntry(operation, acknowledgement = receipt))
        render(state().copy(fuelings = listOf(held)))

        compose.onNodeWithTag("enterprise-fueling-sync", useUnmergedTree = true).onChildren()
            .assertAny(hasText("O plano da empresa mudou antes do recebimento", substring = true))
        compose.onNodeWithTag("enterprise-fueling-review").assertTextEquals("Revisão: precisa de revisão")
        compose.onNodeWithTag("enterprise-fueling-detail-paid").assertTextEquals("Total pago: R$ 970")
        compose.onAllNodesWithText("Aceito pela empresa", substring = true).assertCountEquals(0)
    }

    @Test fun theFormSavesExactEntriesWithItsWarningAndAFailedSaveSaysSo() {
        val (calls, update) = render(state(open = false))
        compose.onNodeWithTag("enterprise-fueling-new").performClick()
        compose.onNodeWithTag("enterprise-asset-picker-row-${id(20)}").performClick()
        compose.onNodeWithTag("enterprise-fueling-chosen").assertTextContains("Caminhão sintético", substring = true)
        compose.onNodeWithTag("enterprise-fueling-save").performScrollTo().assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-fueling-quantity").performScrollTo().performTextInput("150")
        compose.onNodeWithTag("enterprise-fueling-paid").performScrollTo().performTextInput("970,00")
        compose.onNodeWithTag("enterprise-fueling-price").performScrollTo().performTextInput("6,490")
        compose.onNodeWithTag("enterprise-fueling-reading-value").performScrollTo().performTextInput("184600")
        compose.onNodeWithTag("enterprise-fueling-warning-arithmetic").performScrollTo().assertExists()
        compose.onNodeWithTag("enterprise-fueling-save").performScrollTo().assertIsEnabled().performClick()
        val (form, draft) = calls.saved.single()
        assertEquals(listOf("150", "970,00", "6,490"), listOf(form.quantity, form.paidTotal, form.unitPrice))
        assertEquals(FuelProduct.DIESEL_S10, form.product)
        assertNull(draft)
        update(state(open = false, saveFailed = true))
        compose.onNodeWithTag("enterprise-fueling-feedback").assertTextEquals("Nada foi salvo. Tente de novo.")
    }

    @Test fun unreadableReceiptKeepsManualValuesAndSuggestionsNeedSeparateConfirmation() {
        var reads = 0
        val (calls, _) = render(state(open = false), readMoney = {
            reads += 1
            if (reads == 1) ReceiptMoneyReading.read(sequenceOf(ReceiptTextLine("unreadable", 0.15f)))
            else ReceiptMoneyReading.read(sequenceOf(ReceiptTextLine("Total a pagar: R$ 97,00", 0.60f), ReceiptTextLine("Desconto: 5,00", null)))
        })
        compose.onNodeWithTag("enterprise-fueling-new").performClick()
        compose.onNodeWithTag("enterprise-asset-picker-row-${id(20)}").performClick()
        compose.onNodeWithTag("enterprise-fueling-quantity").performScrollTo().performTextInput("15")
        compose.onNodeWithTag("enterprise-fueling-paid").performScrollTo().performTextInput("90,00")
        compose.onNodeWithTag("enterprise-fueling-discount").performScrollTo().performTextInput("1,00")
        compose.onNodeWithTag("enterprise-fueling-reading-value").performScrollTo().performTextInput("184600")
        compose.onNodeWithTag("enterprise-receipt-import").performScrollTo().performClick()
        compose.runOnIdle { assertEquals(0, reads) }
        val scan = compose.onNodeWithTag("enterprise-receipt-read-money")
        scan.performScrollTo().performClick()
        compose.onNodeWithTag("enterprise-receipt-money-manual").assertExists()
        compose.onNodeWithTag("enterprise-fueling-paid").performScrollTo().assertTextContains("90,00")

        scan.performScrollTo().performClick()
        compose.onNodeWithText("Confiança da leitura: 60%").assertExists()
        compose.onNodeWithText("Confiança da leitura indisponível").assertExists()
        compose.onNodeWithTag("enterprise-fueling-paid").performScrollTo().assertTextContains("90,00")
        compose.onNodeWithTag("enterprise-receipt-use-paid").performScrollTo().performClick()
        compose.onNodeWithTag("enterprise-fueling-paid").performScrollTo().assertTextContains("97,00")
        compose.onNodeWithTag("enterprise-fueling-discount").performScrollTo().assertTextContains("1,00")
        compose.onNodeWithTag("enterprise-receipt-use-discount").performScrollTo().performClick()
        compose.onNodeWithTag("enterprise-fueling-save").performScrollTo().performClick()
        val (form, draft) = compose.runOnIdle { calls.saved.single() }
        assertEquals("97,00", form.paidTotal)
        assertEquals("5,00", form.discount)
        assertNotNull(draft)
    }

    @Test fun theLocalModelReadsMoneyFromSyntheticPixels(): Unit = runBlocking {
        assumeTrue(InstrumentationRegistry.getArguments().getString("enterpriseLocalReceiptModel") == "true")
        val app = InstrumentationRegistry.getInstrumentation().targetContext
        val recognizer = MlKitImagePriceOcr(app)
        assertTrue("The Android device must have the local Latin model for this check", recognizer.prepareModel())
        val bitmap = Bitmap.createBitmap(1200, 400, Bitmap.Config.ARGB_8888)
        val canvas = Canvas(bitmap)
        val ink = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.BLACK; textSize = 64f }
        canvas.drawColor(Color.WHITE)
        canvas.drawText("TOTAL A PAGAR: R$ 97,00", 45f, 150f, ink)
        canvas.drawText("DESCONTO: R$ 5,00", 45f, 260f, ink)
        val bytes = ByteArrayOutputStream().use { output ->
            try { check(bitmap.compress(Bitmap.CompressFormat.JPEG, 95, output)); output.toByteArray() }
            finally { bitmap.recycle() }
        }
        val original = (ReceiptIntake.inspect(bytes, ReceiptSource.IMPORT) as ReceiptInspection.Accepted).original
        val result = recognizer.recognizeReceipt(original)
        assertEquals("97,00", result.paid?.decimal)
        assertEquals("5,00", result.discount?.decimal)
    }
}
