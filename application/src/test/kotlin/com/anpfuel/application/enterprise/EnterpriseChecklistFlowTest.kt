package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.checklist.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.maintenance.checklist.*
import com.anpfuel.domain.enterprise.sync.*
import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

private fun cid(n: Int) = "7e000000-0000-4000-8000-" + n.toString().padStart(12, '0')
private const val CHECK_ORIGIN = "https://checklists.example.invalid"

/** The CHECK-002 intake and run read in miniature: every well-formed run is ACCEPTED and rechecked with the server's own policy. */
internal class ChecklistServerFake {
    val delivered = mutableListOf<String>()
    val runs = linkedMapOf<String, ChecklistRunFact>()
    var mode = PolicyMode.BLOCK
    var policyRevision = 1L
    var unreachable = false
    /** BUC-EMP-224: another member released the BLOCKING run (revision 2). */
    var released = false
    var applicableCalls = 0
    var runReads = 0
    var applicable: ChecklistApplicable? = null
    val items = listOf(ChecklistItem("Luzes", optional = true), ChecklistItem("Freios", critical = true), ChecklistItem("Pneus"))
    fun answer(op: EnterpriseOperation): OperationAcknowledgement {
        delivered += op.operationId
        val fact = ChecklistRunOperations.read(op)
        runs[fact.runId] = fact
        return OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}",""" +
            """"result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-15T18:00:00Z","review_item_id":null,"warnings":[]}""").toByteArray(), op)
    }
    val gateway = object : EnterpriseChecklistGateway {
        override suspend fun applicable(context: EnterpriseContextSummary, assetId: String, checklistContext: ChecklistContext): EnterpriseAccessResult<ChecklistApplicable> {
            applicableCalls++
            if (unreachable) return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
            return EnterpriseAccessResult.Ok(requireNotNull(applicable).also { require(it.assetId == assetId && it.context == checklistContext) })
        }
        override suspend fun run(context: EnterpriseContextSummary, assetId: String, runId: String): EnterpriseAccessResult<ChecklistServerRun> {
            runReads++
            if (unreachable) return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
            val fact = runs[runId] ?: return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
            require(fact.assetId == assetId)
            val evaluation = Checklist.evaluate(items, mode.name, 43_200, fact.answers.map { ChecklistResponse(it.answer, it.note ?: "") })
            return EnterpriseAccessResult.Ok(ChecklistServerRun(fact.runId, fact.assetId, fact.templateId, fact.context, fact.templateVersion,
                fact.policyRevision, fact.startedAt, fact.declared.occurredAt, fact.acknowledged, ChecklistServerResult(if (released) 2 else 1, policyRevision,
                    Instant.parse("2026-11-15T18:00:00Z"), fact.deviceOutcome, evaluation.outcome,
                    if (released) ChecklistOutcome.COMPLETED_WITH_ISSUES else evaluation.outcome, evaluation.outcome != fact.deviceOutcome,
                    evaluation.outcome != fact.deviceOutcome, false, evaluation.issues, evaluation.critical,
                    if (released) Instant.parse("2026-11-15T19:00:00Z") else null)))
        }
    }
}

