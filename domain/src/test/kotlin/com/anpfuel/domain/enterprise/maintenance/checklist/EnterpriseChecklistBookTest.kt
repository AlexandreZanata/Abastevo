package com.anpfuel.domain.enterprise.maintenance.checklist

import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertSame
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

internal object ChecklistBookFixture {
    fun id(n: Int) = "7c000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    const val MEMBER = "7c000000-0000-4000-8000-000000000003"
    const val ENROLLMENT = "7c000000-0000-4000-8000-000000000004"
    val binding: SyncBinding = SyncBinding.of(id(1), "https://checklists.example.invalid", id(2))
    val asset = id(20)
    val t0: Instant = Instant.parse("2026-11-15T11:40:00Z")
    /** Luzes (optional), Freios (critical), Pneus — the BUC-EMP-222 template. */
    val items = listOf(ChecklistItem("Luzes", optional = true), ChecklistItem("Freios", critical = true), ChecklistItem("Pneus"))
    fun template(version: Long = 1, items: List<ChecklistItem> = this.items) =
        ChecklistCachedTemplate(id(30), "Checklist de saída", version, Instant.parse("2026-11-14T10:00:00Z"), items)
    fun policy(mode: PolicyMode = PolicyMode.BLOCK, revision: Long = 1, validity: Long = 43_200) =
        ChecklistCachedPolicy(revision, mode, validity, Instant.parse("2026-11-14T10:00:00Z"))
    fun applicable(mode: PolicyMode = PolicyMode.BLOCK, at: Instant = t0.minusSeconds(600), asset: String = this.asset,
        context: ChecklistContext = ChecklistContext.DEPARTURE, items: List<ChecklistItem> = this.items, revision: Long = 1, validity: Long = 43_200) =
        ChecklistApplicable(asset, context, ChecklistMatch.ORGANIZATION, template(items = items), policy(mode, revision, validity), at)
    fun none(at: Instant = t0.minusSeconds(600), asset: String = this.asset) =
        ChecklistApplicable(asset, ChecklistContext.DEPARTURE, ChecklistMatch.NONE, null, null, at)
    fun book(mode: PolicyMode = PolicyMode.BLOCK) = ChecklistBook.empty(MEMBER, ENROLLMENT).withApplicable(applicable(mode))
    /** A started run of the asset with the given answers saved. */
    fun answered(book: ChecklistBook, run: Int, vararg answers: Pair<String, String?>, at: Instant = t0): ChecklistBook {
        var next = book.withStart(id(run), asset, ChecklistContext.DEPARTURE, at)
        answers.forEachIndexed { i, (answer, note) -> next = next.withAnswer(id(run), i + 1, answer, note, at.plusSeconds(60)) }
        return next
    }
    /** The operation the service builds for a run of [book] completed at [at]. */
    fun operation(book: ChecklistBook, run: Int, op: Int, at: Instant, acknowledged: Boolean): EnterpriseOperation {
        val local = book.run(id(run))!!
        val evaluation = book.evaluate(id(run), at)
        return ChecklistRunOperations.complete(ChecklistRunEnvelope(binding, MEMBER, ENROLLMENT, id(op), TimeDeclaration(at, "America/Cuiaba", -14_400)),
            id(run), local.assetId, local.context.name, local.template.templateId, local.template.version, local.policy.revision,
            com.anpfuel.domain.enterprise.EnterpriseTime.formatInstant(local.startedAt),
            local.answers.map { ChecklistRunAnswerInput(it.answer!!, it.note) }, evaluation.outcome.name,
            acknowledged && evaluation.acknowledgementRequired)
    }
    fun receipt(op: EnterpriseOperation, result: String, code: String? = null): OperationAcknowledgement = OperationAcknowledgement.parse(
        ("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"$result",""" +
            """"code":${code?.let { "\"$it\"" } ?: "null"},"entity_revision":${if (result == "ACCEPTED") "1" else "null"},""" +
            """"received_at":"2026-11-15T12:30:00Z","review_item_id":null,"warnings":[]}""").toByteArray(), op)
    fun server(run: LocalChecklistRun, recheck: ChecklistOutcome, outcome: ChecklistOutcome = recheck, serverPolicy: Long = run.policy.revision,
        runId: String = run.runId) = ChecklistServerRun(runId, run.assetId, run.template.templateId, run.context, run.template.version,
        run.policy.revision, run.startedAt, run.completedAt!!, run.acknowledged, ChecklistServerResult(1, serverPolicy,
            Instant.parse("2026-11-15T12:30:00.5Z"), run.deviceOutcome!!, recheck, outcome, recheck != run.deviceOutcome, recheck != run.deviceOutcome,
            false, listOf(2), listOf(2), null))
}

