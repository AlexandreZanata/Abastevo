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
import com.anpfuel.app.ui.enterprise.trip.TripPanel
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.application.enterprise.trip.TripViewState
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.trip.*
import java.time.Instant
import java.util.Locale
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * EMP-ROUTE-004 (B-BR-EMP-532..535, SCR-ROUTE-06) over real inbox and trip views: "Navegar" hands exactly the plan's text
 * destination to the app's own route preview, says it is not recorded, and is absent with its reason otherwise.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseRouteHandoffPresentationDeviceTest {
    @get:Rule val compose = createComposeRule()
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val partition = EnterprisePartition.of(id(1), "https://example.invalid", id(2))
    private val context = EnterpriseContextSummary(partition, id(3), "Synthetic company A", id(5))
    private val binding = SyncBinding.of(id(1), "https://example.invalid", id(2))
    private fun clock() = EnterpriseDeviceClock("boot-1", 0, 1_763_200_000_000L)
    private fun item(mission: Int, text: String?, state: TripMissionState = TripMissionState.ACCEPTED) = MissionInboxItem(id(mission), id(81), id(20),
        "Entrega no cliente Norte", Instant.parse("2026-11-20T11:00:00Z"), Instant.parse("2026-11-20T15:00:00Z"), state, 4, id(5),
        if (state == TripMissionState.ACCEPTED) TripMissionResponse.ACCEPT else null, null,
        MissionPlanView(2, text, if (text == null) id(82) else null, "Entrega programada", null, TripMissionLoad.NONE, null, null,
            Instant.parse("2026-11-15T10:10:00Z")), Instant.parse("2026-11-15T10:00:00Z"), Instant.parse("2026-11-15T10:30:00Z"))
    private fun inbox(vararg items: MissionInboxItem): MissionInboxView =
        MissionInboxPresentation.view(MissionInbox.empty(id(3), id(4)).withPage(id(5), MissionInboxPage(items.toList(), false), clock()), emptyMap(), null, clock())
    private fun trip(mission: Int?): TripView {
        val start = TripOperations.start(TripEnvelope(binding, id(3), id(4), id(100), TimeDeclaration(EnterpriseTime.parseInstant("2026-11-15T12:00:00Z"), "UTC", 0)),
            id(10), id(20), id(5), TripReadingInput(id(60), id(61), id(62), "ODOMETER", "184320000"), null, TripClockReading("boot-1", 1_000),
            missionId = mission?.let(::id))
        val local = LocalTrip(id(10), id(20), TripAssetLabel("Caminhão sintético", "FROTA-12", "ABC1D23"), false, id(100),
            EnterpriseTime.parseInstant("2026-11-15T12:00:00Z"), missionId = mission?.let(::id))
        return TripPresentation.view(local, mapOf(start.operationId to EnterpriseQueueEntry(start)))
    }
    private fun render(state: TripViewState, navigated: MutableList<String>) {
        val application = InstrumentationRegistry.getInstrumentation().targetContext
        val configuration = Configuration(application.resources.configuration).apply { setLocale(Locale.forLanguageTag("pt-BR")) }
        val localized = application.createConfigurationContext(configuration)
        compose.setContent {
            CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                MaterialTheme {
                    TripPanel(state, true, MyAssetsViewState(), {}, {}, {}, {}, { _, _ -> null }, { _, _, _ -> }, { _, _, _ -> }, { _, _ -> },
                        { _, _ -> emptyList() }, {}, {}, missionsEnabled = true, onNavigate = { navigated += it })
                }
            }
        }
    }
    private fun state(missions: MissionInboxView, vararg trips: TripView, open: String? = null) =
        TripViewState(generation = 3, epoch = 0, context = context, loaded = true, trips = trips.toList(), openTripId = open, missions = missions)
    private fun scrollTo(tag: String) = compose.onNodeWithTag("enterprise-trip-panel").performScrollToNode(hasTestTag(tag))

    @Test fun aMissionHandsExactlyItsPlanTextToTheAppsOwnRoute() {
        val navigated = mutableListOf<String>()
        render(state(inbox(item(70, "Cliente Norte, doca 3"))), navigated)
        compose.onNodeWithTag("enterprise-mission-open-${id(70)}").performClick()
        compose.onNodeWithTag("enterprise-route-notice").assertTextEquals("Esta navegação não é registrada na viagem e não envia sua posição.")
        compose.onNodeWithTag("enterprise-route-navigate").assertTextEquals("Navegar").performClick()
        assertEquals(listOf("Cliente Norte, doca 3"), navigated)
    }

    @Test fun aUnitDestinationOffersNoNavigationAndSaysWhy() {
        val navigated = mutableListOf<String>()
        render(state(inbox(item(70, null))), navigated)
        compose.onNodeWithTag("enterprise-mission-open-${id(70)}").performClick()
        compose.onNodeWithTag("enterprise-route-navigate").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-route-unsupported").assertTextContains("unidade da empresa", substring = true)
        assertTrue(navigated.isEmpty())
    }

    @Test fun aTripNavigatesToItsOwnMissionAndATripWithoutMissionSaysSo() {
        val navigated = mutableListOf<String>()
        render(state(inbox(item(70, "Cliente Norte, doca 3"), item(71, "Cliente Sul")), trip(70), open = id(10)), navigated)
        scrollTo("enterprise-route-navigate")
        compose.onNodeWithTag("enterprise-route-navigate").performClick()
        assertEquals(listOf("Cliente Norte, doca 3"), navigated)
    }

    @Test fun anOrdinaryTripOffersNoNavigation() {
        val navigated = mutableListOf<String>()
        render(state(inbox(item(70, "Cliente Norte, doca 3")), trip(null), open = id(10)), navigated)
        scrollTo("enterprise-route-unsupported")
        compose.onNodeWithTag("enterprise-route-unsupported").assertTextEquals("Esta viagem não tem missão com destino.")
        compose.onNodeWithTag("enterprise-route-navigate").assertDoesNotExist()
        assertTrue(navigated.isEmpty())
    }
}