internal class ChecklistFlowFixture(val disk: TripDisk = TripDisk(), val server: ChecklistServerFake = ChecklistServerFake()) {
    val fence = EnterpriseSessionFence()
    val a = EnterpriseContextSummary(EnterprisePartition.of(cid(1), CHECK_ORIGIN, cid(2)), cid(3), "Synthetic A", cid(5))
    val b = EnterpriseContextSummary(EnterprisePartition.of(cid(1), CHECK_ORIGIN, cid(20)), cid(30), "Synthetic B", cid(50))
    val asset = cid(70)
    var identity = EnterpriseLocalIdentity(cid(1), CHECK_ORIGIN, cid(9))
    var wall = Instant.parse("2026-11-15T11:40:00Z").toEpochMilli()
    var failSave = false
    var failUpdate = false
    private var nonce = 1000
    private fun membership(p: EnterprisePartition) = if (p == a.partition) a.membershipId else b.membershipId
    val source = object : EnterpriseQueueSource {
        override suspend fun account(): EnterpriseQueueAccountLease {
            val captured = identity; val epoch = fence.epoch; val background = fence.backgroundEpoch
            return EnterpriseQueueAccountLease.capture(captured, epoch, background, cid(nonce++)) {
                identity == captured && fence.epoch == epoch && fence.backgroundEpoch == background
            }
        }
        override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
            account.assertCurrent()
            return account.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, membership(partition)), cid(4), 1))
        }
    }
    private fun tx(p: EnterprisePartition, values: MutableMap<String, EnterpriseLocalRecord>) = object : EnterpriseQueueTransaction {
        override fun record(id: String) = values[id]
        override fun put(record: EnterpriseLocalRecord) { require(record.partition == p); values[record.id] = record }
    }
    val queue = object : EnterpriseOperationQueue {
        override suspend fun save(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, project: (EnterpriseQueueTransaction) -> Unit): Boolean {
            attempt.assertCurrent()
            val p = disk.of(attempt.partition)
            p.entries[operation.operationId]?.let { require(it.operation.sameIntent(operation)); return false }
            val changed = p.records.toMutableMap(); project(tx(attempt.partition, changed))
            if (failSave) throw EnterprisePersistenceUnavailable()
            p.entries[operation.operationId] = EnterpriseQueueEntry(operation); p.records.clear(); p.records.putAll(changed); return true
        }
        override suspend fun pending(attempt: EnterpriseQueueAttempt) = disk.of(attempt.partition).entries.values.filter { it.acknowledgement == null }
        override suspend fun entry(attempt: EnterpriseQueueAttempt, operationId: String) = disk.of(attempt.partition).entries[operationId]
        override suspend fun acknowledge(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, ack: OperationAcknowledgement,
            project: (EnterpriseQueueTransaction) -> Unit): Boolean {
            val p = disk.of(attempt.partition)
            val existing = p.entries.getValue(operation.operationId)
            existing.acknowledgement?.let { return false }
            val changed = p.records.toMutableMap(); project(tx(attempt.partition, changed))
            p.entries[operation.operationId] = EnterpriseQueueEntry(operation, existing.retry, ack); p.records.clear(); p.records.putAll(changed); return true
        }
        override suspend fun retry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, expected: QueueRetryState, next: QueueRetryState): Boolean {
            val p = disk.of(attempt.partition); val old = p.entries.getValue(operation.operationId)
            if (old.acknowledgement != null || !old.retry.sameAs(expected)) return false
            p.entries[operation.operationId] = EnterpriseQueueEntry(old.operation, next); return true
        }
        override suspend fun stats(attempt: EnterpriseQueueAttempt) = EnterpriseQueueStats(0, 0, 0, 0, 2048)
    }
    private fun book(attempt: EnterpriseQueueAttempt): ChecklistBook? = disk.of(attempt.partition).records[EnterpriseChecklistProducers.BOOK_RECORD_ID]
        ?.let { ChecklistBook.decode(it.envelope).requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId) }
    val store = object : EnterpriseChecklistStore {
        override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseChecklistSnapshot {
            attempt.assertCurrent()
            val book = book(attempt)
            val p = disk.of(attempt.partition)
            return EnterpriseChecklistSnapshot(book, book?.runs?.mapNotNull { it.operationId }?.associateWith { p.entries.getValue(it) } ?: emptyMap())
        }
        override suspend fun update(attempt: EnterpriseQueueAttempt, change: (ChecklistBook) -> ChecklistBook): ChecklistBook {
            attempt.assertCurrent()
            val current = book(attempt) ?: ChecklistBook.empty(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
            val next = change(current)
            if (failUpdate) throw EnterprisePersistenceUnavailable()
            disk.of(attempt.partition).records[EnterpriseChecklistProducers.BOOK_RECORD_ID] =
                EnterpriseLocalRecord(attempt.partition, EnterpriseChecklistProducers.BOOK_RECORD_ID, next.canonicalBytes)
            return next
        }
    }
    val clock = { EnterpriseDeviceClock("boot-7", 5_000, wall) }
    val queueService = EnterpriseQueueService(source, queue, { _, operations ->
        if (server.unreachable) throw EnterpriseQueueDeliveryFailure()
        EnterpriseOperationDelivery(operations.map(server::answer))
    }, EnterpriseQueueRegistry(*EnterpriseChecklistProducers.all()), clock, { 0 })
    val service = EnterpriseChecklistService(source, queueService::save, store, server.gateway, { ZoneId.of("America/Cuiaba") }, { wall }, { cid(nonce++) })
    fun flow() = ChecklistFlow(source, service, fence) { account, partition -> queueService.dispatch(account, partition) }
    fun applicable(mode: PolicyMode = PolicyMode.BLOCK, revision: Long = 1, items: List<ChecklistItem> = server.items, asset: String = this.asset) =
        ChecklistApplicable(asset, ChecklistContext.DEPARTURE, ChecklistMatch.ORGANIZATION,
            ChecklistCachedTemplate(cid(80), "Checklist de saída", 1, Instant.parse("2026-11-14T10:00:00Z"), items),
            ChecklistCachedPolicy(revision, mode, 43_200, Instant.parse("2026-11-14T10:00:00Z")), Instant.ofEpochMilli(wall).minusSeconds(60))
    fun later(seconds: Long) { wall += seconds * 1000 }
    fun entries(p: EnterprisePartition = a.partition) = disk.of(p).entries
    fun storedBook(p: EnterprisePartition = a.partition) = disk.of(p).records[EnterpriseChecklistProducers.BOOK_RECORD_ID]?.let { ChecklistBook.decode(it.envelope) }
}

