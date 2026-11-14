package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.sync.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.*
import kotlinx.coroutines.*
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

internal fun syncId(n: Int) = "7e000000-0000-4000-8000-" + n.toString().padStart(12, '0')
internal const val SYNC_ORIGIN = "https://status.example.invalid"
internal fun syncOperation(context: EnterpriseContextSummary, n: Int = 10): EnterpriseOperation {
    val p = context.partition
    return EnterpriseOperation.seal("""{"schema_version":1,"operation_id":"${syncId(n)}","owner_account_id":"${p.ownerAccount}","api_origin":"${p.origin}","org_id":"${p.organizationId}","membership_id":"${context.membershipId}","enrollment_id":"${syncId(4)}","type":"validation.status","operation_schema":1,"entity":{"type":"record","id":"${syncId(8)}","expected_revision":null},"occurred_at":"2026-11-13T00:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"SYNTHETIC-PRIVATE-STATUS"}}""".toByteArray(), SyncBinding.of(p.ownerAccount, p.origin, p.organizationId))
}
internal fun syncAck(operation: EnterpriseOperation): OperationAcknowledgement = OperationAcknowledgement.parse(
    """{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T00:01:00Z","review_item_id":null,"warnings":[]}""".toByteArray(), operation)

internal class SyncFlowFixture {
    val fence = EnterpriseSessionFence()
    val first = EnterpriseContextSummary(EnterprisePartition.of(syncId(1), SYNC_ORIGIN, syncId(2)), syncId(3), "Synthetic A")
    val second = EnterpriseContextSummary(EnterprisePartition.of(syncId(1), SYNC_ORIGIN, syncId(20)), syncId(30), "Synthetic B")
    var identity = EnterpriseLocalIdentity(syncId(1), SYNC_ORIGIN, syncId(9))
    var offlineCaptures = 0; var deliveryCaptures = 0; var pageCalls = 0; var detailCalls = 0; var dismissCalls = 0; var networkCalls = 0
    val now = EnterpriseDeviceClock("boot", 1000, 10000)
    var gate: CompletableDeferred<Unit>? = null
    var blockedOrganization: String? = null
    var pageError: Exception? = null
    var pageErrorOrganization: String? = null
    var latePageChanged = false
    var detailGate: CompletableDeferred<Unit>? = null
    var detailStarted = CompletableDeferred<Unit>()
    var deliveryGate: CompletableDeferred<Unit>? = null
    var deliveryStarted = CompletableDeferred<Unit>()
    var membershipOverride: String? = null
    var afterAccountCapture: (() -> Unit)? = null
    var tokenVersion = 1
    var refreshTokenOnDelivery = false
    var attemptGate: CompletableDeferred<Unit>? = null
    val attemptStarted = CompletableDeferred<Unit>()
    var retainedHook: (() -> Unit)? = null
    var accountError: Exception? = null
    var queueRetry = QueueRetryState(1, now, 120000)
    private val receipts = mutableMapOf<String, OperationAcknowledgement>()
    var next: EnterpriseSyncCursor? = null
    var changedOnce = false
    var lastDismiss: List<String>? = null
    val activity = EnterpriseDeliveryActivity()
    private var nonce = 100
    val source = object : EnterpriseQueueSource {
        override suspend fun account(): EnterpriseQueueAccountLease {
            offlineCaptures++
            accountError?.let { throw it }
            val captured = identity; val epoch = fence.epoch; val background = fence.backgroundEpoch; val token = tokenVersion
            return EnterpriseQueueAccountLease.capture(captured, epoch, background, syncId(nonce++)) {
                val retained = identity == captured && fence.epoch == epoch && fence.backgroundEpoch == background && tokenVersion == token
                retainedHook?.invoke()
                retained
            }.also { afterAccountCapture?.invoke() }
        }
        override suspend fun deliveryAccount(): EnterpriseQueueAccountLease {
            deliveryCaptures++
            if (refreshTokenOnDelivery) tokenVersion++
            val captured = account()
            deliveryStarted.complete(Unit)
            deliveryGate?.let { withContext(NonCancellable) { it.await() } }
            return captured
        }
        override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
            account.assertCurrent()
            attemptGate?.let { attemptStarted.complete(Unit); withContext(NonCancellable) { it.await() } }
            val selected = if (partition == first.partition) first else second
            return account.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, membershipOverride ?: selected.membershipId), syncId(4), 1))
        }
    }
    fun context(attempt: EnterpriseQueueAttempt) = if (attempt.partition == first.partition) first else second
    private fun operation(context: EnterpriseContextSummary) = syncOperation(context, if (context == first) 10 else 11)
    fun item(context: EnterpriseContextSummary): EnterpriseSyncItem {
        val operation = operation(context)
        return EnterpriseSyncProjection.item(EnterpriseQueueEntry(operation, acknowledgement = receipts[operation.operationId]))
    }
    val store = object : EnterpriseSyncStatusStore {
        override suspend fun page(attempt: EnterpriseQueueAttempt, mode: EnterpriseSyncMode, cursor: EnterpriseSyncCursor?, limit: Int): EnterpriseSyncPage {
            pageCalls++
            val items = listOf(item(context(attempt)))
            if (attempt.partition.organizationId == blockedOrganization) gate?.let {
                withContext(NonCancellable) { it.await() }
                if (latePageChanged) { latePageChanged = false; throw EnterpriseSyncPageChanged() }
            }
            if (pageErrorOrganization == null || pageErrorOrganization == attempt.partition.organizationId) pageError?.let { throw it }
            if (cursor != null && changedOnce) { changedOnce = false; throw EnterpriseSyncPageChanged() }
            return EnterpriseSyncPage(items, EnterpriseSyncProjection.totals(items, EnterpriseQueueStats(1, 10, 0, 2, 2048)),
                1, if (cursor == null) next else null, mode)
        }
        override suspend fun detail(attempt: EnterpriseQueueAttempt, operationId: String, digest: String): EnterpriseSyncDetail {
            detailCalls++; detailStarted.complete(Unit)
            detailGate?.let { withContext(NonCancellable) { it.await() } }
            val item = item(context(attempt))
            require(item.identity == EnterpriseOperationIdentity(operationId, digest))
            return EnterpriseSyncDetail(item, emptyList())
        }
        override suspend fun dismissRejected(attempt: EnterpriseQueueAttempt, operationId: String, digest: String, expectedReceiptDigest: String): Boolean {
            dismissCalls++; lastDismiss = listOf(attempt.partition.organizationId, operationId, digest, expectedReceiptDigest); return true
        }
    }
    private val producer = object : EnterpriseQueueProducer {
        override fun descriptor() = EnterpriseQueueProducerDescriptor("validation.status")
        override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
            tx.put(EnterpriseLocalRecord(attempt.partition, operation.entity.reference.id, operation.operationId.toByteArray()))
        }
        override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, ack: OperationAcknowledgement) {
            tx.put(EnterpriseLocalRecord(attempt.partition, operation.entity.reference.id, ack.result.name.toByteArray()))
        }
    }
    private val queue = object : EnterpriseOperationQueue {
        private val records = mutableMapOf<String, EnterpriseLocalRecord>()
        private val tx = object : EnterpriseQueueTransaction {
            override fun record(id: String) = records[id]
            override fun put(record: EnterpriseLocalRecord) { records[record.id] = record }
        }
        override suspend fun save(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, project: (EnterpriseQueueTransaction) -> Unit) = false
        override suspend fun pending(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry> {
            val operation = operation(context(attempt))
            return if (operation.operationId in receipts) emptyList() else listOf(EnterpriseQueueEntry(operation, queueRetry))
        }
        override suspend fun entry(attempt: EnterpriseQueueAttempt, operationId: String): EnterpriseQueueEntry {
            val operation = operation(context(attempt))
            require(operation.operationId == operationId)
            return EnterpriseQueueEntry(operation, queueRetry, receipts[operationId])
        }
        override suspend fun acknowledge(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, ack: OperationAcknowledgement, project: (EnterpriseQueueTransaction) -> Unit): Boolean {
            project(tx); receipts[operation.operationId] = ack; return true
        }
        override suspend fun retry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, expected: QueueRetryState, next: QueueRetryState) = false
        override suspend fun stats(attempt: EnterpriseQueueAttempt) = EnterpriseQueueStats(1, 10, 0, 2, 2048)
    }
    val service = EnterpriseQueueService(source, queue, EnterpriseOperationTransport { _, operations ->
        networkCalls++; EnterpriseOperationDelivery(operations.map(::syncAck))
    }, EnterpriseQueueRegistry(producer), { now }, { 0 }, activity = activity)
    val flow = EnterpriseSyncFlow(source, store, service, activity, fence)
}

