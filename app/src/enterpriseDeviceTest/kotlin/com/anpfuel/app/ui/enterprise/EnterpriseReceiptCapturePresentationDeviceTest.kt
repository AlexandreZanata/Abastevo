package com.anpfuel.app.ui.enterprise

import android.content.res.Configuration
import android.graphics.Bitmap
import android.net.Uri
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.core.content.FileProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.ui.enterprise.attachment.ReceiptCaptureUi
import com.anpfuel.app.ui.enterprise.attachment.ReceiptPickers
import com.anpfuel.app.ui.enterprise.trip.TripPanel
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.attachment.ReceiptCaptureController
import com.anpfuel.application.enterprise.attachment.ReceiptDraft
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.application.enterprise.trip.TripViewState
import com.anpfuel.data.enterprise.attachment.AndroidReceiptOriginals
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.attachment.*
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.trip.*
import java.io.File
import java.time.Instant
import java.util.Locale
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * SCR-DOC-01 on a trip reading (EMP-DOC-004, B-BR-EMP-450..454) with the real capture controller over the real Android
 * originals (the validation app's FileProvider camera file and the device decoder). Only the system camera and photo
 * picker activities are replaced: the camera stand-in writes a device-encoded synthetic JPEG into the app's own camera
 * target exactly as the system camera would; the picker stand-in returns a content reference.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseReceiptCapturePresentationDeviceTest {
    @get:Rule val compose = createComposeRule()
    private val app get() = InstrumentationRegistry.getInstrumentation().targetContext
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val partition = EnterprisePartition.of(id(1), "https://example.invalid", id(2))
    private val context = EnterpriseContextSummary(partition, id(3), "Synthetic company A", id(5))
    private val binding = SyncBinding.of(id(1), "https://example.invalid", id(2))
    private val start = TripOperations.start(TripEnvelope(binding, id(3), id(4), id(100), TimeDeclaration(EnterpriseTime.parseInstant("2026-11-15T12:00:00Z"), "UTC", 0)),
        id(10), id(20), id(5), TripReadingInput(id(60), id(61), id(62), "ODOMETER", "184320000"), null, null)
    private val trip = TripBook.empty(id(3), id(4)).withStart(LocalTrip(id(10), id(20), TripAssetLabel("Caminhão sintético", "FROTA-12", "ABC1D23"), false,
        id(100), EnterpriseTime.parseInstant("2026-11-15T12:00:00Z"))).trip(id(10))!!
    private val target = AttachmentTarget(DocumentTargetKind.METER_READING, id(62))
    private fun ack(op: EnterpriseOperation) = OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${op.operationId}",""" +
        """"payload_digest":"${op.payloadDigest}","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-15T16:00:00Z",""" +
        """"review_item_id":null,"warnings":[]}""").toByteArray(), op)
    private fun view(accepted: Boolean = false) = TripPresentation.view(trip, mapOf(start.operationId to EnterpriseQueueEntry(start, acknowledgement = if (accepted) ack(start) else null)))
    private fun state(accepted: Boolean = false, photos: List<AttachmentView> = emptyList(), attached: String? = null, saveFailed: Boolean = false) =
        TripViewState(generation = 3, epoch = 0, context = context, loaded = true, trips = listOf(view(accepted)), openTripId = id(10),
            attachments = if (photos.isEmpty()) emptyMap() else mapOf(start.operationId to photos), lastAttached = attached, saveFailed = saveFailed)
    private val originals by lazy { AndroidReceiptOriginals(app) }
    private val cameraFolder get() = File(app.cacheDir, AndroidReceiptOriginals.DIRECTORY)

    private class Calls { val attached = mutableListOf<Triple<String, AttachmentTarget, ReceiptDraft>>() }

    /** The system camera stand-in: it writes into the app's own private camera file, then returns success. */
    private class Camera(val controller: ReceiptCaptureController, val bytes: () -> ByteArray?, val denied: Boolean = false)

    private fun jpeg(dark: Boolean = false): ByteArray {
        val width = 600
        val height = 900
        val pixels = IntArray(width * height) { index ->
            val x = index % width
            val y = index / width
            val v = if (y % 24 < 8 && x % 48 < 36 && x in 40 until 560) (if (dark) 8 else 25) else (if (dark) 38 else 235)
            android.graphics.Color.rgb(v, v, v)
        }
        val bitmap = Bitmap.createBitmap(pixels, width, height, Bitmap.Config.ARGB_8888)
        return java.io.ByteArrayOutputStream().use { out -> check(bitmap.compress(Bitmap.CompressFormat.JPEG, 92, out)); out.toByteArray() }
            .also { bitmap.recycle() }
    }
    private fun png(): ByteArray {
        val bitmap = Bitmap.createBitmap(48, 48, Bitmap.Config.ARGB_8888).apply { eraseColor(android.graphics.Color.WHITE) }
        return java.io.ByteArrayOutputStream().use { out -> check(bitmap.compress(Bitmap.CompressFormat.PNG, 100, out)); out.toByteArray() }
            .also { bitmap.recycle() }
    }
    /** A picker-like content reference to a file outside the camera folder (the app never deletes it). */
    private fun imported(name: String, bytes: ByteArray?): String {
        val file = File(File(app.cacheDir, "capture").apply { mkdirs() }, name)
        if (bytes == null) file.delete() else file.writeBytes(bytes)
        return FileProvider.getUriForFile(app, app.packageName + ".fileprovider", file).toString()
    }

    @After fun clean() {
        File(app.cacheDir, "capture").listFiles()?.filter { it.name.startsWith("price_doc004") }?.forEach { it.delete() }
        cameraFolder.listFiles()?.forEach { it.delete() }
    }

    private fun render(initial: TripViewState, camera: () -> ByteArray?, library: () -> String?, deniedCamera: Boolean = false,
        enabled: Boolean = true): Pair<Calls, (TripViewState) -> Unit> {
        val calls = Calls()
        val controller = ReceiptCaptureController(originals)
        var current by mutableStateOf(initial)
        val configuration = Configuration(app.resources.configuration).apply { setLocale(Locale.forLanguageTag("pt-BR")) }
        val localized = app.createConfigurationContext(configuration)
        val pickers = object : ReceiptPickers {
            override fun camera() {
                if (deniedCamera) { controller.cameraDenied(); return }
                runBlocking {
                    val reference = controller.cameraTarget()
                    val bytes = camera()
                    if (bytes != null) app.contentResolver.openOutputStream(Uri.parse(reference))!!.use { it.write(bytes) }
                    controller.cameraResult(reference, bytes != null)
                }
            }
            override fun library() { runBlocking { controller.imported(library()) } }
        }
        compose.setContent {
            val receipt by controller.state.collectAsState()
            CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                MaterialTheme {
                    TripPanel(current, false, MyAssetsViewState(), {}, {}, {}, {}, { _, _ -> null }, { _, _, _ -> }, { _, _, _ -> },
                        { _, _ -> }, { _, _ -> emptyList() }, {}, {},
                        capture = if (enabled) ReceiptCaptureUi(receipt, pickers, controller::reset) else null,
                        onAttach = { owner, target, draft -> calls.attached += Triple(owner, target, draft) })
                }
            }
        }
        return calls to { next -> current = next }
    }
    private fun scrollTo(tag: String) = compose.onNodeWithTag("enterprise-trip-panel").performScrollToNode(hasTestTag(tag))

    @Test fun aCameraPhotoShowsItsHintAndIsSavedOnlyAfterTheFlowSaysSo() {
        val photo = jpeg()
        val (calls, update) = render(state(), camera = { photo }, library = { null })
        scrollTo("enterprise-trip-add-photo-start")
        compose.onNodeWithTag("enterprise-trip-add-photo-start").performClick()
        compose.onNodeWithText("Fase de teste", substring = true).assertExists()
        compose.onNodeWithTag("enterprise-receipt-save").assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-receipt-camera").performClick()
        compose.waitUntil(10_000) { compose.onAllNodesWithTag("enterprise-receipt-preview").fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("enterprise-receipt-hint", useUnmergedTree = true).onChildren().assertAny(hasText("A imagem parece nítida."))
        compose.onNodeWithTag("enterprise-receipt-hint", useUnmergedTree = true).onChildren()
            .assertAny(hasText("Esta conferência é só uma dica; ela não valida o comprovante."))
        // The camera's private file is deleted as soon as its bytes are read (EDR-291).
        assertTrue(cameraFolder.listFiles().orEmpty().isEmpty())
        // Nothing is announced as saved before the record's flow reports the protected write.
        compose.onAllNodesWithText("Foto salva no aparelho.").assertCountEquals(0)
        compose.onNodeWithTag("enterprise-receipt-save").assertIsEnabled().performClick()
        val (owner, chosen, draft) = calls.attached.single()
        assertEquals(start.operationId, owner)
        assertEquals(target, chosen)
        assertArrayEquals("The original must be saved unmodified", photo, draft.original.bytes())
        assertEquals(ReceiptSource.CAMERA, draft.original.source)
        val saved = EnterpriseAttachment(id(700), id(701), id(702), start.operationId, target, draft.original.sha256, draft.original.size,
            Instant.parse("2026-11-15T12:05:00Z"))
        update(state(photos = listOf(AttachmentPresentation.view(saved, null)), attached = start.operationId))
        compose.waitUntil(5_000) { compose.onAllNodesWithTag("enterprise-receipt-capture").fetchSemanticsNodes().isEmpty() }
        compose.onNodeWithTag("enterprise-trip-photo-saved").assertTextEquals("Foto salva no aparelho.")
        scrollTo("enterprise-trip-photo-start-0")
        compose.onNodeWithTag("enterprise-trip-photo-start-0", useUnmergedTree = true).onChildren().assertAny(hasText("Foto 1: Salvo no aparelho"))
    }

    @Test fun deniedCameraMissingAndUnsupportedFilesSaveNothing() {
        var pick: String? = null
        val (calls, _) = render(state(), camera = { null }, library = { pick }, deniedCamera = true)
        scrollTo("enterprise-trip-add-photo-start")
        compose.onNodeWithTag("enterprise-trip-add-photo-start").performClick()
        compose.onNodeWithTag("enterprise-receipt-camera").performClick()
        compose.onNodeWithTag("enterprise-receipt-message").assertTextEquals("Permissão da câmera negada. Você pode importar uma foto. Nada foi salvo.")
        pick = imported("price_doc004_import.png", png())
        compose.onNodeWithTag("enterprise-receipt-import").performClick()
        compose.waitUntil(5_000) { compose.onAllNodesWithText("Formato não aceito", substring = true).fetchSemanticsNodes().isNotEmpty() }
        compose.onAllNodesWithTag("enterprise-receipt-preview").assertCountEquals(0)
        pick = imported("price_doc004_gone.jpg", null)
        compose.onNodeWithTag("enterprise-receipt-import").performClick()
        compose.waitUntil(5_000) { compose.onAllNodesWithText("Arquivo não encontrado no aparelho", substring = true).fetchSemanticsNodes().isNotEmpty() }
        pick = null
        compose.onNodeWithTag("enterprise-receipt-import").performClick()
        compose.waitUntil(5_000) { compose.onAllNodesWithText("Nenhuma foto foi escolhida", substring = true).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("enterprise-receipt-save").assertIsNotEnabled()
        assertTrue(calls.attached.isEmpty())
        // The picker's file is never deleted by the app.
        assertTrue(File(File(app.cacheDir, "capture"), "price_doc004_import.png").exists())
    }

    @Test fun aDarkImportIsHintedKeptUnmodifiedAndAFailedSaveKeepsIt() {
        val photo = jpeg(dark = true)
        val reference = imported("price_doc004_dark.jpg", photo)
        val (calls, update) = render(state(), camera = { null }, library = { reference })
        scrollTo("enterprise-trip-add-photo-start")
        compose.onNodeWithTag("enterprise-trip-add-photo-start").performClick()
        compose.onNodeWithTag("enterprise-receipt-import").performClick()
        compose.waitUntil(10_000) { compose.onAllNodesWithTag("enterprise-receipt-preview").fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("enterprise-receipt-hint", useUnmergedTree = true).onChildren().assertAny(hasText("A imagem pode estar escura."))
        // The hint never blocks: the person may keep the photo.
        compose.onNodeWithTag("enterprise-receipt-save").assertIsEnabled().performClick()
        assertArrayEquals(photo, calls.attached.single().third.original.bytes())
        update(state(saveFailed = true))
        compose.onNodeWithTag("enterprise-trip-feedback").assertTextEquals("Nada foi salvo. Tente de novo.")
        compose.onNodeWithTag("enterprise-receipt-preview").assertExists()
        compose.onNodeWithTag("enterprise-receipt-save").assertIsEnabled()
    }

    @Test fun eachPhotoShowsItsOwnStateIndependentOfTheAcceptedRecord() {
        fun item(n: Int, state: AttachmentState, refusal: AttachmentRefusal? = null) = EnterpriseAttachment(id(800 + n * 3), id(801 + n * 3),
            id(802 + n * 3), start.operationId, target, "ab".repeat(32), 1024, Instant.parse("2026-11-15T12:05:00Z"), state, refusal,
            if (state == AttachmentState.WAITING_FOR_RECORD) null else 1)
        val receipt = ack(start)
        val photos = listOf(item(0, AttachmentState.DECLARED), item(1, AttachmentState.STORED), item(2, AttachmentState.FILE_MISSING),
            item(3, AttachmentState.REFUSED, AttachmentRefusal.FORBIDDEN)).map { AttachmentPresentation.view(it, receipt) }
        render(state(accepted = true, photos = photos), camera = { null }, library = { null })
        scrollTo("enterprise-trip-sync-start")
        compose.onNodeWithTag("enterprise-trip-sync-start", useUnmergedTree = true).onChildren().assertAny(hasText("Aceito pela empresa", substring = true))
        val expected = listOf("Foto 1: Enviando", "Foto 2: Arquivo guardado pela empresa",
            "Foto 3: Comprovante não encontrado no aparelho. Fotografe de novo.", "Foto 4: Não aceito")
        expected.forEachIndexed { index, text ->
            scrollTo("enterprise-trip-photo-start-$index")
            compose.onNodeWithTag("enterprise-trip-photo-start-$index", useUnmergedTree = true).onChildren().assertAny(hasText(text))
        }
        compose.onNodeWithTag("enterprise-trip-photo-start-3", useUnmergedTree = true).onChildren().assertAny(hasText("Motivo: não permitido para este registro"))
        // A missing original offers a new capture; nothing claims a receipt is valid.
        scrollTo("enterprise-trip-photo-start-2-retake")
        compose.onNodeWithTag("enterprise-trip-photo-start-2-retake").performClick()
        compose.onNodeWithTag("enterprise-receipt-capture").assertExists()
        compose.onAllNodesWithText("válido", substring = true).assertCountEquals(0)
    }

    @Test fun captureIsAbsentWhenSwitchedOff() {
        render(state(), camera = { null }, library = { null }, enabled = false)
        scrollTo("enterprise-trip-record-start")
        compose.onAllNodesWithTag("enterprise-trip-add-photo-start").assertCountEquals(0)
    }
}