/** EMP-CHECK-002 application: one visible owner over the durable book, the queue producer and the server's read (B-BR-EMP-438/443). */
class EnterpriseChecklistFlowTest {
    private val g: ChecklistFlow.() -> Long = { state.value.generation }

    @Test fun `BUC-EMP-222 the template is cached online, answers survive a restart offline and a BLOCKING completion is queued`() = runTest {
        val f = ChecklistFlowFixture(); f.server.applicable = f.applicable()
        val flow = f.flow()
        flow.open(f.a, f.asset, ChecklistContext.DEPARTURE, 0)
        assertTrue(flow.state.value.loaded); assertNull(flow.state.value.applicable, "nothing is cached before the first online read")
        flow.refresh(0, flow.g())
        val cached = flow.state.value.applicable!!
        assertEquals(listOf("Luzes", "Freios", "Pneus"), cached.template!!.items.map { it.label })
        assertEquals(1, f.server.applicableCalls)
        // Offline from here on.
        f.server.unreachable = true
        flow.start(0, flow.g())
        val run = flow.state.value.open!!.run.runId
        flow.answer(run, 1, ChecklistAnswer.OK, null, 0, flow.g())
        assertNotNull(flow.state.value.lastSaved, "the answer is reported saved only after the durable write")
        assertEquals(ChecklistAnswer.OK, f.storedBook()!!.run(run)!!.answers[0].answer)
        // Process death: a new owner reads the same durable book.
        flow.close()
        val restarted = f.flow()
        restarted.open(f.a, f.asset, ChecklistContext.DEPARTURE, 0)
        val reopened = restarted.state.value.open!!
        assertEquals(run, reopened.run.runId)
        assertEquals(ChecklistAnswer.OK, reopened.run.answers[0].answer)
        restarted.answer(run, 2, ChecklistAnswer.NOT_OK, "Freios rangendo", 0, restarted.g())
        restarted.answer(run, 3, ChecklistAnswer.OK, null, 0, restarted.g())
        f.later(60)
        restarted.complete(run, false, 0, restarted.g())
        val state = restarted.state.value
        assertNull(state.open)
        assertEquals(run, state.completed)
        val latest = state.latest!!
        assertEquals(ChecklistOutcome.BLOCKING, latest.outcome)
        assertEquals(listOf(2), latest.blocking)
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, latest.sync)
        assertEquals(run, state.departure!!.run.runId)
        assertTrue(ChecklistPresentation.startBlocked(state.departure))
        val queued = f.entries().values.single()
        val fact = ChecklistRunOperations.read(queued.operation)
        assertEquals(listOf(run, f.asset, "DEPARTURE", "BLOCKING"), listOf(fact.runId, fact.assetId, fact.context.name, fact.deviceOutcome.name))
        assertEquals(listOf(ChecklistRunAnswer("OK", null), ChecklistRunAnswer("NOT_OK", "Freios rangendo"), ChecklistRunAnswer("OK", null)), fact.answers)
        assertEquals(queued.operation.operationId, f.storedBook()!!.run(run)!!.operationId, "the run's record and its operation committed together")
        assertTrue(f.server.delivered.isEmpty())
        assertTrue(f.entries(f.b.partition).isEmpty() && f.storedBook(f.b.partition) == null, "another organization holds nothing")
    }

    @Test fun `BUC-EMP-223 after an ACCEPTED receipt the run is read and a disagreement is kept with both outcomes`() = runTest {
        val f = ChecklistFlowFixture(); f.server.applicable = f.applicable(PolicyMode.WARN, revision = 2)
        val flow = f.flow()
        flow.open(f.a, f.asset, ChecklistContext.DEPARTURE, 0); flow.refresh(0, flow.g())
        flow.start(0, flow.g())
        val run = flow.state.value.open!!.run.runId
        for ((position, answer) in listOf(1 to ChecklistAnswer.OK, 2 to ChecklistAnswer.NOT_OK, 3 to ChecklistAnswer.OK)) flow.answer(run, position, answer, null, 0, flow.g())
        flow.complete(run, false, 0, flow.g())
        assertEquals(ChecklistLocalFailure.ACKNOWLEDGEMENT_REQUIRED, flow.state.value.refusal, "WARN with a critical issue asks for the acknowledgement")
        assertTrue(f.entries().isEmpty())
        flow.complete(run, true, 0, flow.g())
        assertEquals(ChecklistOutcome.COMPLETED_WITH_ISSUES, flow.state.value.latest!!.outcome)
        assertTrue(flow.state.value.latest!!.run.acknowledged)
        // The server's policy at that time was BLOCK.
        f.server.mode = PolicyMode.BLOCK; f.server.policyRevision = 3
        flow.sendNow(0, flow.g())
        val latest = flow.state.value.latest!!
        assertEquals(EnterpriseSyncStatus.ACCEPTED, latest.sync)
        assertEquals(1, f.server.runReads)
        assertTrue(latest.differs)
        assertEquals(ChecklistOutcome.BLOCKING, latest.outcome)
        assertEquals(ChecklistOutcome.COMPLETED_WITH_ISSUES, latest.run.deviceOutcome)
        assertEquals(3, latest.run.server!!.serverPolicyRevision)
        assertTrue(ChecklistPresentation.startBlocked(flow.state.value.departure))
        // A BLOCKING run is read again (another member may release it, BUC-EMP-224); nothing is delivered twice.
        flow.refresh(0, flow.g())
        assertEquals(2, f.server.runReads)
        assertTrue(ChecklistPresentation.startBlocked(flow.state.value.departure))
        f.server.released = true
        flow.refresh(0, flow.g())
        assertEquals(3, f.server.runReads)
        assertEquals(ChecklistOutcome.COMPLETED_WITH_ISSUES, flow.state.value.latest!!.outcome)
        assertEquals(2, flow.state.value.latest!!.run.server!!.revision)
        assertFalse(ChecklistPresentation.startBlocked(flow.state.value.departure), "a released run no longer blocks the start")
        // A run whose outcome is final on the server is not read again.
        flow.refresh(0, flow.g())
        assertEquals(3, f.server.runReads)
        assertEquals(1, f.server.delivered.size)
    }

    @Test fun `a failed write saves nothing and says so`() = runTest {
        val f = ChecklistFlowFixture(); f.server.applicable = f.applicable()
        val flow = f.flow()
        flow.open(f.a, f.asset, ChecklistContext.DEPARTURE, 0); flow.refresh(0, flow.g())
        flow.start(0, flow.g())
        val run = flow.state.value.open!!.run.runId
        f.failUpdate = true
        flow.answer(run, 1, ChecklistAnswer.OK, null, 0, flow.g())
        assertTrue(flow.state.value.saveFailed)
        assertNull(flow.state.value.lastSaved)
        assertNull(f.storedBook()!!.run(run)!!.answers[0].answer)
        assertNull(flow.state.value.open!!.run.answers[0].answer, "the screen never shows an answer that was not written")
        f.failUpdate = false
        for (position in 1..3) flow.answer(run, position, ChecklistAnswer.OK, null, 0, flow.g())
        assertFalse(flow.state.value.saveFailed)
        f.failSave = true
        flow.complete(run, false, 0, flow.g())
        assertTrue(flow.state.value.saveFailed)
        assertTrue(f.entries().isEmpty())
        assertFalse(f.storedBook()!!.run(run)!!.completed)
        assertEquals(run, flow.state.value.open!!.run.runId)
        f.failSave = false
        flow.complete(run, false, 0, flow.g())
        assertEquals(ChecklistOutcome.COMPLETED_OK, flow.state.value.latest!!.outcome)
        assertFalse(ChecklistPresentation.startBlocked(flow.state.value.departure))
    }

    @Test fun `local refusals keep the run and queue nothing`() = runTest {
        val photo = listOf(ChecklistItem("Pneus", photoOnNotOk = true), ChecklistItem("Freios", critical = true), ChecklistItem("Luzes", optional = true))
        val f = ChecklistFlowFixture(); f.server.applicable = f.applicable(items = photo)
        val flow = f.flow()
        flow.open(f.a, f.asset, ChecklistContext.DEPARTURE, 0)
        flow.start(0, flow.g())
        assertEquals(ChecklistLocalFailure.NO_CHECKLIST, flow.state.value.refusal, "no cached template, no run")
        flow.refresh(0, flow.g())
        flow.start(0, flow.g())
        val run = flow.state.value.open!!.run.runId
        flow.start(0, flow.g())
        assertEquals(ChecklistLocalFailure.RUN_IN_PROGRESS, flow.state.value.refusal)
        // The shared rule's own precedence: a missing photo is named before an unanswered item, so INCOMPLETE is shown first
        // with no NOT_OK photo item (fixture corrected before GREEN; the refusal codes are unchanged).
        flow.answer(run, 2, ChecklistAnswer.OK, null, 0, flow.g())
        flow.complete(run, false, 0, flow.g())
        assertEquals(ChecklistLocalFailure.INCOMPLETE, flow.state.value.refusal)
        flow.answer(run, 1, ChecklistAnswer.NOT_OK, "Careca", 0, flow.g()); flow.answer(run, 3, ChecklistAnswer.NOT_APPLICABLE, null, 0, flow.g())
        flow.complete(run, false, 0, flow.g())
        assertEquals(ChecklistLocalFailure.PHOTO_REQUIRED, flow.state.value.refusal, "a photo is never faked (EDR-286)")
        flow.answer(run, 1, "SOMETIMES", null, 0, flow.g())
        assertEquals(ChecklistLocalFailure.INVALID_ANSWER, flow.state.value.refusal)
        f.later(43_201)
        flow.answer(run, 1, ChecklistAnswer.OK, null, 0, flow.g())
        assertEquals(ChecklistLocalFailure.CHECKLIST_EXPIRED, flow.state.value.refusal)
        flow.complete(run, false, 0, flow.g())
        assertEquals(ChecklistLocalFailure.CHECKLIST_EXPIRED, flow.state.value.refusal)
        assertTrue(flow.state.value.open!!.expired)
        assertTrue(f.entries().isEmpty())
        // An expired run is replaced by a new one.
        flow.start(0, flow.g())
        assertNotEquals(run, flow.state.value.open!!.run.runId)
        assertNull(flow.state.value.refusal)
    }

    @Test fun `no applicable template is a stated empty state and a failed read keeps the cache`() = runTest {
        val f = ChecklistFlowFixture()
        // The server's evaluated_at only grows: the first read is older than the later one (fixture corrected before GREEN).
        f.server.applicable = ChecklistApplicable(f.asset, ChecklistContext.DEPARTURE, ChecklistMatch.NONE, null, null, Instant.ofEpochMilli(f.wall).minusSeconds(120))
        val flow = f.flow()
        flow.open(f.a, f.asset, ChecklistContext.DEPARTURE, 0); flow.refresh(0, flow.g())
        assertFalse(flow.state.value.applicable!!.required)
        flow.start(0, flow.g())
        assertEquals(ChecklistLocalFailure.NO_CHECKLIST, flow.state.value.refusal)
        f.server.applicable = f.applicable()
        flow.refresh(0, flow.g())
        assertTrue(flow.state.value.applicable!!.required)
        f.server.unreachable = true
        flow.refresh(0, flow.g())
        assertTrue(flow.state.value.refreshFailed)
        assertTrue(flow.state.value.applicable!!.required, "the cached template stays usable offline")
        assertNull(flow.state.value.error)
    }

    @Test fun `the producer is registered for check run complete and refuses an operation that is not the saved run`() = runTest {
        val producers = EnterpriseChecklistProducers.all()
        assertEquals(listOf(ChecklistRunOperations.COMPLETE), producers.map { it.descriptor().type })
        val f = ChecklistFlowFixture(); f.server.applicable = f.applicable()
        val flow = f.flow()
        flow.open(f.a, f.asset, ChecklistContext.DEPARTURE, 0); flow.refresh(0, flow.g()); flow.start(0, flow.g())
        val run = flow.state.value.open!!.run.runId
        for (position in 1..3) flow.answer(run, position, ChecklistAnswer.OK, null, 0, flow.g())
        val local = f.storedBook()!!.run(run)!!
        val envelope = ChecklistRunEnvelope(SyncBinding.of(f.a.partition.ownerAccount, f.a.partition.origin, f.a.partition.organizationId),
            f.a.membershipId, cid(4), cid(900), TimeDeclaration(Instant.ofEpochMilli(f.wall), "UTC", 0))
        fun op(answers: List<String>, outcome: String) = ChecklistRunOperations.complete(envelope, run, f.asset, "DEPARTURE", local.template.templateId,
            1, 1, EnterpriseTime.formatInstant(local.startedAt), answers.map { ChecklistRunAnswerInput(it, null) }, outcome, false)
        // Answers that differ from the saved ones, or an outcome the device did not compute, are refused inside the write.
        for (wrong in listOf(op(listOf("OK", "NOT_OK", "OK"), "BLOCKING"), op(listOf("OK", "OK", "OK"), "BLOCKING"))) {
            assertThrows(EnterprisePersistenceUnavailable::class.java) { kotlinx.coroutines.runBlocking { f.queueService.save(f.a.partition, wrong) } }
        }
        assertTrue(f.entries().isEmpty())
        assertFalse(f.storedBook()!!.run(run)!!.completed)
        assertTrue(f.queueService.save(f.a.partition, op(listOf("OK", "OK", "OK"), "COMPLETED_OK")))
        assertEquals(cid(900), f.storedBook()!!.run(run)!!.operationId)
    }
}