class EnterpriseSyncFlowTest {
    @Test fun offlineOpenAndWithdrawalNeverAcquireAnotherSelectionOwner(): Unit = runTest {
        val f = SyncFlowFixture(); f.flow.open(f.first, f.fence.epoch)
        assertEquals(f.first, f.flow.state.value.context)
        assertEquals(1, f.flow.state.value.items.size); assertEquals(0, f.deliveryCaptures); assertEquals(0, f.networkCalls)
        val before = f.flow.state.value.generation
        f.flow.withdraw()
        assertTrue(f.flow.state.value.generation > before); assertTrue(f.flow.state.value.items.isEmpty())
        assertNull(f.flow.state.value.detail); assertNull(f.flow.state.value.context)
    }

    @Test fun latePageFromReplacedContextCannotAttachToNewVisibleOwner(): Unit = runTest {
        val f = SyncFlowFixture(); f.gate = CompletableDeferred(); f.blockedOrganization = f.first.partition.organizationId
        val old = launch(start = CoroutineStart.UNDISPATCHED) { f.flow.open(f.first, f.fence.epoch) }
        f.flow.open(f.second, f.fence.epoch)
        assertEquals(f.second, f.flow.state.value.context)
        f.gate!!.complete(Unit); old.join()
        assertEquals(f.second, f.flow.state.value.context)
        assertEquals(listOf(f.item(f.second).identity), f.flow.state.value.items.map { it.identity })
    }

