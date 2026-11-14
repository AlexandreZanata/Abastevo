package com.anpfuel.app.ui.enterprise

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
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.application.enterprise.sync.EnterpriseSyncFlow
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.enterprise.*
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
            InstrumentationRegistry.getInstrumentation().runOnMainSync {
                contextModel = EnterpriseContextViewModel(graph.contexts, graph.network, fence)
                syncModel = EnterpriseSyncViewModel(graph.statusFlow, graph.network, fence)
                models.put("context", contextModel); models.put("status", syncModel)
            }
            val configuration = Configuration(compose.activity.resources.configuration).apply { setLocale(Locale.ENGLISH) }
            val localized = ContextThemeWrapper(compose.activity, 0).apply { applyOverrideConfiguration(configuration) }
            compose.setContent {
                CompositionLocalProvider(LocalContext provides localized, LocalConfiguration provides configuration) {
                    MaterialTheme { EnterpriseContextScreen({ personal = true }, { sourceCalls.incrementAndGet() },
                        { sourceCalls.incrementAndGet() }, contextModel, syncModel) }
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
