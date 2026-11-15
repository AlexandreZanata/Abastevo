package com.anpfuel.app.ui.enterprise

import android.content.res.Configuration
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.ui.enterprise.fleet.EnterpriseAssetPicker
import com.anpfuel.app.ui.enterprise.fleet.MyAssetsPanel
import com.anpfuel.application.enterprise.*
import com.anpfuel.domain.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.fleet.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.fleet.*
import java.util.Locale
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** SCR-FLEET-08/09/10 presentation over synthetic protected-cache values (EMP-FLEET-013); not a runtime producer. */
@RunWith(AndroidJUnit4::class)
class EnterpriseMyAssetsPresentationDeviceTest {
    @get:Rule val compose = createComposeRule()
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val context = EnterpriseContextSummary(EnterprisePartition.of(id(1), "https://example.invalid", id(2)), id(3), "Synthetic company A")
    private val received = EnterpriseDeviceClock("boot-1", 1_000, System.currentTimeMillis())
    private fun asset(n: Int, name: String, state: String = "ACTIVE", meters: String = "[]", products: String = "[\"DIESEL_S10\"]") =
        """{"id":"${id(n)}","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"$state","name":"$name","fleet_number":"F-$n","plate":"ABC1D2$n","catalog":"fuel-products-v1","products":$products,"meters":$meters,"assignment":{"kind":"PRIMARY","valid_to":null}}"""
    private fun cache(vararg assets: String, reason: String = "NEW_CURSOR"): ReferenceCache {
        val request = ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(id(20), id(21), "synthetic-reference-access", context.membershipId, id(4), null))
        val changes = assets.mapIndexed { index, value ->
            val entity = Regex("\"id\":\"([^\"]+)\"").find(value)!!.groupValues[1]
            """{"revision":${index + 1},"entity_type":"asset","entity_id":"$entity","op":"UPSERT","asset":$value}"""
        }
        val raw = """{"schema_version":1,"request_id":"${id(20)}","membership_id":"${context.membershipId}","enrollment_id":"${id(4)}","mode":"RESET","reason":"$reason","from_revision":null,"revision":${if (reason == "FORBIDDEN") 1 else assets.size},"confirmed_at":"2026-11-15T12:00:00Z","cursor":"cursor-1","entity_types":["asset"],"changes":[${changes.joinToString(",")}]}"""
        return ReferenceCache.apply(null, request, ReferenceFeedResponse.parse(raw.toByteArray(), request, null), received)
    }
    private fun state(cache: ReferenceCache?, now: EnterpriseDeviceClock = received, loaded: Boolean = true, busy: Boolean = false,
        error: EnterpriseAccessError? = null, refreshFailed: Boolean = false, detail: String? = null) =
        MyAssetsViewState(generation = 4, epoch = 0, context = context, busy = busy, loaded = loaded,
            snapshot = cache?.let { MyAssets.present(it, now) }, refreshFailed = refreshFailed, error = error, detailAssetId = detail)

    private fun localized(content: @androidx.compose.runtime.Composable () -> Unit) {
        val application = InstrumentationRegistry.getInstrumentation().targetContext
        val configuration = Configuration(application.resources.configuration).apply { setLocale(Locale.ENGLISH) }
        val localized = application.createConfigurationContext(configuration)
        compose.setContent {
            CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) { MaterialTheme { content() } }
        }
    }
    private fun panel(state: MyAssetsViewState, connected: Boolean = true, onOpen: (String) -> Unit = {}) =
        localized { MyAssetsPanel(state, connected, {}, {}, {}, onOpen) }
    private fun scrollTo(tag: String) = compose.onNodeWithTag("enterprise-assets-panel").performScrollToNode(hasTestTag(tag))

    @Test fun loadingErrorAndNeedsConnectionStatesWithholdEveryAsset() {
        var current by mutableStateOf(state(null, loaded = false, busy = true))
        localized { MyAssetsPanel(current, false, {}, {}, {}, {}) }
        compose.onNodeWithTag("enterprise-assets-loading").assertExists()
        current = state(null, error = EnterpriseAccessError.UNAVAILABLE)
        compose.onNodeWithTag("enterprise-assets-error").assertTextContains("unavailable now", substring = true)
        compose.onNodeWithTag("enterprise-assets-retry").assertIsEnabled()
        current = state(null)
        compose.onNodeWithTag("enterprise-assets-unavailable").assertTextContains("needs a connection once", substring = true)
        compose.onNodeWithTag("enterprise-assets-refresh").assertIsNotEnabled()
        compose.onAllNodes(hasTestTag("enterprise-assets-empty")).assertCountEquals(0)
    }

    @Test fun emptyNamesWhoCanAssignAndForbiddenShowsNoAsset() {
        var current by mutableStateOf(state(cache()))
        localized { MyAssetsPanel(current, true, {}, {}, {}, {}) }
        compose.onNodeWithTag("enterprise-assets-empty").assert(hasAnyDescendant(hasText("No asset assigned to you", substring = true)))
            .assert(hasAnyDescendant(hasText("fleet manager", substring = true)))
        current = state(cache(reason = "FORBIDDEN"))
        compose.onNodeWithTag("enterprise-assets-forbidden").assert(hasAnyDescendant(hasText("No permission", substring = true)))
        compose.onAllNodes(hasTestTag("enterprise-assets-empty")).assertCountEquals(0)
        compose.onAllNodes(hasText("Caminhão", substring = true)).assertCountEquals(0)
    }

    @Test fun offlineListKeepsItsConfirmationAndMarksWhatItCannotConfirm() {
        val stale = received.copy(elapsedMillis = received.elapsedMillis + MyAssets.CONFIRMATION_MILLIS)
        val opened = mutableListOf<String>()
        panel(state(cache(asset(30, "Caminhão 30"), asset(40, "Trator 40", state = "INACTIVE")), now = stale, refreshFailed = true),
            connected = false) { opened += it }
        compose.onNodeWithTag("enterprise-assets-confirmed-at").assertTextContains("Updated on", substring = true)
        compose.onNodeWithTag("enterprise-assets-not-confirmed").assertExists()
        compose.onNodeWithTag("enterprise-assets-refresh-failed").assertExists()
        compose.onNodeWithTag("enterprise-assets-offline").assertExists()
        compose.onNodeWithTag("enterprise-assets-refresh").assertIsNotEnabled()
        scrollTo("enterprise-assets-row-${id(40)}")
        compose.onNodeWithTag("enterprise-assets-row-${id(40)}").assertTextContains("Not available: inactive", substring = true)
            .assertTextContains("Not confirmed", substring = true)
        compose.onNodeWithTag("enterprise-assets-row-${id(40)}").assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.StateDescription))
        scrollTo("enterprise-assets-row-${id(30)}")
        compose.onNodeWithTag("enterprise-assets-row-${id(30)}").assertTextContains("Fleet number F-30", substring = true).performClick()
        assertEquals(listOf(id(30)), opened)
        compose.onNode(hasText("My assets") and SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading)).assertExists()
    }

    @Test fun detailIsReadOnlyAndSaysWhatTheOfflineListLacks() {
        val meters = """[{"id":"${id(31)}","kind":"ODOMETER","modulus":null,"segment_id":"${id(32)}","installed_at":"2026-11-01T08:00:00Z"}]"""
        panel(state(cache(asset(30, "Caminhão 30", meters = meters, products = "[\"DIESEL_S10\",\"ARLA_32\"]")), detail = id(30)))
        scrollTo("enterprise-assets-detail")
        for (text in listOf("Diesel S10", "AdBlue", "Odometer installed on", "Readings and tank capacity are not part"))
            compose.onNodeWithTag("enterprise-assets-detail").assert(hasAnyDescendant(hasText(text, substring = true)))
        compose.onAllNodes(hasTestTag("enterprise-assets-refresh")).assertCountEquals(0)
        compose.onNodeWithTag("enterprise-assets-back").assertTextContains("Back to my assets")
    }

    @Test fun pickerReturnsTheAssetToTheCallingScreenWithNoNetworkAndWarnsFirstWhenNeeded(): Unit = runBlocking {
        val pulls = AtomicInteger()
        val stored = cache(asset(30, "Caminhão 30"), asset(40, "Trator 40", state = "INACTIVE"))
        val fence = EnterpriseSessionFence()
        val identity = EnterpriseLocalIdentity(context.partition.ownerAccount, context.partition.origin, id(9))
        val source = object : EnterpriseQueueSource {
            override suspend fun account() = EnterpriseQueueAccountLease.capture(identity, fence.epoch, fence.backgroundEpoch, id(50)) { true }
            override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition) =
                account.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, context.membershipId), id(4), 1))
        }
        val store = object : EnterpriseReferenceCacheStore {
            override suspend fun read(attempt: EnterpriseQueueAttempt) = stored
            override suspend fun replace(attempt: EnterpriseQueueAttempt, expected: ReferenceCache?, next: ReferenceCache) = false
        }
        val transport = EnterpriseReferenceTransport { _, _, _ -> pulls.incrementAndGet(); throw EnterpriseQueueDeliveryFailure() }
        val flow = MyAssetsFlow(source, EnterpriseReferenceService(source, store, transport) { received }, fence) { received }
        flow.open(context, fence.epoch, online = false)
        assertEquals(0, pulls.get())
        localized {
            val state by flow.state.collectAsState()
            var chosen by remember { mutableStateOf<EnterpriseAssetChoice?>(null) }
            var picking by remember { mutableStateOf(true) }
            Column {
                // A minimal calling screen: it opens the picker and receives the hint back.
                Text(chosen?.let { "Chosen " + it.assetId + " warned=" + it.reviewWarningAccepted } ?: "Nothing chosen",
                    Modifier.testTag("calling-screen-result"))
                if (picking) EnterpriseAssetPicker(state, connected = false, onChoose = { asset, warned ->
                    flow.choose(asset, warned, state.epoch ?: -1, state.generation)?.let { chosen = it; picking = false }
                }, onCancel = { picking = false }, accepts = { it.kind == AssetKind.VEHICLE })
            }
        }
        compose.onNodeWithText("No internet. The list shown is the last confirmed one.").assertExists()
        compose.onNodeWithTag("enterprise-asset-picker").performScrollToNode(hasTestTag("enterprise-asset-picker-row-${id(40)}"))
        compose.onNodeWithTag("enterprise-asset-picker-row-${id(40)}").assertHeightIsAtLeast(48.dp).performClick()
        compose.onNodeWithText("The company will review this choice").assertIsDisplayed()
        compose.onNodeWithTag("enterprise-asset-picker-warning-cancel").performClick()
        compose.onNodeWithTag("calling-screen-result").assertTextEquals("Nothing chosen")
        compose.onNodeWithTag("enterprise-asset-picker-row-${id(40)}").performClick()
        compose.onNodeWithTag("enterprise-asset-picker-warning-confirm").performClick()
        compose.onNodeWithTag("calling-screen-result").assertTextEquals("Chosen ${id(40)} warned=true")
        assertEquals("The picker never needs the network", 0, pulls.get())
    }

    @Test fun pickerChoosesAConfirmedAssetAtOnceAndHonoursTheCallerFilter(): Unit = runBlocking {
        val chosen = mutableListOf<Pair<String, Boolean>>()
        localized {
            EnterpriseAssetPicker(state(cache(asset(30, "Caminhão 30"), asset(40, "Trator 40"))), connected = true,
                onChoose = { asset, warned -> chosen += asset to warned }, onCancel = {}, accepts = { it.id == id(30) })
        }
        compose.onAllNodes(hasTestTag("enterprise-asset-picker-row-${id(40)}")).assertCountEquals(0)
        compose.onNodeWithTag("enterprise-asset-picker").performScrollToNode(hasTestTag("enterprise-asset-picker-row-${id(30)}"))
        compose.onNodeWithTag("enterprise-asset-picker-row-${id(30)}").performClick()
        assertEquals(listOf(id(30) to false), chosen)
        compose.onAllNodes(hasText("The company will review this choice")).assertCountEquals(0)
    }
}