    @Test fun renderedOldGesturesCannotReadDismissOrSendReplacementOrganization(): Unit = runTest {
        val f = SyncFlowFixture(); f.flow.open(f.first, f.fence.epoch)
        val old = f.flow.state.value; val item = f.item(f.first)
        assertEquals(f.first, old.context); assertNotNull(old.epoch)
        val renderedEpoch = requireNotNull(old.epoch)
        f.flow.open(f.second, f.fence.epoch)
        val reads = f.pageCalls
        f.flow.showDetail(item.identity.operationId, item.identity.payloadDigest, renderedEpoch, old.generation)
        f.flow.dismiss(item.identity.operationId, item.identity.payloadDigest, "sha256:" + "0".repeat(64), renderedEpoch, old.generation)
        f.flow.manualCheck(true, renderedEpoch, old.generation)
        assertEquals(reads, f.pageCalls); assertEquals(0, f.detailCalls); assertEquals(0, f.dismissCalls); assertEquals(0, f.deliveryCaptures)
        assertEquals(f.second, f.flow.state.value.context)
    }

    @Test fun actualEpochBackgroundAndFamilyChangesClearPrivatePresentationSynchronously(): Unit = runTest {
        for (kind in listOf("epoch", "background", "family")) {
            val f = SyncFlowFixture(); f.flow.open(f.first, f.fence.epoch)
            assertFalse(f.flow.state.value.items.isEmpty())
            when (kind) {
                "epoch" -> f.fence.lock()
                "background" -> f.fence.markBackground()
                else -> f.identity = f.identity.copy(family = syncId(90))
            }
            f.flow.invalidateIfStale()
            assertTrue(f.flow.state.value.items.isEmpty()); assertNull(f.flow.state.value.detail); assertTrue(f.flow.state.value.active.isEmpty())
        }
    }

    @Test fun unlockLossAndStorageOutageCannotMasqueradeAsEmptySuccessfulQueue(): Unit = runTest {
        for (failure in listOf(EnterpriseUnlockRequired(), EnterpriseVaultUnrecoverable(), EnterprisePersistenceUnavailable())) {
            val f = SyncFlowFixture(); f.pageError = failure; f.flow.open(f.first, f.fence.epoch)
            val state = f.flow.state.value
            assertTrue(state.items.isEmpty()); assertNotNull(state.error)
            assertEquals(failure is EnterpriseUnlockRequired, state.unlockRequired)
            assertEquals(failure is EnterpriseVaultUnrecoverable, state.unrecoverable)
        }
    }

    @Test fun detailAndManualCheckUseExactVisibleScopeWithoutResettingRetryFloor(): Unit = runTest {
        val f = SyncFlowFixture(); f.flow.open(f.second, f.fence.epoch)
        var state = f.flow.state.value; assertEquals(1, state.items.size); val item = state.items.single()
        f.flow.showDetail(item.identity.operationId, item.identity.payloadDigest, state.epoch!!, state.generation)
        assertEquals(item.identity, f.flow.state.value.detail!!.item.identity)
        state = f.flow.state.value
        val epoch = requireNotNull(state.epoch)
        f.flow.manualCheck(false, epoch, state.generation)
        assertEquals(0, f.deliveryCaptures)
        f.flow.manualCheck(true, epoch, state.generation)
        assertEquals(1, f.deliveryCaptures); assertEquals(0, f.networkCalls)
        assertEquals(120000L, f.flow.state.value.manualWaitingMillis)
        assertEquals(f.second, f.flow.state.value.context)
    }