/** The `checklist-book` v1 index, its local transitions and the presentation (EMP-CHECK-002, B-BR-EMP-438/443, EDR-283/286). */
class EnterpriseChecklistBookTest {
    private val f = ChecklistBookFixture
    private fun failure(block: () -> Unit): ChecklistLocalFailure? = try { block(); null } catch (e: ChecklistLocalException) { e.failure }

    @Test fun `BUC-EMP-222 answers are kept one by one and a critical NOT_OK under BLOCK completes BLOCKING with its item`() {
        val started = f.book().withStart(f.id(100), f.asset, ChecklistContext.DEPARTURE, f.t0)
        val open = started.openRun(f.asset, ChecklistContext.DEPARTURE)!!
        assertEquals(f.id(100), open.runId)
        assertEquals(listOf("Luzes", "Freios", "Pneus"), open.template.items.map { it.label })
        assertEquals(0, open.answered)
        val one = started.withAnswer(f.id(100), 1, ChecklistAnswer.OK, null, f.t0.plusSeconds(30))
        // The codec keeps the answer exactly (process death reads the same bytes back).
        val restored = ChecklistBook.decode(one.canonicalBytes)
        assertArrayEquals(one.canonicalBytes, restored.canonicalBytes)
        assertEquals(ChecklistAnswer.OK, restored.run(f.id(100))!!.answers[0].answer)
        assertNull(restored.run(f.id(100))!!.answers[1].answer)
        val done = restored.withAnswer(f.id(100), 2, ChecklistAnswer.NOT_OK, "Freios rangendo", f.t0.plusSeconds(60))
            .withAnswer(f.id(100), 3, ChecklistAnswer.OK, null, f.t0.plusSeconds(90))
        val at = f.t0.plusSeconds(120)
        val evaluation = done.evaluate(f.id(100), at)
        assertEquals(ChecklistOutcome.BLOCKING, evaluation.outcome)
        assertEquals(listOf(2), evaluation.critical)
        val completed = done.withCompletion(f.id(100), f.id(500), at, acknowledged = false)
        val run = completed.run(f.id(100))!!
        assertTrue(run.completed)
        assertEquals(ChecklistOutcome.BLOCKING, run.deviceOutcome)
        assertEquals(at, run.completedAt)
        assertFalse(run.acknowledged)
        assertNull(completed.openRun(f.asset, ChecklistContext.DEPARTURE))
        assertEquals(failure { completed.withAnswer(f.id(100), 1, ChecklistAnswer.OK, null, at) }, ChecklistLocalFailure.RUN_NOT_OPEN)
        assertEquals(failure { completed.withCompletion(f.id(100), f.id(501), at, false) }, ChecklistLocalFailure.RUN_NOT_OPEN)
        assertArrayEquals(completed.canonicalBytes, ChecklistBook.decode(completed.canonicalBytes).canonicalBytes)
    }

