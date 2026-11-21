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
import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripViewState
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.*
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.trip.*
import java.time.Instant
import java.util.Locale
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * EMP-TRIP-021 missions inbox and trip execution (B-BR-EMP-527..531) over real inbox and trip views built from real operations
 * and receipts; no producer runs here.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseMissionPresentationDeviceTest {
    @get:Rule val compose = createComposeRule()
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val partition = EnterprisePartition.of(id(1), "https://example.invalid", id(2))
    private val context = EnterpriseContextSummary(partition, id(3), "Synthetic company A", id(5))
    private val binding = SyncBinding.of(id(1), "https://example.invalid", id(2))
    private fun envelope(op: Int, at: String) = TripEnvelope(binding, id(3), id(4), id(op), TimeDeclaration(EnterpriseTime.parseInstant(at), "UTC", 0))
    private val hour = 3_600_000L
    private fun clock(elapsed: Long) = EnterpriseDeviceClock("boot-1", elapsed, 1_763_200_000_000L + elapsed)
    private fun item(mission: Int, state: TripMissionState = TripMissionState.ASSIGNED, revision: Long = 3, answer: TripMissionResponse? = null,
        destination: String? = "Cliente Norte, doca 3") = MissionInboxItem(id(mission), id(81), id(20), "Entrega no cliente Norte",
        Instant.parse("2026-11-20T11:00:00Z"), Instant.parse("2026-11-20T15:00:00Z"), state, revision, id(5), answer, null,
        MissionPlanView(2, destination, if (destination == null) id(82) else null, "Entrega programada", null, TripMissionLoad.PASSENGERS, null, 4,
            Instant.parse("2026-11-15T10:10:00Z")), Instant.parse("2026-11-15T10:00:00Z"), Instant.parse("2026-11-15T10:30:00Z"))
    private fun inbox(vararg items: MissionInboxItem) = MissionInbox.empty(id(3), id(4)).withPage(id(5), MissionInboxPage(items.toList(), false), clock(0))
    private fun ack(op: EnterpriseOperation, revision: Long? = 1) = OperationAcknowledgement.parse(
        ("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"ACCEPTED","code":null,""" +
            """"entity_revision":${revision ?: "null"},"received_at":"2026-11-15T16:00:00Z","review_item_id":null,"warnings":[]}""").toByteArray(), op)
    private val label = TripAssetLabel("Caminhão sintético", "FROTA-12", "ABC1D23")
    private val assets = run {
        val request = ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(id(70), id(71), "synthetic-reference-access", id(3), id(4), null))
        val asset = { n: Int, name: String -> """{"id":"${id(n)}","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"$name","fleet_number":"FROTA-$n","plate":"ABC1D$n","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[{"id":"${id(n + 40)}","kind":"ODOMETER","modulus":null,"segment_id":"${id(n + 41)}","installed_at":"2026-11-01T00:00:00Z"}],"assignment":{"kind":"PRIMARY","valid_to":null}}""" }
        val raw = """{"schema_version":1,"request_id":"${id(70)}","membership_id":"${id(3)}","enrollment_id":"${id(4)}","mode":"RESET","reason":"NEW_CURSOR","from_revision":null,"revision":2,"confirmed_at":"2026-11-15T12:00:00Z","cursor":"c-1","entity_types":["asset"],"changes":[{"revision":1,"entity_type":"asset","entity_id":"${id(20)}","op":"UPSERT","asset":${asset(20, "Caminhão sintético")}},{"revision":2,"entity_type":"asset","entity_id":"${id(21)}","op":"UPSERT","asset":${asset(21, "Van sintética")}}]}"""
        val cache = ReferenceCache.apply(null, request, ReferenceFeedResponse.parse(raw.toByteArray(), request, null),
            EnterpriseDeviceClock("boot-1", 1_000, System.currentTimeMillis()))
        MyAssetsViewState(generation = 9, epoch = 0, context = context, loaded = true, snapshot = MyAssets.present(cache, EnterpriseDeviceClock("boot-1", 2_000, System.currentTimeMillis())))
    }
    private fun state(missions: MissionInboxView?, vararg trips: TripView, open: String? = null, saved: String? = null,
        refusal: MissionLocalFailure? = null, refreshed: Boolean = false, failed: EnterpriseAccessError? = null) =
        TripViewState(generation = 3, epoch = 0, context = context, loaded = true, trips = trips.toList(), openTripId = open, lastSaved = saved,
            missions = missions, missionRefusal = refusal, refreshed = refreshed, refreshFailed = failed)

    private class Calls {
        var refreshes = 0
        val answers = mutableListOf<Triple<String, TripMissionResponse, String?>>()
        val missionStarts = mutableListOf<Pair<String, EnterpriseAssetChoice>>()
        val commands = mutableListOf<Triple<String, TripCommand, String?>>()
    }

    private fun render(initial: TripViewState, connected: Boolean = true): Pair<Calls, (TripViewState) -> Unit> {
        val calls = Calls()
        var current by mutableStateOf(initial)
        val application = InstrumentationRegistry.getInstrumentation().targetContext
        val configuration = Configuration(application.resources.configuration).apply { setLocale(Locale.forLanguageTag("pt-BR")) }
        val localized = application.createConfigurationContext(configuration)
        compose.setContent {
            CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                MaterialTheme {
                    TripPanel(current, connected, assets, {}, {}, {}, {},
                        { id, warning -> MyAssets.choose(partition.organizationId, assets.snapshot!!, id, warning) },
                        { _, _, _ -> }, { _, _, _ -> }, { _, _ -> }, { _, _ -> emptyList() }, {}, {},
                        missionsEnabled = true,
                        onRefreshMissions = { calls.refreshes++ },
                        onRespond = { mission, response, reason -> calls.answers += Triple(mission, response, reason) },
                        onStartMission = { mission, choice, _, _ -> calls.missionStarts += mission to choice },
                        onLifecycle = { trip, command, reason -> calls.commands += Triple(trip, command, reason) })
                }
            }
        }
        return calls to { next -> current = next }
    }
    private fun scrollTo(tag: String) = compose.onNodeWithTag("enterprise-trip-panel").performScrollToNode(hasTestTag(tag))

    @Test fun theCopyAlwaysShowsItsAgeAndRefreshingIsOnlineOnly() {
        val copy = inbox(item(70))
        val (calls, update) = render(state(MissionInboxPresentation.view(copy, emptyMap(), null, clock(3 * hour))), connected = false)
        compose.onNodeWithTag("enterprise-missions-age").assertTextEquals("Atualizadas há 3 h")
        compose.onNodeWithTag("enterprise-missions-stale").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-missions-refresh").assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-missions-refresh-offline").assertTextEquals("Atualizar missões precisa de conexão.")
        assertEquals(0, calls.refreshes)
        // A day later the copy is labelled; a clock moved back makes the age unknown, never hidden.
        update(state(MissionInboxPresentation.view(copy, emptyMap(), null, clock(30 * hour))))
        compose.onNodeWithTag("enterprise-missions-age").assertTextEquals("Atualizadas há 30 h")
        compose.onNodeWithTag("enterprise-missions-stale").assertTextContains("pode estar desatualizada", substring = true)
        update(state(MissionInboxPresentation.view(copy, emptyMap(), null, EnterpriseDeviceClock("boot-2", 5, 1))))
        compose.onNodeWithTag("enterprise-missions-age").assertTextContains("desconhecida", substring = true)
        compose.onNodeWithTag("enterprise-missions-stale").assertExists()
        // With a connection the refresh is offered; a failure keeps the copy and says nothing was updated.
        update(state(MissionInboxPresentation.view(copy, emptyMap(), null, clock(hour)), failed = EnterpriseAccessError.UNAVAILABLE))
        compose.onNodeWithTag("enterprise-missions-refresh-failed").assertTextContains("lista anterior foi mantida", substring = true)
        compose.onNodeWithTag("enterprise-mission-row-${id(70)}").assertExists()
        update(state(MissionInboxPresentation.view(null, emptyMap(), null, clock(0))))
        compose.onNodeWithTag("enterprise-missions-never").assertTextContains("ainda não foram baixadas", substring = true)
    }

    @Test fun anOfflineAnswerIsShownSavedAndAnUnconfirmedAcceptanceExplainsTheMissingStart() {
        val accept = TripMissionOperations.respond(envelope(600, "2026-11-15T12:00:00Z"), id(70), 3, "ACCEPT", null)
        val copy = inbox(item(70))
        val (calls, update) = render(state(MissionInboxPresentation.view(copy, emptyMap(), null, clock(0))), connected = false)
        compose.onNodeWithTag("enterprise-mission-open-${id(70)}").performClick()
        compose.onNodeWithTag("enterprise-mission-destination").assertTextEquals("Destino: Cliente Norte, doca 3")
        compose.onNodeWithText("Passageiros: 4").assertExists()
        compose.onNodeWithTag("enterprise-mission-accept").assertIsEnabled().performClick()
        assertEquals(Triple(id(70), TripMissionResponse.ACCEPT, null), calls.answers.single())
        val saved = copy.withResponse(MissionResponseLine(id(70), id(600), 3, TripMissionResponse.ACCEPT))
        update(state(MissionInboxPresentation.view(saved, mapOf(accept.operationId to EnterpriseQueueEntry(accept)), null, clock(0)), saved = id(600)))
        compose.onNodeWithTag("enterprise-mission-state-${id(70)}").assert(SemanticsMatcher("saved, never accepted") {
            val described = it.config.getOrElseNullable(androidx.compose.ui.semantics.SemanticsProperties.StateDescription) { null }.orEmpty()
            "Resposta salva no aparelho" in described && "Aceita pela empresa" !in described
        })
        compose.onNodeWithTag("enterprise-mission-start-blocked").assertTextContains("quando a empresa confirmar", substring = true)
        compose.onNodeWithTag("enterprise-mission-accept").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-mission-start").assertDoesNotExist()
        // The answer's own ACCEPTED receipt shows it accepted and offers the planned start.
        update(state(MissionInboxPresentation.view(saved, mapOf(accept.operationId to EnterpriseQueueEntry(accept, acknowledgement = ack(accept, 4))), null,
            clock(0))))
        compose.onNodeWithTag("enterprise-mission-state-${id(70)}", useUnmergedTree = true).onChildren().assertAny(hasText("Aceita pela empresa"))
        compose.onNodeWithTag("enterprise-mission-start").assertIsEnabled()
    }

    @Test fun aDeclineNeedsAReasonAndAnUnconfirmedAnswerIsExplainedAfterARefresh() {
        val copy = inbox(item(70), item(71))
        val (calls, update) = render(state(MissionInboxPresentation.view(copy, emptyMap(), null, clock(0))))
        compose.onNodeWithTag("enterprise-mission-open-${id(70)}").performClick()
        compose.onNodeWithTag("enterprise-mission-decline").performClick()
        compose.onNodeWithTag("enterprise-mission-decline-form-save").assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-mission-decline-form-text").performTextInput("Estarei de folga")
        compose.onNodeWithTag("enterprise-mission-decline-form-save").performClick()
        assertEquals(Triple(id(70), TripMissionResponse.DECLINE, "Estarei de folga"), calls.answers.single())
        val decline = TripMissionOperations.respond(envelope(601, "2026-11-15T12:00:00Z"), id(70), 3, "DECLINE", "Estarei de folga")
        val reassigned = copy.withResponse(MissionResponseLine(id(70), id(601), 3, TripMissionResponse.DECLINE))
            .withPage(id(5), MissionInboxPage(listOf(item(71)), false), clock(10))
        update(state(MissionInboxPresentation.view(reassigned, mapOf(decline.operationId to EnterpriseQueueEntry(decline)), null, clock(20))))
        compose.onNodeWithTag("enterprise-trip-back").performClick()
        scrollTo("enterprise-mission-state-${id(70)}")
        compose.onNodeWithTag("enterprise-mission-state-${id(70)}", useUnmergedTree = true).onChildren()
            .assertAny(hasText("Resposta não confirmada: esta missão não está mais atribuída a você."))
        compose.onNodeWithTag("enterprise-mission-open-${id(70)}").performClick()
        compose.onNodeWithTag("enterprise-mission-accept").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-mission-decline").assertDoesNotExist()
    }

    @Test fun aPlannedStartOffersOnlyTheMissionsVehicle() {
        val copy = inbox(item(71, TripMissionState.ACCEPTED, 4, TripMissionResponse.ACCEPT))
        val (calls, update) = render(state(MissionInboxPresentation.view(copy, emptyMap(), TripBook.empty(id(3), id(4)), clock(0))))
        compose.onNodeWithTag("enterprise-mission-open-${id(71)}").performClick()
        compose.onNodeWithTag("enterprise-mission-start").performClick()
        compose.onNodeWithTag("enterprise-asset-picker-row-${id(21)}").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-asset-picker-row-${id(20)}").performClick()
        compose.onNodeWithTag("enterprise-trip-reading-value").performTextInput("184320")
        compose.onNodeWithTag("enterprise-trip-save-start").performClick()
        val (mission, choice) = calls.missionStarts.single()
        assertEquals(id(71), mission); assertEquals(id(20), choice.assetId)
        // A local refusal says why; nothing else changes.
        update(state(MissionInboxPresentation.view(copy, emptyMap(), TripBook.empty(id(3), id(4)), clock(0)), refusal = MissionLocalFailure.WRONG_VEHICLE))
        compose.onNodeWithTag("enterprise-trip-feedback").assertTextEquals("A viagem da missão começa no veículo da missão.")
    }

    @Test fun pauseResumeAndCancelAreOfferedByLocalStateAndACancellationNeedsAReason() {
        val start = TripOperations.start(envelope(100, "2026-11-15T12:00:00Z"), id(10), id(20), id(5),
            TripReadingInput(id(60), id(61), id(62), "ODOMETER", "184320000"), null, TripClockReading("boot-1", 1_000), missionId = id(71))
        val pause = TripOperations.lifecycle(envelope(500, "2026-11-15T13:00:00Z"), TripCommand.PAUSE, id(10), 1, "Almoço", id(100))
        val book = TripBook.empty(id(3), id(4)).withStart(LocalTrip(id(10), id(20), label, false, id(100), EnterpriseTime.parseInstant("2026-11-15T12:00:00Z"),
            missionId = id(71)))
        val copy = inbox(item(71, TripMissionState.ACCEPTED, 4, TripMissionResponse.ACCEPT))
        val running = TripPresentation.view(book.trip(id(10))!!, mapOf(start.operationId to EnterpriseQueueEntry(start)))
        val (calls, update) = render(state(MissionInboxPresentation.view(copy, emptyMap(), book, clock(0)), running, open = id(10)))
        compose.onNodeWithTag("enterprise-trip-mission").assertTextEquals("Viagem da missão: Entrega no cliente Norte")
        scrollTo("enterprise-trip-pause")
        compose.onNodeWithTag("enterprise-trip-resume").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-trip-pause").performClick()
        compose.onNodeWithTag("enterprise-trip-lifecycle-form-text").performTextInput("Almoço")
        compose.onNodeWithTag("enterprise-trip-lifecycle-form-save").performClick()
        assertEquals(Triple(id(10), TripCommand.PAUSE, "Almoço"), calls.commands.single())
        val paused = book.withPause(id(10), id(500), EnterpriseTime.parseInstant("2026-11-15T13:00:00Z"))
        val pausedView = TripPresentation.view(paused.trip(id(10))!!, mapOf(start.operationId to EnterpriseQueueEntry(start, acknowledgement = ack(start)),
            pause.operationId to EnterpriseQueueEntry(pause)))
        update(state(MissionInboxPresentation.view(copy, emptyMap(), paused, clock(0)), pausedView, open = id(10), saved = id(500)))
        compose.onNodeWithTag("enterprise-trip-operational").assertTextEquals("Pausada")
        scrollTo("enterprise-trip-sync-lifecycle-${id(500)}")
        compose.onNodeWithTag("enterprise-trip-sync-lifecycle-${id(500)}", useUnmergedTree = true).onChildren()
            .assertAny(hasText("Salvo neste aparelho", substring = true))
        scrollTo("enterprise-trip-resume")
        compose.onNodeWithTag("enterprise-trip-pause").assertDoesNotExist()
        scrollTo("enterprise-trip-cancel-trip")
        compose.onNodeWithTag("enterprise-trip-cancel-trip").performClick()
        compose.onNodeWithTag("enterprise-trip-lifecycle-form-save").assertIsNotEnabled()
        compose.onNodeWithTag("enterprise-trip-lifecycle-form-text").performTextInput("Cliente cancelou")
        compose.onNodeWithTag("enterprise-trip-lifecycle-form-save").performClick()
        assertEquals(Triple(id(10), TripCommand.CANCEL, "Cliente cancelou"), calls.commands.last())
    }
}