    @Test fun selectedMembershipCannotBorrowAnotherCurrentEnrollment(): Unit = runTest {
        val f = SyncFlowFixture(); f.membershipOverride = syncId(77)
        f.flow.open(f.first, f.fence.epoch)
        assertEquals(0, f.pageCalls); assertTrue(f.flow.state.value.items.isEmpty())
        assertNull(f.flow.state.value.context); assertEquals(EnterpriseAccessError.STALE, f.flow.state.value.error)
    }

    @Test fun lateDetailCannotAttachToReplacementOwnerAtSameEpoch(): Unit = runTest {
        val f = SyncFlowFixture(); f.flow.open(f.first, f.fence.epoch)
        val before = f.flow.state.value; val item = before.items.single()
        f.detailGate = CompletableDeferred()
        val old = launch(start = CoroutineStart.UNDISPATCHED) {
            f.flow.showDetail(item.identity.operationId, item.identity.payloadDigest, before.epoch!!, before.generation)
        }
        withTimeout(3000) { f.detailStarted.await() }
        f.flow.open(f.second, f.fence.epoch)
        val replacement = f.flow.state.value
        f.detailGate!!.complete(Unit); withTimeout(3000) { old.join() }
        assertEquals(replacement, f.flow.state.value); assertNull(f.flow.state.value.detail)
    }

    @Test fun latePageChangedCannotStartAnotherReadAfterOwnerReplacement(): Unit = runTest {
        val f = SyncFlowFixture(); f.gate = CompletableDeferred(); f.blockedOrganization = f.first.partition.organizationId
        f.latePageChanged = true
        val old = launch(start = CoroutineStart.UNDISPATCHED) { f.flow.open(f.first, f.fence.epoch) }
        f.flow.open(f.second, f.fence.epoch)
        val replacement = f.flow.state.value
        f.gate!!.complete(Unit); withTimeout(3000) { old.join() }
        assertEquals(2, f.pageCalls, "A retired page may not start another private read")
        assertEquals(replacement, f.flow.state.value)
    }

    @Test fun lateFailedRefreshCannotClearReplacementOwnerAtSameEpoch(): Unit = runTest {
        val f = SyncFlowFixture(); f.flow.open(f.first, f.fence.epoch)
        val before = f.flow.state.value
        f.gate = CompletableDeferred(); f.blockedOrganization = f.first.partition.organizationId
        f.pageError = EnterprisePersistenceUnavailable(); f.pageErrorOrganization = f.first.partition.organizationId
        val old = launch(start = CoroutineStart.UNDISPATCHED) { f.flow.refresh(before.epoch!!, before.generation) }
        f.flow.open(f.second, f.fence.epoch)
        val replacement = f.flow.state.value
        f.gate!!.complete(Unit); withTimeout(3000) { old.join() }
        assertEquals(replacement, f.flow.state.value); assertNull(f.flow.state.value.error)
    }

    @Test fun deliveryProofReplyCannotSendAfterOriginalViewWasReplaced(): Unit = runTest {
        val f = SyncFlowFixture(); f.flow.open(f.first, f.fence.epoch)
        val before = f.flow.state.value
        f.deliveryGate = CompletableDeferred()
        val old = launch(start = CoroutineStart.UNDISPATCHED) { f.flow.manualCheck(true, before.epoch!!, before.generation) }
        withTimeout(3000) { f.deliveryStarted.await() }
        f.flow.open(f.second, f.fence.epoch)
        val replacement = f.flow.state.value; val reads = f.pageCalls
        f.deliveryGate!!.complete(Unit); withTimeout(3000) { old.join() }
        assertEquals(reads, f.pageCalls); assertEquals(0, f.networkCalls)
        assertEquals(replacement, f.flow.state.value)
    }