    @Test fun `completion is refused with the shared rule, the validity and the acknowledgement`() {
        val book = f.book()
        assertEquals(ChecklistLocalFailure.INCOMPLETE, failure { f.answered(book, 101, ChecklistAnswer.OK to null).evaluate(f.id(101), f.t0.plusSeconds(120)) })
        assertEquals(ChecklistLocalFailure.ANSWER_NOT_ALLOWED, failure {
            f.answered(book, 102, ChecklistAnswer.OK to null, ChecklistAnswer.NOT_APPLICABLE to null, ChecklistAnswer.OK to null)
                .withCompletion(f.id(102), f.id(502), f.t0.plusSeconds(120), false)
        })
        val photo = ChecklistBook.empty(f.MEMBER, f.ENROLLMENT).withApplicable(f.applicable(items = listOf(ChecklistItem("Pneus", photoOnNotOk = true), ChecklistItem("Freios", critical = true))))
        val noPhoto = photo.withStart(f.id(103), f.asset, ChecklistContext.DEPARTURE, f.t0).withAnswer(f.id(103), 1, ChecklistAnswer.NOT_OK, "Careca", f.t0)
            .withAnswer(f.id(103), 2, ChecklistAnswer.OK, null, f.t0)
        assertEquals(ChecklistLocalFailure.PHOTO_REQUIRED, failure { noPhoto.withCompletion(f.id(103), f.id(503), f.t0.plusSeconds(60), false) })
        // The same item answered OK completes: the photo is required only when not OK (EDR-286).
        assertEquals(ChecklistOutcome.COMPLETED_OK, noPhoto.withAnswer(f.id(103), 1, ChecklistAnswer.OK, null, f.t0).evaluate(f.id(103), f.t0.plusSeconds(60)).outcome)
        val all = f.answered(book, 104, ChecklistAnswer.OK to null, ChecklistAnswer.OK to null, ChecklistAnswer.OK to null)
        assertEquals(ChecklistLocalFailure.CHECKLIST_EXPIRED, failure { all.withCompletion(f.id(104), f.id(504), f.t0.plusSeconds(43_201), false) })
        assertEquals(ChecklistOutcome.COMPLETED_OK, all.evaluate(f.id(104), f.t0.plusSeconds(43_200)).outcome)
        assertEquals(ChecklistLocalFailure.BEFORE_START, failure { all.withCompletion(f.id(104), f.id(504), f.t0.minusSeconds(1), false) })
        assertEquals(ChecklistLocalFailure.UNKNOWN_RUN, failure { all.evaluate(f.id(999), f.t0) })
        // WARN with a critical issue requires "Estou ciente das pendências".
        val warn = f.answered(f.book(PolicyMode.WARN), 105, ChecklistAnswer.OK to null, ChecklistAnswer.NOT_OK to null, ChecklistAnswer.OK to null)
        val warned = warn.evaluate(f.id(105), f.t0.plusSeconds(120))
        assertEquals(ChecklistOutcome.COMPLETED_WITH_ISSUES, warned.outcome)
        assertTrue(warned.acknowledgementRequired)
        assertEquals(ChecklistLocalFailure.ACKNOWLEDGEMENT_REQUIRED, failure { warn.withCompletion(f.id(105), f.id(505), f.t0.plusSeconds(120), false) })
        assertTrue(warn.withCompletion(f.id(105), f.id(505), f.t0.plusSeconds(120), true).run(f.id(105))!!.acknowledged)
        // A non-critical issue needs no acknowledgement and never records one.
        val minor = f.answered(book, 106, ChecklistAnswer.OK to null, ChecklistAnswer.OK to null, ChecklistAnswer.NOT_OK to "Calibrar")
            .withCompletion(f.id(106), f.id(506), f.t0.plusSeconds(120), true).run(f.id(106))!!
        assertEquals(ChecklistOutcome.COMPLETED_WITH_ISSUES, minor.deviceOutcome)
        assertFalse(minor.acknowledged)
    }

