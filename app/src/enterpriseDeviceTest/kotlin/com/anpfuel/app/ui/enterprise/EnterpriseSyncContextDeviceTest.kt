package com.anpfuel.app.ui.enterprise

import com.anpfuel.app.ui.enterprise.attachment.ReceiptCaptureViewModel
import com.anpfuel.app.ui.enterprise.fueling.FuelingViewModel
import com.anpfuel.application.enterprise.fueling.EnterpriseFuelingService
import com.anpfuel.application.enterprise.fueling.FuelingFlow
import com.anpfuel.data.enterprise.fueling.ProtectedFuelingStore
import com.anpfuel.app.ui.enterprise.trip.TripViewModel
import com.anpfuel.data.enterprise.attachment.AndroidReceiptOriginals
import com.anpfuel.application.enterprise.trip.EnterpriseTripService
import com.anpfuel.application.enterprise.trip.EnterpriseTripStartHints
import com.anpfuel.application.enterprise.trip.TripFlow
import com.anpfuel.data.enterprise.trip.ProtectedTripStore
import java.time.ZoneId
import android.content.Context
import android.content.res.Configuration
import android.view.ContextThemeWrapper
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.ViewModelStore
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.ui.enterprise.fleet.MyAssetsViewModel
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.fleet.EnterpriseReferencePull
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceService
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceTransport
import com.anpfuel.application.enterprise.fleet.MyAssetsFlow
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.application.enterprise.sync.EnterpriseSyncFlow
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.enterprise.*
import com.anpfuel.data.enterprise.fleet.ProtectedReferenceCacheStore
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedRequest
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedRequestSnapshot
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedResponse
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseSyncStatusStore
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.*
import dagger.hilt.android.testing.HiltAndroidRule
import dagger.hilt.android.testing.HiltAndroidTest
import java.time.Instant
import java.util.Collections
import java.util.Locale
import java.util.UUID
import java.util.concurrent.atomic.AtomicInteger
import javax.inject.Inject
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Actual context-screen/Keystore acceptance with the existing Hilt encrypted account
 * session. Only contact/catalog outcomes and retained receipts are named local fixtures;
 * no network, runtime business permission, operational producer or live grant is claimed.
 */
@HiltAndroidTest
@RunWith(AndroidJUnit4::class)
class EnterpriseSyncContextDeviceTest {
    @Inject lateinit var sessions: AuthSessionStore
    @Inject lateinit var auth: AuthFlow
    @Inject lateinit var fence: EnterpriseSessionFence
    @Inject lateinit var database: AnpFuelDatabase
    @Inject lateinit var scenario: EnterpriseDeviceScenario
    @get:Rule(order = 0) val hilt = HiltAndroidRule(this)
    @get:Rule(order = 1) val compose = createAndroidComposeRule<ComponentActivity>()

    @Before fun prepareAccount() {
        hilt.inject()
        val target = InstrumentationRegistry.getInstrumentation().targetContext
        check(target.packageName == "com.anpfuel.app.enterprisevalidation")
        // Reuse an existing exact fixture account without logout, data clearing or
        // replacement of another account's encrypted state.
        if (sessions.load() == null) assertTrue(auth.loginWithKey(EnterpriseDeviceScenario.ACCOUNT_KEY) is AuthApiResult.Ok)
        check(sessions.load()?.accountId == EnterpriseDeviceScenario.ACCOUNT)
    }