    @Test fun actualFamilyDenialOutranksConcurrentStorageFailure(): Unit = runTest {
        val f = SyncFlowFixture(); f.flow.open(f.first, f.fence.epoch)
        val before = f.flow.state.value
        f.gate = CompletableDeferred(); f.blockedOrganization = f.first.partition.organizationId
        f.pageError = EnterprisePersistenceUnavailable()
        val old = launch(start = CoroutineStart.UNDISPATCHED) { f.flow.refresh(before.epoch!!, before.generation) }
        f.identity = f.identity.copy(family = syncId(90))
        f.gate!!.complete(Unit); withTimeout(3000) { old.join() }
        val result = f.flow.state.value
        assertNull(result.context); assertTrue(result.items.isEmpty()); assertNull(result.detail)
        assertEquals(EnterpriseAccessError.STALE, result.error)
    }

    @Test fun backgroundDuringAccountCaptureCannotLeaveBusyPrivateOwner(): Unit = runTest {
        val f = SyncFlowFixture(); f.afterAccountCapture = { f.fence.markBackground() }
        f.flow.open(f.first, f.fence.epoch)
        val result = f.flow.state.value
        assertEquals(0, f.pageCalls); assertFalse(result.busy)
        assertNull(result.context); assertTrue(result.items.isEmpty())
        assertEquals(EnterpriseAccessError.STALE, result.error)
    }

    @Test fun rejectedAndNoOpGesturesClearActualStaleOwnerBeforeReturning(): Unit = runTest {
        val gestures: List<Pair<String, suspend (SyncFlowFixture, EnterpriseSyncViewState) -> Unit>> = listOf(
            "open_wrong_epoch" to { f, state -> f.flow.open(f.second, state.epoch!! - 1) },
            "more_without_cursor" to { f, state -> f.flow.more(state.epoch!!, state.generation) },
            "manual_offline" to { f, state -> f.flow.manualCheck(false, state.epoch!!, state.generation) },
            "unknown_detail" to { f, state -> f.flow.showDetail(syncId(99), "sha256:" + "0".repeat(64), state.epoch!!, state.generation) },
            "malformed_detail" to { f, state ->
                try { f.flow.showDetail("not-an-id", "not-a-digest", state.epoch!!, state.generation) }
                catch (_: IllegalArgumentException) { }
            },
            "missing_dismissal" to { f, state -> f.flow.dismiss(syncId(99), "sha256:" + "0".repeat(64), "sha256:" + "1".repeat(64), state.epoch!!, state.generation) },
            "ineligible_dismissal" to { f, state ->
                val item = state.items.single()
                f.flow.dismiss(item.identity.operationId, item.identity.payloadDigest, "sha256:" + "1".repeat(64), state.epoch!!, state.generation)
            },
        )
        for (invalidation in listOf("epoch", "family")) for ((name, gesture) in gestures) {
            val f = SyncFlowFixture(); f.flow.open(f.first, f.fence.epoch)
            val before = f.flow.state.value
            if (invalidation == "epoch") f.fence.lock() else f.identity = f.identity.copy(family = syncId(90))
            gesture(f, before)
            val actual = f.flow.state.value
            assertNull(actual.context, "$invalidation/$name retained a stale selector")
            assertTrue(actual.items.isEmpty(), "$invalidation/$name retained private rows")
            assertNull(actual.detail); assertTrue(actual.active.isEmpty()); assertFalse(actual.busy)
            assertEquals(EnterpriseAccessError.STALE, actual.error)
        }
    }