    @Test fun `starting needs a cached required template, one open run per asset and context, and an expired run starts again`() {
        val empty = ChecklistBook.empty(f.MEMBER, f.ENROLLMENT)
        assertEquals(ChecklistLocalFailure.NO_CHECKLIST, failure { empty.withStart(f.id(110), f.asset, ChecklistContext.DEPARTURE, f.t0) })
        assertEquals(ChecklistLocalFailure.NO_CHECKLIST, failure { empty.withApplicable(f.none()).withStart(f.id(110), f.asset, ChecklistContext.DEPARTURE, f.t0) })
        assertEquals(ChecklistLocalFailure.NO_CHECKLIST, failure { f.book().withStart(f.id(110), f.asset, ChecklistContext.RETURN, f.t0) })
        val started = f.book().withStart(f.id(110), f.asset, ChecklistContext.DEPARTURE, f.t0)
        assertEquals(ChecklistLocalFailure.RUN_IN_PROGRESS, failure { started.withStart(f.id(111), f.asset, ChecklistContext.DEPARTURE, f.t0.plusSeconds(60)) })
        assertEquals(ChecklistLocalFailure.DUPLICATE, failure { started.withStart(f.id(110), f.id(21), ChecklistContext.DEPARTURE, f.t0) })
        val expiredAt = f.t0.plusSeconds(43_201)
        assertTrue(started.run(f.id(110))!!.expiredAt(expiredAt))
        assertFalse(started.run(f.id(110))!!.expiredAt(f.t0.plusSeconds(43_200)))
        assertEquals(ChecklistLocalFailure.CHECKLIST_EXPIRED, failure { started.withAnswer(f.id(110), 1, ChecklistAnswer.OK, null, expiredAt) })
        val again = started.withStart(f.id(112), f.asset, ChecklistContext.DEPARTURE, expiredAt)
        assertNull(again.run(f.id(110)), "the expired run is replaced, never completed")
        assertEquals(f.id(112), again.openRun(f.asset, ChecklistContext.DEPARTURE)!!.runId)
        // The run pins the template it started with: a later refresh does not change it.
        val refreshed = started.withApplicable(f.applicable(mode = PolicyMode.WARN, at = f.t0.plusSeconds(10), revision = 2,
            items = listOf(ChecklistItem("Outro"))))
        assertEquals(3, refreshed.run(f.id(110))!!.template.items.size)
        assertEquals(PolicyMode.BLOCK, refreshed.run(f.id(110))!!.policy.mode)
        assertEquals(2, refreshed.applicable(f.asset, ChecklistContext.DEPARTURE)!!.policy!!.revision)
        // An older read never replaces a newer confirmation.
        assertSame(refreshed, refreshed.withApplicable(f.applicable(at = f.t0.minusSeconds(3600))))
    }

    @Test fun `answers are validated before they are kept`() {
        val started = f.book().withStart(f.id(120), f.asset, ChecklistContext.DEPARTURE, f.t0)
        for ((position, answer, note) in listOf(Triple(0, ChecklistAnswer.OK, null), Triple(4, ChecklistAnswer.OK, null), Triple(1, "MAYBE", null),
            Triple(1, "ok", null), Triple(1, ChecklistAnswer.NOT_OK, ""), Triple(1, ChecklistAnswer.NOT_OK, "   "), Triple(1, ChecklistAnswer.NOT_OK, "a\nb"),
            Triple(1, ChecklistAnswer.NOT_OK, "é".repeat(141)))) {
            assertEquals(ChecklistLocalFailure.INVALID_ANSWER, failure { started.withAnswer(f.id(120), position, answer, note, f.t0) }, "$position $answer")
        }
        assertEquals("é".repeat(140), started.withAnswer(f.id(120), 2, ChecklistAnswer.NOT_OK, "é".repeat(140), f.t0).run(f.id(120))!!.answers[1].note)
        assertEquals(ChecklistLocalFailure.UNKNOWN_RUN, failure { started.withAnswer(f.id(121), 1, ChecklistAnswer.OK, null, f.t0) })
        // An answer may change until completion; the latest one is kept.
        val changed = started.withAnswer(f.id(120), 2, ChecklistAnswer.NOT_OK, "x", f.t0).withAnswer(f.id(120), 2, ChecklistAnswer.OK, null, f.t0)
        assertEquals(ChecklistAnswer.OK, changed.run(f.id(120))!!.answers[1].answer)
        assertNull(changed.run(f.id(120))!!.answers[1].note)
    }

    @Test fun `the codec is strict, bound to its enrollment and refuses other formats`() {
        val book = f.answered(f.book(), 130, ChecklistAnswer.OK to null).withApplicable(f.none(asset = f.id(21)))
        val bytes = book.canonicalBytes
        val text = String(bytes, Charsets.UTF_8)
        assertTrue(text.contains("\"format\":\"checklist-book\"") && text.contains("\"version\":1"))
        assertArrayEquals(bytes, ChecklistBook.decode(bytes).canonicalBytes)
        assertSame(ChecklistBook.decode(bytes).let { it.requireBinding(f.MEMBER, f.ENROLLMENT) }.javaClass, ChecklistBook::class.java)
        assertEquals(ChecklistLocalFailure.FOREIGN, failure { ChecklistBook.decode(bytes).requireBinding(f.id(9), f.ENROLLMENT) })
        assertEquals(ChecklistLocalFailure.FOREIGN, failure { ChecklistBook.decode(bytes).requireBinding(f.MEMBER, f.id(9)) })
        for (bad in listOf(text.replace("\"version\":1", "\"version\":2"), text.replace("checklist-book", "trip-book"), " $text",
            text.replace("\"format\"", "\"extra\":1,\"format\""), "{}")) {
            assertThrows(IllegalArgumentException::class.java) { ChecklistBook.decode(bad.toByteArray()) }
        }
        assertTrue(ChecklistBook.decode(bytes).applicable(f.id(21), ChecklistContext.DEPARTURE)!!.required.not())
    }

