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
import com.anpfuel.app.ui.enterprise.checklist.ChecklistActions
import com.anpfuel.app.ui.enterprise.checklist.ChecklistRunPanel
import com.anpfuel.app.ui.enterprise.trip.TripPanel
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.checklist.ChecklistViewState
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.application.enterprise.trip.TripViewState
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.*
import com.anpfuel.domain.enterprise.maintenance.checklist.*
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.SyncBinding
import androidx.compose.ui.unit.dp
import java.time.Instant
import java.util.Locale
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * The checklist execution screens (EMP-CHECK-002, B-BR-EMP-443) over real checklist views built from the real book,
 * real operations and the shared rule; no producer or network runs here. pt-BR, the app's light theme tokens.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseChecklistPresentationDeviceTest {
    @get:Rule val compose = createComposeRule()
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val partition = EnterprisePartition.of(id(1), "https://example.invalid", id(2))
    private val context = EnterpriseContextSummary(partition, id(3), "Synthetic company A", id(5))
    private val binding = SyncBinding.of(id(1), "https://example.invalid", id(2))
    private val asset = id(20)
    private val now: Instant = Instant.now().truncatedTo(java.time.temporal.ChronoUnit.MILLIS)
    private val items = listOf(ChecklistItem("Luzes", optional = true), ChecklistItem("Freios", critical = true), ChecklistItem("Pneus", photoOnNotOk = true))
    private fun applicable(mode: PolicyMode = PolicyMode.BLOCK) = ChecklistApplicable(asset, ChecklistContext.DEPARTURE, ChecklistMatch.ORGANIZATION,
        ChecklistCachedTemplate(id(30), "Saída de caminhões", 1, Instant.parse("2026-11-14T10:00:00Z"), items),
        ChecklistCachedPolicy(1, mode, 43_200, Instant.parse("2026-11-14T10:00:00Z")), now.minusSeconds(3_600))
    private fun book(mode: PolicyMode = PolicyMode.BLOCK) = ChecklistBook.empty(id(3), id(4)).withApplicable(applicable(mode))
    private fun answered(book: ChecklistBook, run: Int, vararg answers: String?, at: Instant = now.minusSeconds(600)): ChecklistBook {
        var next = book.withStart(id(run), asset, ChecklistContext.DEPARTURE, at)
        answers.forEachIndexed { i, answer -> if (answer != null) next = next.withAnswer(id(run), i + 1, answer, if (answer == ChecklistAnswer.NOT_OK && i == 1) "Freios rangendo" else null, at) }
        return next
    }
    private fun complete(book: ChecklistBook, run: Int, op: Int, acknowledged: Boolean = false, at: Instant = now.minusSeconds(300)): Pair<ChecklistBook, EnterpriseOperation> {
        val local = book.run(id(run))!!
        val evaluation = book.evaluate(id(run), at)
        val operation = ChecklistRunOperations.complete(ChecklistRunEnvelope(binding, id(3), id(4), id(op), TimeDeclaration(at, "UTC", 0)), id(run), asset,
            "DEPARTURE", id(30), 1, 1, EnterpriseTime.formatInstant(local.startedAt), local.answers.map { ChecklistRunAnswerInput(it.answer!!, it.note) },
            evaluation.outcome.name, acknowledged && evaluation.acknowledgementRequired)
        return book.withCompletion(id(run), id(op), at, acknowledged) to operation
    }
    private fun server(book: ChecklistBook, run: Int, recheck: ChecklistOutcome): ChecklistBook {
        val local = book.run(id(run))!!
        return book.withServerResult(ChecklistServerRun(local.runId, asset, id(30), ChecklistContext.DEPARTURE, 1, 1, local.startedAt, local.completedAt!!,
            local.acknowledged, ChecklistServerResult(1, 3, now.minusSeconds(60), local.deviceOutcome!!, recheck, recheck, recheck != local.deviceOutcome,
                recheck != local.deviceOutcome, false, listOf(2), listOf(2), null)))
    }
    private fun state(book: ChecklistBook, operations: List<EnterpriseOperation> = emptyList(), applicable: ChecklistApplicable? = book.applicable(asset, ChecklistContext.DEPARTURE),
        at: Instant = now): ChecklistViewState {
        val entries = operations.associate { it.operationId to EnterpriseQueueEntry(it) }
        val open = book.openRun(asset, ChecklistContext.DEPARTURE)?.let { ChecklistPresentation.view(it, entries, at) }
        val latest = book.runs.filter { it.completed }.maxByOrNull { it.completedAt!! }?.let { ChecklistPresentation.view(it, entries, at) }
        return ChecklistViewState(generation = 3, epoch = 0, context = context, assetId = asset, checklistContext = ChecklistContext.DEPARTURE, loaded = true,
            applicable = applicable, open = open, latest = latest, departure = ChecklistPresentation.latestDeparture(book, entries, asset, at), at = at)
    }

    private class Calls {
        val answers = mutableListOf<List<Any?>>()
        val completions = mutableListOf<Pair<String, Boolean>>()
        var starts = 0
        var sends = 0
        var refreshes = 0
        val departureChecks = mutableListOf<String>()
    }
    private val localized = run {
        val application = InstrumentationRegistry.getInstrumentation().targetContext
        val configuration = Configuration(application.resources.configuration).apply { setLocale(Locale.forLanguageTag("pt-BR")) }
        application.createConfigurationContext(configuration) to configuration
    }
    private fun actions(calls: Calls) = ChecklistActions(onRefresh = { calls.refreshes++ }, onRetry = {}, onStart = { calls.starts++ },
        onAnswer = { run, position, answer, note -> calls.answers += listOf(run, position, answer, note) },
        onComplete = { run, acknowledged -> calls.completions += run to acknowledged }, onSend = { calls.sends++ })
    private fun render(initial: ChecklistViewState, connected: Boolean = false): Pair<Calls, (ChecklistViewState) -> Unit> {
        val calls = Calls()
        var current by mutableStateOf(initial)
        compose.setContent {
            CompositionLocalProvider(LocalContext provides localized.first, LocalConfiguration provides localized.second) {
                MaterialTheme { ChecklistRunPanel(current, connected, "Caminhão sintético · FROTA-12", actions(calls), onBack = {}) }
            }
        }
        return calls to { next -> current = next }
    }
    private fun scrollTo(tag: String) = compose.onNodeWithTag("enterprise-checklist-panel").performScrollToNode(hasTestTag(tag))

    @Test fun anOpenRunShowsOneItemPerRowWithCriticalMarksAndNotApplicableOnlyOnOptionalItems() {
        val (calls, _) = render(state(answered(book(), 100, ChecklistAnswer.OK)))
        compose.onNodeWithText("Checklist de saída").assertExists()
        compose.onNodeWithText("Saída de caminhões").assertExists()
        compose.onNodeWithTag("enterprise-checklist-confirmed").assertTextContains("Checklist confirmado pela empresa", substring = true)
        compose.onNodeWithTag("enterprise-checklist-offline").assertTextContains("Sem internet", substring = true)
        compose.onNodeWithTag("enterprise-checklist-refresh").assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-checklist-progress").assertTextEquals("Respondidos: 1/3")
        scrollTo("enterprise-checklist-item-2")
        compose.onNodeWithTag("enterprise-checklist-critical-2", useUnmergedTree = true).assertTextEquals("Item crítico")
        compose.onNodeWithTag("enterprise-checklist-answer-1-NOT_APPLICABLE").assertExists()
        compose.onAllNodesWithTag("enterprise-checklist-answer-2-NOT_APPLICABLE").assertCountEquals(0)
        compose.onNodeWithTag("enterprise-checklist-answer-2-NOT_OK").assertTextEquals("Não OK").assertHeightIsAtLeast(56.dp)
            .performClick()
        assertEquals(listOf(listOf(id(100), 2, ChecklistAnswer.NOT_OK, null)), calls.answers)
        scrollTo("enterprise-checklist-complete")
        compose.onNodeWithTag("enterprise-checklist-complete").assertIsNotEnabled()
    }

    @Test fun aPhotoItemAnsweredNotOkSaysPhotosAreNotAvailableAndNeverFakesOne() {
        render(state(answered(book(), 101, ChecklistAnswer.OK, ChecklistAnswer.OK, ChecklistAnswer.NOT_OK)))
        scrollTo("enterprise-checklist-photo-3")
        compose.onNodeWithTag("enterprise-checklist-photo-3").assertTextContains("Fotos de checklist ainda não estão disponíveis neste app", substring = true)
    }

    @Test fun aBlockingOutcomeIsStatedInTextAndIconWithItsBlockingItems() {
        val (completed, operation) = complete(answered(book(), 102, ChecklistAnswer.OK, ChecklistAnswer.NOT_OK, ChecklistAnswer.OK), 102, 502)
        val (calls, _) = render(state(completed, listOf(operation)))
        scrollTo("enterprise-checklist-outcome")
        compose.onNodeWithTag("enterprise-checklist-device-outcome").assertTextContains("Bloqueando o uso")
        compose.onAllNodesWithContentDescription("Bloqueando o uso").assertCountEquals(1)
        compose.onNodeWithTag("enterprise-checklist-blocking-items").assertTextEquals("Itens que bloqueiam: Freios")
        compose.onNodeWithText("A empresa confere este resultado quando o receber.").assertExists()
        compose.onNodeWithText("O resultado da empresa é lido depois que o checklist é entregue.").assertExists()
        compose.onNodeWithTag("enterprise-checklist-sync").assertTextContains("Salvo neste aparelho", substring = true)
        compose.onAllNodesWithText("Aceito pela empresa", substring = true).assertCountEquals(0)
        compose.onNodeWithTag("enterprise-checklist-send").assertIsNotEnabled()
        assertEquals(0, calls.sends)
    }

    @Test fun aRecalculatedResultShowsTheCompanysOutcomeAndBoth() {
        val (completed, operation) = complete(answered(book(PolicyMode.WARN), 103, ChecklistAnswer.OK, ChecklistAnswer.NOT_OK, ChecklistAnswer.OK), 103, 503, acknowledged = true)
        render(state(server(completed, 103, ChecklistOutcome.BLOCKING), listOf(operation)), connected = true)
        scrollTo("enterprise-checklist-recalculated")
        compose.onNodeWithTag("enterprise-checklist-device-outcome").assertTextContains("Concluído com pendências")
        compose.onNodeWithTag("enterprise-checklist-server-outcome").assertTextContains("Bloqueando o uso")
        compose.onNodeWithTag("enterprise-checklist-recalculated").assertTextContains("A empresa recalculou o resultado", substring = true)
        compose.onNodeWithText("No aparelho: Concluído com pendências. Empresa: Bloqueando o uso.").assertExists()
    }

    @Test fun warnWithACriticalIssueRequiresTheAcknowledgementBeforeCompletion() {
        val (calls, _) = render(state(answered(book(PolicyMode.WARN), 104, ChecklistAnswer.OK, ChecklistAnswer.NOT_OK, ChecklistAnswer.OK)))
        scrollTo("enterprise-checklist-complete")
        compose.onNodeWithTag("enterprise-checklist-complete").assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-checklist-acknowledge").assertTextContains("Estou ciente das pendências").performClick()
        compose.onNodeWithTag("enterprise-checklist-complete").assertIsEnabled().performClick()
        assertEquals(listOf(id(104) to true), calls.completions)
    }

    @Test fun noRequiredChecklistNotDownloadedAndExpiredRunsAreStated() {
        val none = ChecklistApplicable(asset, ChecklistContext.DEPARTURE, ChecklistMatch.NONE, null, null, now.minusSeconds(60))
        val (calls, update) = render(state(ChecklistBook.empty(id(3), id(4)).withApplicable(none)))
        compose.onNodeWithTag("enterprise-checklist-none").assertTextEquals("Nenhum checklist exigido para este veículo")
        compose.onAllNodesWithTag("enterprise-checklist-start").assertCountEquals(0)
        update(state(ChecklistBook.empty(id(3), id(4)), applicable = null))
        compose.onNodeWithTag("enterprise-checklist-not-downloaded").assertTextContains("Conecte-se para baixá-lo", substring = true)
        val stale = answered(book(), 105, ChecklistAnswer.OK, at = now.minusSeconds(43_300))
        update(state(stale))
        // The expired notice is a descendant of a non-merging container card.
        compose.onNodeWithTag("enterprise-checklist-expired").onChildren().assertAny(hasText("Este checklist passou da validade", substring = true))
        compose.onNodeWithTag("enterprise-checklist-start-again").performClick()
        assertEquals(1, calls.starts)
    }

    @Test fun theTripStartIsDisabledWhileTheChosenVehiclesDepartureRunBlocks() {
        val (blocked, operation) = complete(answered(book(), 106, ChecklistAnswer.OK, ChecklistAnswer.NOT_OK, ChecklistAnswer.OK), 106, 506)
        val calls = Calls()
        var checklist by mutableStateOf(state(blocked, listOf(operation)))
        val trips = TripViewState(generation = 3, epoch = 0, context = context, loaded = true)
        compose.setContent {
            CompositionLocalProvider(LocalContext provides localized.first, LocalConfiguration provides localized.second) {
                MaterialTheme {
                    TripPanel(trips, true, assets, {}, {}, {}, {}, { id, warning -> MyAssets.choose(partition.organizationId, assets.snapshot!!, id, warning) },
                        { _, _, _ -> }, { _, _, _ -> }, { _, _ -> }, { _, _ -> emptyList() }, {}, {},
                        checklist = checklist, onDepartureCheck = { calls.departureChecks += it },
                        departureContent = { _, label, _ -> ChecklistRunPanel(checklist, true, label, actions(calls), onBack = {}) })
                }
            }
        }
        compose.onNodeWithTag("enterprise-trip-start").performClick()
        compose.onNodeWithTag("enterprise-asset-picker-row-$asset").performClick()
        compose.waitForIdle()
        assertEquals(listOf(asset), calls.departureChecks)
        compose.onNodeWithTag("enterprise-trip-departure-blocked").assertTextContains("A viagem não pode ser iniciada neste aparelho", substring = true)
        compose.onNodeWithTag("enterprise-trip-reading-value").performTextInput("184320")
        compose.onNodeWithTag("enterprise-trip-save-start").assertIsNotEnabled()
        // A later OK departure run of the same vehicle frees the start on this device.
        val (ok, okOperation) = complete(answered(blocked, 107, ChecklistAnswer.OK, ChecklistAnswer.OK, ChecklistAnswer.OK, at = now.minusSeconds(400)), 107, 507, at = now.minusSeconds(200))
        checklist = state(ok, listOf(operation, okOperation))
        compose.waitForIdle()
        compose.onAllNodesWithTag("enterprise-trip-departure-blocked").assertCountEquals(0)
        compose.onNodeWithTag("enterprise-trip-save-start").assertIsEnabled()
        compose.onNodeWithTag("enterprise-trip-departure-open").performClick()
        compose.onNodeWithTag("enterprise-checklist-panel").assertExists()
    }

    private val assets = run {
        val request = ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(id(70), id(71), "synthetic-reference-access", id(3), id(4), null))
        val json = """{"id":"$asset","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"Caminhão sintético","fleet_number":"FROTA-12","plate":"ABC1D23","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[{"id":"${id(60)}","kind":"ODOMETER","modulus":null,"segment_id":"${id(61)}","installed_at":"2026-11-01T00:00:00Z"}],"assignment":{"kind":"PRIMARY","valid_to":null}}"""
        val raw = """{"schema_version":1,"request_id":"${id(70)}","membership_id":"${id(3)}","enrollment_id":"${id(4)}","mode":"RESET","reason":"NEW_CURSOR","from_revision":null,"revision":1,"confirmed_at":"2026-11-15T12:00:00Z","cursor":"c-1","entity_types":["asset"],"changes":[{"revision":1,"entity_type":"asset","entity_id":"$asset","op":"UPSERT","asset":$json}]}"""
        val cache = ReferenceCache.apply(null, request, ReferenceFeedResponse.parse(raw.toByteArray(), request, null),
            EnterpriseDeviceClock("boot-1", 1_000, System.currentTimeMillis()))
        MyAssetsViewState(generation = 9, epoch = 0, context = context, loaded = true, snapshot = MyAssets.present(cache, EnterpriseDeviceClock("boot-1", 2_000, System.currentTimeMillis())))
    }
}
