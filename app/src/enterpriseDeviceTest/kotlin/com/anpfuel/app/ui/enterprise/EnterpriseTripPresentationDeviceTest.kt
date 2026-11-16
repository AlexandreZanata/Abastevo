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
import com.anpfuel.app.ui.enterprise.trip.TripPanel
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripViewState
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.*
import com.anpfuel.domain.enterprise.meter.ReadingFlag
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.trip.*
import com.anpfuel.domain.enterprise.attachment.AttachmentPresentation
import com.anpfuel.domain.enterprise.attachment.AttachmentState
import com.anpfuel.domain.enterprise.attachment.AttachmentTarget
import com.anpfuel.domain.enterprise.attachment.EnterpriseAttachment
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.expense.ExpenseEnvelope
import com.anpfuel.domain.enterprise.expense.ExpenseOperations
import com.anpfuel.domain.enterprise.expense.ExpensePresentation
import com.anpfuel.domain.enterprise.fueling.FuelingEnvelope
import com.anpfuel.domain.enterprise.fueling.FuelingOperations
import com.anpfuel.domain.enterprise.fueling.FuelingPresentation
import com.anpfuel.domain.enterprise.fueling.FuelingSubmitInput
import java.util.Locale
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** SCR-TRIP-01 presentation (EMP-TRIP-005) over real trip views built from real operations and receipts; no producer runs here. */
@RunWith(AndroidJUnit4::class)
class EnterpriseTripPresentationDeviceTest {
    @get:Rule val compose = createComposeRule()
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val partition = EnterprisePartition.of(id(1), "https://example.invalid", id(2))
    private val context = EnterpriseContextSummary(partition, id(3), "Synthetic company A", id(5))
    private val binding = SyncBinding.of(id(1), "https://example.invalid", id(2))
    private fun envelope(op: Int, at: String) = TripEnvelope(binding, id(3), id(4), id(op), TimeDeclaration(EnterpriseTime.parseInstant(at), "UTC", 0))
    private val start = TripOperations.start(envelope(100, "2026-11-15T12:00:00Z"), id(10), id(20), id(5),
        TripReadingInput(id(60), id(61), id(62), "ODOMETER", "184320000"), null, TripClockReading("boot-1", 1_000))
    private val stop = TripOperations.event(envelope(200, "2026-11-15T13:00:00Z"), id(210), id(10), "STOP", "Pátio do cliente", id(100))
    private val note = TripOperations.event(envelope(201, "2026-11-15T12:30:00Z"), id(211), id(10), "NOTE", "Pneu calibrado", id(100))
    private val finish = TripOperations.finish(envelope(300, "2026-11-15T15:00:00Z"), id(10), TripReadingInput(id(60), id(61), id(63), "ODOMETER", "184610000"),
        null, TripClockReading("boot-1", 10_801_000), id(100))
    private val submit = TripOperations.submit(envelope(400, "2026-11-15T15:01:00Z"), id(10), 1, id(300))
    private val label = TripAssetLabel("Caminhão sintético", "FROTA-12", "ABC1D23")
    private fun ack(op: EnterpriseOperation, result: String = "ACCEPTED", code: String? = null, revision: Long? = 1) = OperationAcknowledgement.parse(
        ("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"$result",""" +
            """"code":${code?.let { "\"$it\"" } ?: "null"},"entity_revision":${revision ?: "null"},"received_at":"2026-11-15T16:00:00Z",""" +
            """"review_item_id":${if (result == "ACCEPTED_FOR_REVIEW") "\"${id(999)}\"" else "null"},"warnings":[]}""").toByteArray(), op)
    private fun book(finished: Boolean = true, submitted: Boolean = true): LocalTrip {
        var book = TripBook.empty(id(3), id(4)).withStart(LocalTrip(id(10), id(20), label, false, id(100), EnterpriseTime.parseInstant("2026-11-15T12:00:00Z")))
            .withEvent(id(10), id(200), EnterpriseTime.parseInstant("2026-11-15T13:00:00Z")).withEvent(id(10), id(201), EnterpriseTime.parseInstant("2026-11-15T12:30:00Z"))
        if (finished) book = book.withFinish(id(10), id(300), EnterpriseTime.parseInstant("2026-11-15T15:00:00Z"))
        if (submitted) book = book.withSubmission(id(10), id(400))
        return book.trip(id(10))!!
    }
    private fun view(trip: LocalTrip, acks: Map<EnterpriseOperation, OperationAcknowledgement> = emptyMap()): TripView =
        TripPresentation.view(trip, listOf(start, stop, note, finish, submit).filter { it.operationId in trip.operationIds }
            .associate { it.operationId to EnterpriseQueueEntry(it, acknowledgement = acks[it]) })
    private fun state(vararg trips: TripView, open: String? = null, connected: Boolean = true, refusal: TripLocalFailure? = null,
        saveFailed: Boolean = false, saved: String? = null) =
        TripViewState(generation = 3, epoch = 0, context = context, loaded = true, trips = trips.toList(), openTripId = open,
            refusal = refusal, saveFailed = saveFailed, lastSaved = saved)
    private val assets = run {
        val request = ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(id(70), id(71), "synthetic-reference-access", id(3), id(4), null))
        val asset = """{"id":"${id(20)}","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"Caminhão sintético","fleet_number":"FROTA-12","plate":"ABC1D23","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[{"id":"${id(60)}","kind":"ODOMETER","modulus":null,"segment_id":"${id(61)}","installed_at":"2026-11-01T00:00:00Z"}],"assignment":{"kind":"PRIMARY","valid_to":null}}"""
        val raw = """{"schema_version":1,"request_id":"${id(70)}","membership_id":"${id(3)}","enrollment_id":"${id(4)}","mode":"RESET","reason":"NEW_CURSOR","from_revision":null,"revision":1,"confirmed_at":"2026-11-15T12:00:00Z","cursor":"c-1","entity_types":["asset"],"changes":[{"revision":1,"entity_type":"asset","entity_id":"${id(20)}","op":"UPSERT","asset":$asset}]}"""
        val cache = ReferenceCache.apply(null, request, ReferenceFeedResponse.parse(raw.toByteArray(), request, null),
            EnterpriseDeviceClock("boot-1", 1_000, System.currentTimeMillis()))
        MyAssetsViewState(generation = 9, epoch = 0, context = context, loaded = true, snapshot = MyAssets.present(cache, EnterpriseDeviceClock("boot-1", 2_000, System.currentTimeMillis())))
    }

    private class Calls {
        val starts = mutableListOf<Pair<EnterpriseAssetChoice, TripCapture>>()
        val logs = mutableListOf<Triple<String, TripLogbookType, String?>>()
        val finishes = mutableListOf<TripCapture>()
        val ready = mutableListOf<String>()
        var sends = 0
        val opened = mutableListOf<String?>()
    }

    private fun render(initial: TripViewState, connected: Boolean = true, warnings: List<ReadingFlag> = emptyList()): Pair<Calls, (TripViewState) -> Unit> {
        val calls = Calls()
        var current by mutableStateOf(initial)
        val application = InstrumentationRegistry.getInstrumentation().targetContext
        val configuration = Configuration(application.resources.configuration).apply { setLocale(Locale.forLanguageTag("pt-BR")) }
        val localized = application.createConfigurationContext(configuration)
        compose.setContent {
            CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                MaterialTheme {
                    TripPanel(current, connected, assets, {}, {}, { calls.opened += it }, {},
                        { id, warning -> MyAssets.choose(partition.organizationId, assets.snapshot!!, id, warning) },
                        { choice, _, capture -> calls.starts += choice to capture }, { trip, type, text -> calls.logs += Triple(trip, type, text) },
                        { _, capture -> calls.finishes += capture }, { _, _ -> warnings }, { calls.ready += it }, { calls.sends++ })
                }
            }
        }
        return calls to { next -> current = next }
    }
    private fun scrollTo(tag: String) = compose.onNodeWithTag("enterprise-trip-panel").performScrollToNode(hasTestTag(tag))

    @Test fun offlineSavedRecordsAreNeverShownAsAcceptedAndSendingNeedsAConnection() {
        val (calls, _) = render(state(view(book()), open = id(10)), connected = false)
        compose.onNodeWithTag("enterprise-trip-offline").assertTextContains("Sem internet", substring = true)
        compose.onNodeWithTag("enterprise-trip-operational").assertTextEquals("Encerrada no aparelho")
        compose.onNodeWithTag("enterprise-trip-review").assertTextEquals("Revisão: pronta para enviar")
        for (tag in listOf("start", "event-${id(210)}", "event-${id(211)}", "finish", "submission")) {
            scrollTo("enterprise-trip-sync-$tag")
            compose.onNodeWithTag("enterprise-trip-sync-$tag", useUnmergedTree = true).onChildren()
                .assertAny(hasText("Salvo neste aparelho", substring = true))
            compose.onAllNodesWithText("Aceito pela empresa", substring = true).assertCountEquals(0)
        }
        // Occurrence order: the 12:30 note before the 13:00 stop, whatever the creation order.
        val noteTop = compose.onNodeWithTag("enterprise-trip-record-event-${id(211)}").fetchSemanticsNode().positionInRoot.y
        val stopTop = compose.onNodeWithTag("enterprise-trip-record-event-${id(210)}").fetchSemanticsNode().positionInRoot.y
        assertTrue(noteTop < stopTop)
        scrollTo("enterprise-trip-send")
        compose.onNodeWithTag("enterprise-trip-send").assertIsNotEnabled()
        assertEquals(0, calls.sends)
    }

    @Test fun eachRecordChangesOnlyWithItsOwnReceiptAndApprovalIsNeverClaimed() {
        val trip = book()
        val (calls, update) = render(state(view(trip, mapOf(start to ack(start))), open = id(10)))
        scrollTo("enterprise-trip-sync-start")
        compose.onNodeWithTag("enterprise-trip-sync-start", useUnmergedTree = true).onChildren().assertAny(hasText("Aceito pela empresa", substring = true))
        scrollTo("enterprise-trip-sync-finish")
        compose.onNodeWithTag("enterprise-trip-sync-finish", useUnmergedTree = true).onChildren().assertAny(hasText("Salvo neste aparelho", substring = true))
        update(state(view(trip, listOf(start, stop, note, finish, submit).associateWith { ack(it, revision = if (it == finish) 2 else 1) }), open = id(10)))
        compose.onNodeWithTag("enterprise-trip-review").assertTextEquals("Revisão: aguardando aprovação")
        compose.onNodeWithText("A decisão da empresa ainda não é recebida neste aparelho.").assertExists()
        compose.onAllNodesWithText("Aprovado", substring = true).assertCountEquals(0)
        scrollTo("enterprise-trip-send")
        compose.onNodeWithTag("enterprise-trip-send").assertIsEnabled().performClick()
        assertEquals(1, calls.sends)
    }

    @Test fun aHeldStartShowsNeedsReviewWithItsReasonAndItsWaitingChildren() {
        val trip = book(submitted = false)
        render(state(view(trip, mapOf(start to ack(start, "ACCEPTED_FOR_REVIEW", "ASSET_BUSY", null))), open = id(10)))
        compose.onNodeWithTag("enterprise-trip-waiting").assertTextContains("aguardam a revisão do início", substring = true)
        scrollTo("enterprise-trip-sync-start")
        val chip = compose.onNodeWithTag("enterprise-trip-sync-start")
        chip.assert(SemanticsMatcher("state names the review and its reason") {
            val described = it.config.getOrElseNullable(androidx.compose.ui.semantics.SemanticsProperties.StateDescription) { null }.orEmpty()
            "Precisa de revisão" in described && "O ativo já estava em uso" in described
        })
        compose.onNodeWithTag("enterprise-trip-sync-start", useUnmergedTree = true).onChildren().assertAny(hasText("Motivo: O ativo já estava em uso"))
    }

    @Test fun theStartUsesThePickerAndSavesOnlyACompleteEntry() {
        val (calls, update) = render(state())
        compose.onNodeWithTag("enterprise-trip-empty").assertExists()
        compose.onNodeWithTag("enterprise-trip-start").performClick()
        compose.onNodeWithTag("enterprise-asset-picker-row-${id(20)}").performClick()
        compose.onNodeWithTag("enterprise-trip-chosen").assertTextContains("Caminhão sintético", substring = true)
        compose.onNodeWithTag("enterprise-trip-save-start").assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-trip-reading-value").performTextInput("184320,5")
        compose.onNodeWithTag("enterprise-trip-save-start").performClick()
        val (choice, capture) = calls.starts.single()
        assertEquals(id(20), choice.assetId)
        assertEquals("184320.5", (capture as TripCapture.Reading).raw)
        assertEquals(id(60), capture.meter.meterId)
        // A local refusal keeps the form and says why; a durable save returns to the list with "Salvo no aparelho".
        update(state(refusal = TripLocalFailure.ASSET_HAS_UNFINISHED_TRIP))
        compose.onNodeWithTag("enterprise-trip-feedback").assertTextContains("já tem uma viagem não encerrada", substring = true)
        compose.onNodeWithTag("enterprise-trip-start-form").assertExists()
        update(state(view(book(finished = false, submitted = false)), saved = id(100)))
        compose.onNodeWithTag("enterprise-trip-saved").assertTextEquals("Salvo no aparelho.")
        compose.onNodeWithTag("enterprise-trip-row-${id(10)}").assertExists()
    }

    @Test fun eventsFinishAndReadyAreOfferedOnlyInTheirLocalState() {
        val open = book(finished = false, submitted = false)
        val (calls, update) = render(state(view(open), open = id(10)), warnings = listOf(ReadingFlag.READING_BELOW_PREVIOUS))
        compose.onNodeWithTag("enterprise-trip-ready").assertDoesNotExist()
        scrollTo("enterprise-trip-add-event")
        compose.onNodeWithTag("enterprise-trip-add-event").performClick()
        compose.onNodeWithTag("enterprise-trip-event-NOTE").performClick()
        compose.onNodeWithTag("enterprise-trip-save-event").assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-trip-event-text").performTextInput("Entrega conferida")
        compose.onNodeWithTag("enterprise-trip-save-event").performClick()
        assertEquals(Triple(id(10), TripLogbookType.NOTE, "Entrega conferida"), calls.logs.single())
        update(state(view(open), open = id(10), saved = id(201)))
        scrollTo("enterprise-trip-finish")
        compose.onNodeWithTag("enterprise-trip-finish").performClick()
        compose.onNodeWithTag("enterprise-trip-reading-value").performTextInput("184000")
        compose.onNodeWithTag("enterprise-trip-warning").assertTextContains("menor que a de saída", substring = true)
        compose.onNodeWithTag("enterprise-trip-save-finish").assertIsEnabled().performClick()
        assertEquals("184000", (calls.finishes.single() as TripCapture.Reading).raw)
        update(state(view(book(submitted = false)), open = id(10), saved = id(300)))
        scrollTo("enterprise-trip-ready")
        compose.onNodeWithTag("enterprise-trip-add-event").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-trip-ready").performClick()
        assertEquals(listOf(id(10)), calls.ready)
    }

    @Test fun aFailedWriteSaysNothingWasSaved() {
        render(state(view(book(finished = false, submitted = false)), saveFailed = true))
        compose.onNodeWithTag("enterprise-trip-feedback").assertTextEquals("Nada foi salvo. Tente de novo.")
        compose.onAllNodesWithTag("enterprise-trip-saved").assertCountEquals(0)
    }

    /** EMP-TRIP-020 (B-BR-EMP-522..526): the closing conference shows each item's own states, no total and no decision. */
    @Test fun theClosingConferenceShowsEachItemsOwnStatesAndNeverATotal() {
        val fueling = FuelingOperations.submit(FuelingEnvelope(binding, id(3), id(4), id(500), TimeDeclaration(EnterpriseTime.parseInstant("2026-11-15T13:30:00Z"), "UTC", 0)),
            id(510), FuelingSubmitInput(id(20), id(5), "DIESEL_S10", "L", "150000", "97000", null, null, null, "FULL", "COMPANY", "FUEL_CARD", null, "Painel apagado"))
        val toll = ExpenseOperations.submit(ExpenseEnvelope(binding, id(3), id(4), id(600), TimeDeclaration(EnterpriseTime.parseInstant("2026-11-15T14:00:00Z"), "UTC", 0)),
            id(610), id(20), "TOLL", "1250", "1250", "EMPLOYEE", null)
        val photo = AttachmentPresentation.view(EnterpriseAttachment(id(700), id(701), id(702), fueling.operationId, AttachmentTarget(DocumentTargetKind.FUELING, id(510)),
            "ab".repeat(32), 2048, EnterpriseTime.parseInstant("2026-11-15T13:31:00Z"), AttachmentState.STORED, null, 1), ack(fueling))
        val ready = state(view(book(finished = true, submitted = false)), open = id(10)).copy(relatedEnabled = true,
            fuelings = listOf(FuelingPresentation.view(EnterpriseQueueEntry(fueling, acknowledgement = ack(fueling)))),
            expenses = listOf(ExpensePresentation.view(EnterpriseQueueEntry(toll))), attachments = mapOf(fueling.operationId to listOf(photo)))
        val (calls, update) = render(ready)
        scrollTo("enterprise-trip-closing")
        compose.onNodeWithTag("enterprise-trip-closing").assertExists()
        compose.onNodeWithTag("enterprise-trip-closing-fueling-${id(510)}", useUnmergedTree = true).onChildren().apply {
            assertAny(hasText("Abastecimento · R$ 970,00")); assertAny(hasText("Aceito pela empresa")); assertAny(hasText("Comprovante: guardado pela empresa"))
        }
        compose.onNodeWithTag("enterprise-trip-closing-expense-${id(610)}", useUnmergedTree = true).onChildren().apply {
            assertAny(hasText("Despesa · R$ 12,50")); assertAny(hasText("Salvo neste aparelho")); assertAny(hasText("Comprovante: faltando"))
        }
        compose.onAllNodesWithText("Total", substring = true).assertCountEquals(0)
        compose.onAllNodesWithText("válid", substring = true, ignoreCase = true).assertCountEquals(0)
        compose.onAllNodesWithText("Aprovad", substring = true).assertCountEquals(0)
        scrollTo("enterprise-trip-ready")
        compose.onNodeWithTag("enterprise-trip-ready").performClick()
        assertEquals(listOf(id(10)), calls.ready)
        // An unreadable attachment index is never shown as stored.
        update(ready.copy(attachmentsUnavailable = true, attachments = emptyMap()))
        scrollTo("enterprise-trip-closing")
        compose.onAllNodesWithText("Comprovante: guardado pela empresa").assertCountEquals(0)
        compose.onAllNodesWithText("Comprovantes: não é possível ler neste aparelho").fetchSemanticsNodes().isNotEmpty().let { assertTrue(it) }
    }
}