    @Test fun `the server read is stored for the completed run only and its disagreement is visible`() {
        val warn = f.answered(f.book(PolicyMode.WARN), 140, ChecklistAnswer.OK to null, ChecklistAnswer.NOT_OK to null, ChecklistAnswer.OK to null)
        val at = f.t0.plusSeconds(120)
        val op = f.operation(warn, 140, 540, at, acknowledged = true)
        val completed = warn.withCompletion(f.id(140), op.operationId, at, true)
        val run = completed.run(f.id(140))!!
        assertEquals(ChecklistOutcome.COMPLETED_WITH_ISSUES, run.deviceOutcome)
        assertThrows(IllegalArgumentException::class.java) { completed.withServerResult(f.server(run, ChecklistOutcome.BLOCKING, runId = f.id(141))) }
        val stored = completed.withServerResult(f.server(run, ChecklistOutcome.BLOCKING, serverPolicy = 3))
        val result = stored.run(f.id(140))!!.server!!
        assertEquals(ChecklistOutcome.BLOCKING, result.outcome)
        assertTrue(result.outcomeDiffers)
        assertEquals(3, result.serverPolicyRevision)
        assertArrayEquals(stored.canonicalBytes, ChecklistBook.decode(stored.canonicalBytes).canonicalBytes)
        val entries = mapOf(op.operationId to EnterpriseQueueEntry(op, acknowledgement = f.receipt(op, "ACCEPTED")))
        val view = ChecklistPresentation.view(stored.run(f.id(140))!!, entries, at.plusSeconds(600))
        assertEquals(EnterpriseSyncStatus.ACCEPTED, view.sync)
        assertTrue(view.differs)
        assertEquals(ChecklistOutcome.BLOCKING, view.outcome, "the server outcome governs once known")
        assertEquals(ChecklistOutcome.COMPLETED_WITH_ISSUES, view.run.deviceOutcome)
        assertTrue(ChecklistPresentation.startBlocked(view))
        // A run still in progress has no server read to store.
        val open = f.answered(f.book(), 142, ChecklistAnswer.OK to null)
        assertThrows(IllegalArgumentException::class.java) { open.withServerResult(f.server(run, ChecklistOutcome.BLOCKING, runId = f.id(142))) }
    }

