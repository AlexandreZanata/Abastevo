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
import com.anpfuel.app.ui.enterprise.tracking.EnterpriseTrackingIndicator
import com.anpfuel.app.ui.enterprise.tracking.MyTrackingPanel
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.tracking.MyTrackingViewState
import com.anpfuel.application.enterprise.tracking.TrackingActivity
import com.anpfuel.application.enterprise.tracking.TrackingHistory
import com.anpfuel.application.enterprise.tracking.TrackingHistoryEntry
import com.anpfuel.application.enterprise.tracking.TrackingSessionView
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import com.anpfuel.domain.enterprise.tracking.StoredTrackingTransition
import com.anpfuel.domain.enterprise.tracking.TrackingActor
import com.anpfuel.domain.enterprise.tracking.TrackingCause
import com.anpfuel.domain.enterprise.tracking.TrackingGap
import com.anpfuel.domain.enterprise.tracking.TrackingGapReason
import com.anpfuel.domain.enterprise.tracking.TrackingInterruptions
import com.anpfuel.domain.enterprise.tracking.TrackingPolicy
import com.anpfuel.domain.enterprise.tracking.TrackingPolicyRead
import com.anpfuel.domain.enterprise.tracking.TrackingSessionState
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import java.time.Instant
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * EMP-GPS-015 presentation (B-BR-EMP-548..551, SCR-GPS-02/04) in pt-BR: the indicator names the organization and the phone and
 * states an open gap, and disappears without a running collector; "Meu rastreamento" lists the own sessions with their
 * interruptions and gaps, says what is collected and never promises remote erasure. Synthetic sessions; no coordinate.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseTrackingTransparencyDeviceTest {
    @get:Rule val compose = createComposeRule()
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val partition = EnterprisePartition.of(id(1), "https://example.invalid", id(2))
    private val context = EnterpriseContextSummary(partition, id(3), "Transportes Sintéticos", id(5))
    private fun at(minutes: Long) = Instant.parse("2026-11-16T12:00:00Z").plusSeconds(minutes * 60)

    private fun localized(content: @androidx.compose.runtime.Composable () -> Unit) {
        val application = InstrumentationRegistry.getInstrumentation().targetContext
        val configuration = Configuration(application.resources.configuration).apply { setLocale(Locale.forLanguageTag("pt-BR")) }
        val context = application.createConfigurationContext(configuration)
        compose.setContent {
            CompositionLocalProvider(LocalContext provides context, LocalConfiguration provides configuration) { MaterialTheme { content() } }
        }
    }

    @Test fun theIndicatorNamesTheOrganizationAndThePhoneStatesAGapAndDisappearsWithTheCollector() {
        var activity by mutableStateOf<TrackingActivity?>(TrackingActivity(partition, id(41), id(40), "Transportes Sintéticos", at(0),
            LocationPermission.PRECISE, null))
        var opened = 0
        localized { EnterpriseTrackingIndicator(activity, context.organizationName, onOpen = { opened++ }) }
        compose.onNodeWithTag("enterprise-tracking-indicator").assertIsDisplayed()
        compose.onNodeWithText("Rastreamento da empresa ativo — Transportes Sintéticos").assertExists()
        compose.onNodeWithText("posição deste celular, não do veículo", substring = true).assertExists()
        compose.onNodeWithText("Localização precisa").assertExists()
        activity = activity!!.let { TrackingActivity(it.partition, it.sessionId, it.tripId, it.organizationName, it.since, LocationPermission.APPROXIMATE,
            TrackingGapReason.PROVIDER_DISABLED) }
        compose.onNodeWithText("Localização aproximada").assertExists()
        compose.onNodeWithTag("enterprise-tracking-indicator-gap", useUnmergedTree = true).assertTextEquals("Localização desligada: nada é registrado até ela voltar.")
        compose.onNodeWithTag("enterprise-tracking-indicator").performClick()
        assertEquals(1, opened)
        activity = null
        compose.onNodeWithTag("enterprise-tracking-indicator").assertDoesNotExist()
    }

    private fun row(state: TrackingSessionState, cause: TrackingCause?, minutes: Long, op: Int, permission: LocationPermission? = null) =
        StoredTrackingTransition(state, cause, TrackingActor.DEVICE, permission, at(minutes), id(op))

    @Test fun myTrackingListsOwnSessionsWithInterruptionsAndGapsAndNeverPromisesRemoteErasure() {
        val rows = listOf(row(TrackingSessionState.NOT_STARTED, TrackingCause.NOT_STARTED, 0, 100), row(TrackingSessionState.RECORDING, null, 0, 100,
            LocationPermission.PRECISE), row(TrackingSessionState.INTERRUPTED, TrackingCause.DEVICE_RESTARTED, 30, 101),
            row(TrackingSessionState.RECORDING, null, 45, 102, LocationPermission.APPROXIMATE), row(TrackingSessionState.ENDED, TrackingCause.TRIP_COMPLETED, 90, 103))
        val session = TrackingSessionView(id(41), id(40), TrackingPolicy.EXPECTED, TrackingSessionState.ENDED, TrackingCause.TRIP_COMPLETED, at(90),
            null, false, null, at(89), emptyList(), 0, emptyList(), listOf(TrackingGap(TrackingGapReason.SIGNAL_LOST, at(10), at(14))), 0,
            TrackingInterruptions.of(rows), true)
        val history = TrackingHistory(TrackingPolicyRead(TrackingPolicy.EXPECTED, 2, "aviso-2026.11", at(-60), at(-1)),
            listOf(TrackingHistoryEntry(session, TripAssetLabel("Caminhão sintético", "FROTA-12", "ABC1D23"), at(0), at(90))))
        var state by mutableStateOf(MyTrackingViewState(generation = 1, epoch = 0, context = context, loaded = true, history = history))
        localized { MyTrackingPanel(state, null, onBack = {}, onRetry = {}) }
        compose.onNodeWithTag("enterprise-my-tracking-now").assertTextEquals("Nenhum rastreamento da empresa está ativo neste celular.")
        compose.onNodeWithText("um celular sem conexão não é apagado a distância", substring = true).assertExists()
        compose.onAllNodesWithText("apagado remotamente", substring = true).assertCountEquals(0)
        compose.onNodeWithTag("enterprise-my-tracking-policy").assertTextContains("rastreamento esperado", substring = true)
        compose.onNodeWithTag("enterprise-my-tracking-panel").performScrollToNode(hasTestTag("enterprise-my-tracking-session"))
        compose.onNodeWithText("Viagem com Caminhão sintético · FROTA-12 · ABC1D23").assertExists()
        compose.onNodeWithText("O celular reiniciou durante o rastreamento.", substring = true).assertExists()
        compose.onNodeWithText("Sem sinal de GPS de", substring = true).assertExists()
        compose.onNodeWithText("Parte deste rastreamento usou localização aproximada", substring = true).assertExists()
        state = MyTrackingViewState(generation = 2, epoch = 0, context = context, loaded = true, history = TrackingHistory(null, emptyList()))
        compose.onNodeWithTag("enterprise-my-tracking-panel").performScrollToNode(hasTestTag("enterprise-my-tracking-empty"))
        compose.onNodeWithTag("enterprise-my-tracking-empty").assertTextEquals("Nenhuma viagem rastreada")
        compose.onNodeWithTag("enterprise-my-tracking-policy").assertTextEquals("Política da empresa ainda não lida neste celular.")
    }
}