    @Test
    fun actualProtectedStatusUsesTheContextOwnerAndWithdrawsBeforeSameEpochCompanyHandoff(): Unit = runBlocking {
        val graph = Graph(InstrumentationRegistry.getInstrumentation().targetContext)
        val models = ViewModelStore()
        val sourceCalls = AtomicInteger()
        var personal = false
        try {
            graph.seed()
            lateinit var contextModel: EnterpriseContextViewModel
            lateinit var syncModel: EnterpriseSyncViewModel
            lateinit var assetsModel: MyAssetsViewModel
            lateinit var tripModel: TripViewModel
            lateinit var receiptModel: ReceiptCaptureViewModel
            lateinit var fuelingModel: FuelingViewModel
            InstrumentationRegistry.getInstrumentation().runOnMainSync {
                contextModel = EnterpriseContextViewModel(graph.contexts, graph.network, fence)
                syncModel = EnterpriseSyncViewModel(graph.statusFlow, graph.network, fence)
                assetsModel = MyAssetsViewModel(graph.assetsFlow, graph.network, fence)
                tripModel = TripViewModel(graph.tripFlow, graph.network, fence, EnterpriseDeliveryActivity())
                // EMP-DOC-004: capture stays off here (no attachment queue in this graph).
                receiptModel = ReceiptCaptureViewModel(AndroidReceiptOriginals(InstrumentationRegistry.getInstrumentation().targetContext), false)
                models.put("context", contextModel); models.put("status", syncModel); models.put("assets", assetsModel); models.put("trips", tripModel)
                models.put("receipts", receiptModel)
                // EMP-FUEL-011: the fueling owner over the same protected store; no fueling action runs here.
                fuelingModel = FuelingViewModel(graph.fuelingFlow, graph.network, fence, EnterpriseDeliveryActivity())
                models.put("fuelings", fuelingModel)
            }
            val configuration = Configuration(compose.activity.resources.configuration).apply { setLocale(Locale.ENGLISH) }
            val localized = ContextThemeWrapper(compose.activity, 0).apply { applyOverrideConfiguration(configuration) }
            compose.setContent {
                CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                    MaterialTheme { EnterpriseContextScreen({ personal = true }, { sourceCalls.incrementAndGet() },
                        { sourceCalls.incrementAndGet() }, contextModel, syncModel, assetsModel, tripModel, receiptModel, fuelingModel) }
                }
            }
            awaitTag(option(graph.a))
            compose.runOnIdle {
                assertTrue("Private context did not protect its actual Activity window",
                    compose.activity.window.attributes.flags and WindowManager.LayoutParams.FLAG_SECURE != 0)
            }
            compose.onNodeWithTag(option(graph.a)).performScrollTo().performClick()
            await { contextModel.state.value.selected == graph.a && contextModel.state.value.selectionConfirmed && !syncModel.state.value.busy && syncModel.state.value.totals != null }
            compose.onNodeWithTag("enterprise-sync-open").performScrollTo().performClick()
            awaitTag("enterprise-sync-panel")
            awaitReady(syncModel, graph.a, graph.first, "initial")
            compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag(row(graph.first)))
            compose.onNodeWithTag(row(graph.first)).performClick()
            await { syncModel.state.value.detail?.item?.identity?.operationId == graph.first.operationId }
            compose.onNodeWithText("The record format could not be accepted").performScrollTo().assertIsDisplayed()
            compose.onNodeWithText("Back to list").performScrollTo().performClick()
            await { syncModel.state.value.detail == null && !syncModel.state.value.busy }
            compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasText("Back to company"))
            compose.onNodeWithText("Back to company").performClick()
            awaitTag("enterprise-home")

            val contactsBeforeOffline = graph.contacts.get()
            graph.online.value = false
            await { !contextModel.connected.value && !contextModel.state.value.selectionConfirmed }
            await { !syncModel.state.value.busy }
            compose.onNodeWithTag("enterprise-sync-open").performScrollTo().performClick()
            awaitTag("enterprise-sync-panel")
            awaitReady(syncModel, graph.a, graph.first, "offline")
            compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag(row(graph.first)))
            compose.onNodeWithTag(row(graph.first)).assertIsDisplayed()
            compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag("enterprise-sync-check"))
            compose.onNodeWithTag("enterprise-sync-check").assertIsNotEnabled()
            assertEquals("Offline status unexpectedly performed source contact", contactsBeforeOffline, graph.contacts.get())
            compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasText("Back to company"))
            compose.onNodeWithText("Back to company").performClick()
            graph.online.value = true
            await { contextModel.connected.value && !syncModel.state.value.busy }

            compose.onNodeWithTag("enterprise-context-switch").performScrollTo().performClick()
            awaitTag(option(graph.b))
            val heldEpoch = fence.epoch
            graph.holdB = true
            compose.onNodeWithTag(option(graph.b)).performScrollTo().performClick()
            await { graph.bContactStarted.isCompleted }
            // The actual protected contact is waiting before commitHandoff. Both
            // source and UI epochs still match A, so epoch-only guards cannot pass.
            assertEquals("Company contact unexpectedly handed off early", heldEpoch, fence.epoch)
            assertTrue("Parent selection changed before protected contact", contextModel.state.value.selected == graph.a)
            assertNull("Replacement retained private detail", syncModel.state.value.detail)
            assertTrue("Replacement retained private rows", syncModel.state.value.items.isEmpty())
            assertNull("Replacement retained private totals", syncModel.state.value.totals)
            compose.onNodeWithTag(row(graph.first)).assertDoesNotExist()
            graph.releaseB.complete(Unit)
            await { contextModel.state.value.selected == graph.b && contextModel.state.value.selectionConfirmed && !syncModel.state.value.busy && syncModel.state.value.totals != null }
            compose.onNodeWithTag("enterprise-context-header-name").assertTextContains(graph.b.organizationName)
            compose.onNodeWithTag("enterprise-sync-open").performScrollTo().performClick()
            awaitTag("enterprise-sync-panel")
            awaitReady(syncModel, graph.b, graph.second, "replacement")
            compose.onNodeWithTag("enterprise-sync-panel").performScrollToNode(hasTestTag(row(graph.second)))
            compose.onNodeWithTag(row(graph.second)).assertIsDisplayed()
            compose.onNodeWithTag(row(graph.first)).assertDoesNotExist()

            compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
            assertTrue("Stopped screen retained rows", syncModel.state.value.items.isEmpty())
            assertNull("Stopped screen retained detail", syncModel.state.value.detail)
            compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
            awaitTag("enterprise-home")
            await { !syncModel.state.value.busy }
            compose.onNodeWithTag("enterprise-context-switch").performScrollTo().performClick()
            awaitTag("enterprise-context-personal")
            compose.onNodeWithTag("enterprise-context-personal").performScrollTo().performClick()
            await { personal && contextModel.state.value.selected == null }
            assertTrue(syncModel.state.value.items.isEmpty())
            assertNull(syncModel.state.value.context)
            assertEquals("Status opened an unrelated account route", 0, sourceCalls.get())
            graph.assertOriginalsPreserved()
        } finally {
            graph.releaseB.complete(Unit)
            InstrumentationRegistry.getInstrumentation().runOnMainSync { models.clear() }
            graph.cleanup()
        }
    }

    /**
     * EMP-FLEET-013 AC1/AC2/AC4/AC5 through the actual context screen and the actual Keystore-protected
     * reference cache: online pull, offline reopen without contact, organization switch and the
     * untouched personal vehicle table. Feed answers are synthetic named fixtures.
     */
    @Test
    fun myAssetsShowOnlyTheSelectedOrganizationsProtectedCacheOnlineAndOffline(): Unit = runBlocking {
        val graph = Graph(InstrumentationRegistry.getInstrumentation().targetContext)
        val models = ViewModelStore()
        val vehiclesBefore = database.vehicleDao().listAll().map { it.id to it.displayName }
        // The synthetic account API answers a refresh only for its in-process family. Align it with the
        // retained session so the delivery-proof refresh takes the real path, never a synthetic revocation.
        scenario.familyId = checkNotNull(sessions.load()).familyId
        try {
            graph.seed()
            graph.serverAssets[graph.a.partition.organizationId] = listOf(graph.asset("Caminhão sintético A1"), graph.asset("Van sintética A2"))
            graph.serverAssets[graph.b.partition.organizationId] = listOf(graph.asset("Escavadeira sintética B1"))
            lateinit var contextModel: EnterpriseContextViewModel
            lateinit var syncModel: EnterpriseSyncViewModel
            lateinit var assetsModel: MyAssetsViewModel
            lateinit var tripModel: TripViewModel
            lateinit var receiptModel: ReceiptCaptureViewModel
            lateinit var fuelingModel: FuelingViewModel
            InstrumentationRegistry.getInstrumentation().runOnMainSync {
                contextModel = EnterpriseContextViewModel(graph.contexts, graph.network, fence)
                syncModel = EnterpriseSyncViewModel(graph.statusFlow, graph.network, fence)
                assetsModel = MyAssetsViewModel(graph.assetsFlow, graph.network, fence)
                tripModel = TripViewModel(graph.tripFlow, graph.network, fence, EnterpriseDeliveryActivity())
                // EMP-DOC-004: capture stays off here (no attachment queue in this graph).
                receiptModel = ReceiptCaptureViewModel(AndroidReceiptOriginals(InstrumentationRegistry.getInstrumentation().targetContext), false)
                models.put("context", contextModel); models.put("status", syncModel); models.put("assets", assetsModel); models.put("trips", tripModel)
                models.put("receipts", receiptModel)
                // EMP-FUEL-011: the fueling owner over the same protected store; no fueling action runs here.
                fuelingModel = FuelingViewModel(graph.fuelingFlow, graph.network, fence, EnterpriseDeliveryActivity())
                models.put("fuelings", fuelingModel)
            }
            val configuration = Configuration(compose.activity.resources.configuration).apply { setLocale(Locale.ENGLISH) }
            val localized = ContextThemeWrapper(compose.activity, 0).apply { applyOverrideConfiguration(configuration) }
            compose.setContent {
                CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                    MaterialTheme { EnterpriseContextScreen({ }, { }, { }, contextModel, syncModel, assetsModel, tripModel, receiptModel, fuelingModel) }
                }
            }
            fun names() = assetsModel.state.value.snapshot?.assets?.map { it.asset.name }
            fun ready(expected: EnterpriseContextSummary) = assetsModel.state.value.let {
                it.loaded && !it.busy && it.error == null && it.context?.partition == expected.partition && it.epoch == fence.epoch
            }
            fun awaitAssets(expected: EnterpriseContextSummary, wanted: List<String>, stage: String) {
                try { await { ready(expected) && names() == wanted } }
                catch (_: Throwable) {
                    val value = assetsModel.state.value
                    throw AssertionError("My assets not ready at " + stage + "; busy=" + value.busy + "; loaded=" + value.loaded +
                        "; error=" + (value.error?.name ?: "NONE") + "; refresh_failed=" + value.refreshFailed +
                        "; unlock=" + value.unlockRequired + "; unrecoverable=" + value.unrecoverable +
                        "; context_matches=" + (value.context?.partition == expected.partition) + "; epoch_matches=" + (value.epoch == fence.epoch) +
                        "; snapshot=" + (value.snapshot != null) + "; count=" + (value.snapshot?.assets?.size ?: -1) +
                        "; pulls=" + graph.referencePulls.get() + "; failure=" + (graph.lastReferenceFailure?.javaClass?.simpleName ?: "NONE"))
                }
            }
            awaitTag(option(graph.a))
            compose.onNodeWithTag(option(graph.a)).performScrollTo().performClick()
            await { contextModel.state.value.selected == graph.a && contextModel.state.value.selectionConfirmed && !syncModel.state.value.busy }
            compose.onNodeWithTag("enterprise-assets-open").performScrollTo().performClick()
            awaitAssets(graph.a, listOf("Caminhão sintético A1", "Van sintética A2"), "online")
            assertEquals(1, graph.referencePulls.get())
            compose.onNodeWithText("Caminhão sintético A1").assertIsDisplayed()
            compose.onNodeWithTag("enterprise-assets-confirmed-at").assertExists()
            compose.onNodeWithTag("enterprise-assets-back").performClick()
            awaitTag("enterprise-home")

            graph.online.value = false
            await { !contextModel.connected.value && !assetsModel.connected.value }
            compose.onNodeWithTag("enterprise-assets-open").performScrollTo().performClick()
            awaitAssets(graph.a, listOf("Caminhão sintético A1", "Van sintética A2"), "offline")
            assertEquals("The offline list contacted the feed", 1, graph.referencePulls.get())
            compose.onNodeWithTag("enterprise-assets-offline").assertExists()
            compose.onNodeWithTag("enterprise-assets-refresh").assertIsNotEnabled()
            compose.onNodeWithText("Van sintética A2").assertIsDisplayed()
            compose.onNodeWithTag("enterprise-assets-back").performClick()
            awaitTag("enterprise-home")
            graph.online.value = true
            await { contextModel.connected.value && assetsModel.connected.value && !syncModel.state.value.busy }

            compose.onNodeWithTag("enterprise-context-switch").performScrollTo().performClick()
            awaitTag(option(graph.b))
            compose.onNodeWithTag(option(graph.b)).performScrollTo().performClick()
            await { contextModel.state.value.selected == graph.b && contextModel.state.value.selectionConfirmed && !syncModel.state.value.busy }
            assertNull("The organization switch retained the other organization's assets", assetsModel.state.value.snapshot)
            compose.onNodeWithTag("enterprise-assets-open").performScrollTo().performClick()
            awaitAssets(graph.b, listOf("Escavadeira sintética B1"), "switched")
            compose.onNodeWithText("Escavadeira sintética B1").assertIsDisplayed()
            compose.onAllNodes(hasText("Caminhão sintético A1")).assertCountEquals(0)
            assertEquals(2, graph.referencePulls.get())

            compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
            assertNull("A stopped screen retained assets", assetsModel.state.value.snapshot)
            compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
            awaitTag("enterprise-home")
            assertEquals("The personal vehicle list changed", vehiclesBefore, database.vehicleDao().listAll().map { it.id to it.displayName })
        } finally {
            InstrumentationRegistry.getInstrumentation().runOnMainSync { models.clear() }
            graph.cleanup()
        }
    }

    private inner class Graph(context: Context) {
        private val original = checkNotNull(sessions.load())
        val online = MutableStateFlow(true)
        val network = object : NetworkConnectivityGateway { override fun observeIsConnected() = online }
        val contacts = AtomicInteger()
        @Volatile var holdB = false
        val bContactStarted = CompletableDeferred<Unit>()
        val releaseB = CompletableDeferred<Unit>()
        val a = summary("Synthetic protected company A")
        val b = summary("Synthetic protected company B")
        private val enrollments = listOf(a, b).associate { it.partition to UUID.randomUUID().toString() }
        private val actualKeys = EnterpriseModule.vaultKeys()
        private val previousAliases = actualKeys.aliases("abastevo_enterprise_v2_")
        private val created = Collections.synchronizedSet(mutableSetOf<String>())
        private val keys = object : EnterpriseVaultKeys by actualKeys {
            override fun create(alias: String, managedUnlockSeconds: Int?): EnterpriseVaultKey =
                actualKeys.create(alias, managedUnlockSeconds).also { created += alias; assertNull("Android vault key was exportable", it.secret.encoded) }
        }
        private val actualFiles = EnterpriseModule.vaultFiles(context)
        private val denialTokens = Collections.synchronizedSet(mutableSetOf<String>())
        private val files = object : EnterpriseVaultFiles by actualFiles {
            override fun deny(token: String) { actualFiles.deny(token); denialTokens += token }
        }
        private val erasure = EnterpriseVaultErasure(files, keys, fence)
        private val clock = AndroidEnterpriseDeviceClock(context)
        private val identity get() = sessions.load()?.let { EnterpriseLocalIdentity(it.accountId, ApiEnvironment.PREVIEW.origin, it.familyId) }
        private val fixtureCustody = EnterpriseContactAuthority { request ->
            check(request.binding == binding() && request.target.partition in enrollments)
            if (holdB && request.target.partition == b.partition) {
                bContactStarted.complete(Unit)
                withTimeout(15_000) { releaseB.await() }
            }
            contacts.incrementAndGet()
            EnterpriseContactReply.Active(EnterpriseContactGrant(EnterpriseEnrollmentBinding(request.target,
                enrollments.getValue(request.target.partition), 1), original.familyId, System.currentTimeMillis()))
        }
        private val store = ProtectedEnterprisePersistence(files, keys, fence, erasure, { identity }, clock::read, fixtureCustody)
        private val queue = ProtectedEnterpriseQueue(store)
        private val source = EnterpriseOperationSessionSource(ApiEnvironment.PREVIEW.origin, sessions, auth, fence, store::protectedEnrollment)
        private val local = EnterpriseLocalContext({ sessions.load()?.accountId }, { ApiEnvironment.PREVIEW.origin }, store, { fence.epoch })
        private val gateway: EnterpriseContextGateway = object : EnterpriseContextGateway {
            override fun currentBinding(): EnterpriseAccessBinding? = sessions.load()?.let {
                EnterpriseAccessBinding(it.accountId, ApiEnvironment.PREVIEW.origin, it.familyId, fence.epoch)
            }
            override suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding> = EnterpriseAccessResult.Ok(binding())
            override suspend fun list(binding: EnterpriseAccessBinding, afterOrganizationId: String?): EnterpriseAccessResult<EnterpriseContextPage> {
                check(binding == this@Graph.binding()); return EnterpriseAccessResult.Ok(EnterpriseContextPage(listOf(a, b)))
            }
            override suspend fun read(binding: EnterpriseAccessBinding, context: EnterpriseContextSummary): EnterpriseAccessResult<EnterpriseContextSummary> {
                check(binding == this@Graph.binding() && context in listOf(a, b)); return EnterpriseAccessResult.Ok(context)
            }
            override fun invalidate() = Unit
        }
        val contexts = EnterpriseContextFlow(gateway, local, store, fence)
        private val activity = EnterpriseDeliveryActivity()
        private val delivery = EnterpriseQueueService(source, queue,
            object : EnterpriseOperationTransport {
                override suspend fun deliver(attempt: EnterpriseQueueAttempt, operations: List<EnterpriseOperation>): EnterpriseOperationDelivery =
                    throw AssertionError("Presentation acceptance must not perform network delivery")
            }, EnterpriseQueueRegistry(), clock::read, entropy = { 0 }, activity = activity)
        val statusFlow = EnterpriseSyncFlow(source, ProtectedEnterpriseSyncStatusStore(store), delivery, activity, fence)
        /** EMP-FLEET-013 synthetic feed answers per organization; the cache itself is the actual protected store. */
        val referencePulls = AtomicInteger()
        val serverAssets = Collections.synchronizedMap(mutableMapOf<String, List<String>>())
        @Volatile var lastReferenceFailure: Throwable? = null
        private val referenceTransport = EnterpriseReferenceTransport { attempt, cursor, held ->
            referencePulls.incrementAndGet()
            try { synthetic(attempt, cursor, held) } catch (failure: Throwable) { lastReferenceFailure = failure; throw failure }
        }
        private fun synthetic(attempt: EnterpriseQueueAttempt, cursor: String?, held: Long?): EnterpriseReferencePull {
            check(graphOnline()) { "An offline view reached the reference feed" }
            val session = checkNotNull(sessions.load())
            val request = ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(UUID.randomUUID().toString(), session.familyId,
                session.accessToken, attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId, cursor))
            val assets = serverAssets[attempt.partition.organizationId].orEmpty()
            val changes = assets.mapIndexed { index, value ->
                val entity = JSONObject(value).getString("id")
                """{"revision":${index + 1},"entity_type":"asset","entity_id":"$entity","op":"UPSERT","asset":$value}"""
            }
            val raw = """{"schema_version":1,"request_id":"${request.snapshot().requestId}","membership_id":"${attempt.enrollment.target.membershipId}","enrollment_id":"${attempt.enrollment.enrollmentId}","mode":"RESET","reason":"${if (cursor == null) "NEW_CURSOR" else "CURSOR_EXPIRED"}","from_revision":null,"revision":${assets.size},"confirmed_at":"${EnterpriseTime.formatInstant(Instant.now())}","cursor":"synthetic-${assets.size}","entity_types":["asset"],"changes":[${changes.joinToString(",")}]}"""
            return EnterpriseReferencePull(request, ReferenceFeedResponse.parse(raw.toByteArray(), request, held))
        }
        private fun graphOnline() = online.value
        val assetsFlow = MyAssetsFlow(source, EnterpriseReferenceService(source, ProtectedReferenceCacheStore(store), referenceTransport, clock::read),
            fence, clock::read)
        /** EMP-TRIP-005: the trip owner over the same protected store; no trip action runs in these methods. */
        val tripFlow = TripFlow(source, EnterpriseTripService(source, delivery::save, ProtectedTripStore(store), EnterpriseTripStartHints(),
            clock::read, { ZoneId.systemDefault() }), fence) { account, partition -> delivery.dispatch(account, partition) }
        val fuelingFlow = FuelingFlow(source, EnterpriseFuelingService(source, delivery::save, ProtectedFuelingStore(store), { ZoneId.systemDefault() }),
            fence) { account, partition -> delivery.dispatch(account, partition) }
        fun asset(name: String): String = """{"id":"${UUID.randomUUID()}","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"$name","fleet_number":"SYN-1","plate":"","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[],"assignment":{"kind":"PRIMARY","valid_to":null}}"""
        lateinit var first: EnterpriseOperation
        lateinit var second: EnterpriseOperation

        private fun summary(name: String) = EnterpriseContextSummary(EnterprisePartition.of(original.accountId, ApiEnvironment.PREVIEW.origin,
            UUID.randomUUID().toString()), UUID.randomUUID().toString(), name)
        private fun binding(): EnterpriseAccessBinding = checkNotNull(gateway.currentBinding())
        private suspend fun attempt(partition: EnterprisePartition): EnterpriseQueueAttempt = source.attempt(source.account(), partition)
        suspend fun seed() {
            for (context in listOf(a, b)) {
                val before = binding()
                val owner = local.reserveContact(local.acquireOwner(), before, fence, ::binding)
                val handoff = store.confirm(EnterpriseDeviceTarget(context.partition, context.membershipId), before, owner)
                local.select(local.consumeContact(owner, handoff, binding()), context.partition)
            }
            first = operation(a); second = operation(b)
            for (op in listOf(first, second)) assertTrue(queue.save(attempt(partition(op)), op) {
                it.put(EnterpriseLocalRecord(partition(op), op.operationId, "SYNTHETIC-PRESERVED-UI-RECORD".toByteArray()))
            })
            val raw = JSONObject().put("schema_version", 1).put("operation_id", first.operationId).put("payload_digest", first.payloadDigest)
                .put("result", "REJECTED").put("code", "MALFORMED").put("entity_revision", JSONObject.NULL)
                .put("received_at", EnterpriseTime.formatInstant(Instant.now())).put("review_item_id", JSONObject.NULL).put("warnings", JSONArray())
            assertTrue(queue.acknowledge(attempt(a.partition), first, OperationAcknowledgement.parse(raw.toString().toByteArray(), first)) { })
        }
        private fun operation(context: EnterpriseContextSummary): EnterpriseOperation {
            val id = UUID.randomUUID().toString()
            val raw = JSONObject().put("schema_version", 1).put("operation_id", id).put("owner_account_id", original.accountId)
                .put("api_origin", ApiEnvironment.PREVIEW.origin).put("org_id", context.partition.organizationId).put("membership_id", context.membershipId)
                .put("enrollment_id", enrollments.getValue(context.partition)).put("type", "validation.context_status").put("operation_schema", 1)
                .put("entity", JSONObject().put("type", "record").put("id", id).put("expected_revision", JSONObject.NULL))
                .put("occurred_at", EnterpriseTime.formatInstant(Instant.now())).put("entry_zone", "UTC").put("device_offset_seconds", 0)
                .put("depends_on", JSONArray()).put("soft_references", JSONArray()).put("attachments", JSONArray())
                .put("payload", JSONObject().put("value", "SYNTHETIC-PROTECTED-UI-STATUS"))
            return EnterpriseOperation.seal(raw.toString().toByteArray(), SyncBinding.of(original.accountId, ApiEnvironment.PREVIEW.origin, context.partition.organizationId))
        }
        private fun partition(operation: EnterpriseOperation) = EnterprisePartition.of(original.accountId, ApiEnvironment.PREVIEW.origin, operation.binding.organizationId)
        suspend fun assertOriginalsPreserved() {
            assertTrue("UI changed the original encrypted session", sessions.load() == original)
            for (op in listOf(first, second)) {
                val entry = queue.entry(attempt(partition(op)), op.operationId) ?: throw AssertionError("Original protected operation disappeared")
                assertArrayEquals(op.canonicalBytes, entry.operation.canonicalBytes)
                assertFalse("Context navigation dismissed a refusal", entry.dismissedRejected)
            }
        }
        fun cleanup() {
            for (context in listOf(a, b)) assertTrue("Scoped synthetic UI cleanup failed", erasure.partitionRevoked(context.partition))
            // These exact aliases/tombstones were created through this graph only;
            // never enumerate-delete another account or another partition's handles.
            created.toList().forEach(actualKeys::delete)
            denialTokens.toList().forEach(actualFiles::allowFresh)
            assertTrue("Synthetic UI keys survived cleanup", created.none { actualKeys.existing(it) != null })
            assertTrue("Scoped UI cleanup removed another partition key", previousAliases.all { actualKeys.existing(it) != null })
        }
    }

    /** A mounted panel can still be loading a newly captured protected snapshot. */
    private fun awaitReady(model: EnterpriseSyncViewModel, expected: EnterpriseContextSummary,
        operation: EnterpriseOperation, stage: String) {
        fun ready(): Boolean {
            val value = model.state.value
            return !value.busy && value.error == null && !value.unlockRequired && !value.unrecoverable &&
                value.epoch == fence.epoch && value.context?.partition == expected.partition &&
                value.context?.membershipId == expected.membershipId && value.items.any {
                    it.identity.operationId == operation.operationId && it.identity.payloadDigest == operation.payloadDigest
                }
        }
        try { await { ready() } }
        catch (_: Throwable) {
            val value = model.state.value
            throw AssertionError("Protected status snapshot not ready at " + stage +
                "; busy=" + value.busy + "; error=" + (value.error?.name ?: "NONE") +
                "; unlock=" + value.unlockRequired + "; unrecoverable=" + value.unrecoverable +
                "; epoch_matches=" + (value.epoch == fence.epoch) +
                "; context_matches=" + (value.context?.partition == expected.partition && value.context?.membershipId == expected.membershipId) +
                "; item_count=" + value.items.size + "; expected_item_present=" + value.items.any {
                    it.identity.operationId == operation.operationId && it.identity.payloadDigest == operation.payloadDigest
                })
        }
    }

    private fun option(context: EnterpriseContextSummary) = "enterprise-context-option-${context.partition.organizationId}"
    private fun row(operation: EnterpriseOperation) = "enterprise-sync-item-${operation.operationId}"
    private fun awaitTag(tag: String) = await { compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty() }
    private fun await(condition: () -> Boolean) = compose.waitUntil(15_000, condition)
}