    @Test fun `presentation follows the run's own receipt and the latest valid departure run decides the trip start`() {
        val book = f.answered(f.book(), 150, ChecklistAnswer.OK to null, ChecklistAnswer.NOT_OK to "Freios rangendo", ChecklistAnswer.OK to null)
        val at = f.t0.plusSeconds(120)
        val op = f.operation(book, 150, 550, at, acknowledged = false)
        val completed = book.withCompletion(f.id(150), op.operationId, at, false)
        val saved = mapOf(op.operationId to EnterpriseQueueEntry(op))
        val view = ChecklistPresentation.view(completed.run(f.id(150))!!, saved, at)
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, view.sync)
        assertEquals(ChecklistOutcome.BLOCKING, view.outcome)
        assertEquals(listOf(2), view.blocking)
        assertFalse(view.differs)
        assertNull(view.code)
        assertTrue(ChecklistPresentation.startBlocked(ChecklistPresentation.latestDeparture(completed, saved, f.asset, at.plusSeconds(60))))
        assertNull(ChecklistPresentation.latestDeparture(completed, saved, f.id(21), at), "another vehicle has no departure run")
        // A run past its validity no longer decides the start.
        assertNull(ChecklistPresentation.latestDeparture(completed, saved, f.asset, at.plusSeconds(43_201)))
        // A finally refused run never counts.
        val refused = mapOf(op.operationId to EnterpriseQueueEntry(op, acknowledgement = f.receipt(op, "REJECTED", "MALFORMED")))
        assertEquals(EnterpriseSyncStatus.REJECTED, ChecklistPresentation.view(completed.run(f.id(150))!!, refused, at).sync)
        assertNull(ChecklistPresentation.latestDeparture(completed, refused, f.asset, at))
        // A missing or foreign entry is refused, never guessed.
        assertThrows(IllegalArgumentException::class.java) { ChecklistPresentation.view(completed.run(f.id(150))!!, emptyMap(), at) }
        val other = f.operation(f.answered(f.book(), 151, ChecklistAnswer.OK to null, ChecklistAnswer.OK to null, ChecklistAnswer.OK to null), 151, 550, at, false)
        assertThrows(IllegalArgumentException::class.java) { ChecklistPresentation.view(completed.run(f.id(150))!!, mapOf(op.operationId to EnterpriseQueueEntry(other)), at) }
        // A later OK run of the same vehicle takes over; an open run never decides.
        val next = f.answered(completed, 152, ChecklistAnswer.OK to null, ChecklistAnswer.OK to null, ChecklistAnswer.OK to null, at = at.plusSeconds(60))
        val nextOp = f.operation(next, 152, 552, at.plusSeconds(300), false)
        val both = next.withCompletion(f.id(152), nextOp.operationId, at.plusSeconds(300), false)
        val entries = saved + (nextOp.operationId to EnterpriseQueueEntry(nextOp))
        val latest = ChecklistPresentation.latestDeparture(both, entries, f.asset, at.plusSeconds(400))!!
        assertEquals(f.id(152), latest.run.runId)
        assertFalse(ChecklistPresentation.startBlocked(latest))
        assertFalse(ChecklistPresentation.startBlocked(null))
        // EMP-TRIP-019: the start names the latest valid departure run, never a blocking one and nothing without a run.
        assertEquals(f.id(152), ChecklistPresentation.departureRun(latest))
        assertNull(ChecklistPresentation.departureRun(ChecklistPresentation.latestDeparture(completed, saved, f.asset, at.plusSeconds(60))))
        assertNull(ChecklistPresentation.departureRun(null))
        val views = ChecklistPresentation.views(both, entries, at.plusSeconds(400))
        assertEquals(listOf(f.id(152), f.id(150)), views.map { it.run.runId })
        val inProgress = f.answered(f.book(), 153, ChecklistAnswer.OK to null)
        val openView = ChecklistPresentation.view(inProgress.run(f.id(153))!!, emptyMap(), f.t0)
        assertNull(openView.sync); assertNull(openView.evaluation); assertNull(openView.outcome); assertFalse(openView.expired)
        assertTrue(ChecklistPresentation.view(inProgress.run(f.id(153))!!, emptyMap(), f.t0.plusSeconds(43_201)).expired)
        assertNotNull(ChecklistPresentation.views(inProgress, emptyMap(), f.t0).single())
    }

    @Test fun `the book is bounded and only delivered and read runs are pruned`() {
        var book = f.book()
        val limit = ChecklistBook.MAX_RUNS
        var at = f.t0
        for (n in 0 until limit) {
            book = f.answered(book, 1000 + n, ChecklistAnswer.OK to null, ChecklistAnswer.OK to null, ChecklistAnswer.OK to null, at = at)
            book = book.withCompletion(f.id(1000 + n), f.id(5000 + n), at.plusSeconds(90), false)
            at = at.plusSeconds(120)
        }
        assertEquals(ChecklistLocalFailure.FULL, failure { book.withStart(f.id(9000), f.asset, ChecklistContext.DEPARTURE, at) })
        // Once the server read of the oldest run is stored it may leave the device's index.
        val oldest = book.run(f.id(1000))!!
        book = book.withServerResult(f.server(oldest, ChecklistOutcome.COMPLETED_OK))
        val next = book.withStart(f.id(9000), f.asset, ChecklistContext.DEPARTURE, at)
        assertNull(next.run(f.id(1000)))
        assertEquals(limit, next.runs.size)
        assertNotNull(next.run(f.id(9000)))
    }
}