    @Test fun coalescedActualDeliveryCompletionReloadsReceiptWithoutSeeingSending(): Unit = runTest {
        val f = SyncFlowFixture(); f.queueRetry = QueueRetryState()
        f.flow.open(f.first, f.fence.epoch)
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, f.flow.state.value.items.single().status)
        assertTrue(f.flow.state.value.active.isEmpty())
        assertEquals(1, f.service.dispatch(f.first.partition).completed)
        assertEquals(1, f.networkCalls)
        // StateFlow may deliver only the latest version after both begin and end.
        f.flow.onActivityChanged()
        assertEquals(EnterpriseSyncStatus.ACCEPTED, f.flow.state.value.items.single().status)
        assertTrue(f.flow.state.value.active.isEmpty())
    }

    @Test fun completionWhileBusyCannotBeConsumedByAnOlderCapturedPage(): Unit = runTest {
        val f = SyncFlowFixture(); f.queueRetry = QueueRetryState()
        f.flow.open(f.first, f.fence.epoch)
        val before = f.flow.state.value
        f.gate = CompletableDeferred(); f.blockedOrganization = f.first.partition.organizationId
        val refresh = launch(start = CoroutineStart.UNDISPATCHED) { f.flow.refresh(before.epoch!!, before.generation) }
        assertTrue(f.flow.state.value.busy)
        assertEquals(1, f.service.dispatch(f.first.partition).completed)
        f.flow.onActivityChanged()
        f.gate!!.complete(Unit); withTimeout(3000) { refresh.join() }
        assertFalse(f.flow.state.value.busy)
        assertEquals(EnterpriseSyncStatus.ACCEPTED, f.flow.state.value.items.single().status)
        assertTrue(f.pageCalls >= 3, "The snapshot captured before the ACK must be reread")
    }

    @Test fun sameFamilyProofRefreshDoesNotRetainOldAttemptDuringEnrollmentRead(): Unit = runTest {
        val f = SyncFlowFixture(); f.flow.open(f.first, f.fence.epoch)
        val before = f.flow.state.value
        f.refreshTokenOnDelivery = true; f.attemptGate = CompletableDeferred()
        val manual = launch(start = CoroutineStart.UNDISPATCHED) { f.flow.manualCheck(true, before.epoch!!, before.generation) }
        withTimeout(3000) { f.attemptStarted.await() }
        assertTrue(f.flow.state.value.items.isEmpty()); assertNull(f.flow.state.value.totals)
        assertNull(f.flow.state.value.detail); assertTrue(f.flow.state.value.active.isEmpty())
        f.flow.onActivityChanged()
        f.attemptGate!!.complete(Unit); withTimeout(3000) { manual.join() }
        val actual = f.flow.state.value
        assertEquals(2, f.tokenVersion); assertEquals(f.first, actual.context)
        assertNull(actual.error); assertEquals(1, actual.items.size); assertFalse(actual.busy)
        assertEquals(120000L, actual.manualWaitingMillis)
    }

    @Test fun activityObservationRechecksFenceBeforePrivatePublication(): Unit = runTest {
        val f = SyncFlowFixture(); f.flow.open(f.first, f.fence.epoch)
        var checks = 0
        // Invalidate after the predicate computed its answer at activity's last
        // source check; the publication still needs its own current-fence guard.
        f.retainedHook = { if (++checks == 4) f.fence.markBackground() }
        f.flow.onActivityChanged()
        assertTrue(checks >= 4)
        assertNull(f.flow.state.value.context); assertTrue(f.flow.state.value.items.isEmpty())
        assertEquals(EnterpriseAccessError.STALE, f.flow.state.value.error)
    }

    @Test fun sameFamilyProofRefreshDoesNotRetainOldCredentialsInsideDeliveryCapture(): Unit = runTest {
        val f = SyncFlowFixture(); f.flow.open(f.first, f.fence.epoch)
        val before = f.flow.state.value
        f.refreshTokenOnDelivery = true; f.deliveryGate = CompletableDeferred()
        val manual = launch(start = CoroutineStart.UNDISPATCHED) { f.flow.manualCheck(true, before.epoch!!, before.generation) }
        withTimeout(3000) { f.deliveryStarted.await() }
        assertTrue(f.flow.state.value.items.isEmpty()); assertNull(f.flow.state.value.totals)
        assertNull(f.flow.state.value.detail); assertTrue(f.flow.state.value.active.isEmpty())
        f.flow.onActivityChanged()
        f.deliveryGate!!.complete(Unit); withTimeout(3000) { manual.join() }
        val actual = f.flow.state.value
        assertEquals(2, f.tokenVersion); assertEquals(f.first, actual.context)
        assertNull(actual.error); assertEquals(1, actual.items.size); assertFalse(actual.busy)
        assertEquals(120000L, actual.manualWaitingMillis)
    }

    @Test fun manualDeliveryCannotAdoptAnIdentityWhenNoSourceWasCaptured(): Unit = runTest {
        val f = SyncFlowFixture(); f.accountError = EnterprisePersistenceUnavailable()
        f.flow.open(f.first, f.fence.epoch)
        val before = f.flow.state.value
        assertNotNull(before.error); assertTrue(before.items.isEmpty())
        f.accountError = null; f.identity = f.identity.copy(family = syncId(90))
        f.flow.manualCheck(true, before.epoch!!, before.generation)
        assertEquals(0, f.deliveryCaptures)
        assertTrue(f.flow.state.value.items.isEmpty()); assertNull(f.flow.state.value.context)
    }
}
