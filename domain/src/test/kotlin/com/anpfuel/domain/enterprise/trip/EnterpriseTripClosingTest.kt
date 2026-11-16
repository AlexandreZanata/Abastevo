package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.attachment.*
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.expense.ExpenseEnvelope
import com.anpfuel.domain.enterprise.expense.ExpenseOperations
import com.anpfuel.domain.enterprise.expense.ExpensePresentation
import com.anpfuel.domain.enterprise.expense.ExpenseView
import com.anpfuel.domain.enterprise.fueling.*
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** EMP-TRIP-020 (B-BR-EMP-522..526, EDR-339..341): the device-local closing conference of one trip. */
class EnterpriseTripClosingTest {
    private val f = TripLocalFixture
    private fun fueling(op: Int, fuelingId: Int, at: String, asset: Int = 20, paid: String = "97000"): EnterpriseOperation = FuelingOperations.submit(
        FuelingEnvelope(f.binding, f.member, f.enrollment, f.id(op), TimeDeclaration(f.at(at), "UTC", 0)), f.id(fuelingId),
        FuelingSubmitInput(f.id(asset), f.id(30), "DIESEL_S10", "L", "150000", paid, null, null, null, "FULL", "COMPANY", "FUEL_CARD", null, "Painel apagado"))
    private fun expense(op: Int, expenseId: Int, at: String, asset: Int = 20, paid: String = "1250"): EnterpriseOperation = ExpenseOperations.submit(
        ExpenseEnvelope(f.binding, f.member, f.enrollment, f.id(op), TimeDeclaration(f.at(at), "UTC", 0)), f.id(expenseId), f.id(asset), "TOLL",
        paid, paid, "EMPLOYEE", null)
    private fun fv(op: EnterpriseOperation, accepted: Boolean = false): FuelingView =
        FuelingPresentation.view(EnterpriseQueueEntry(op, acknowledgement = if (accepted) f.ack(op) else null))
    private fun ev(op: EnterpriseOperation, result: String? = null): ExpenseView =
        ExpensePresentation.view(EnterpriseQueueEntry(op, acknowledgement = result?.let { f.ack(op, it, if (it == "REJECTED") "FORBIDDEN" else null, if (it == "ACCEPTED") 1 else null) }))
    private fun photo(owner: EnterpriseOperation, kind: DocumentTargetKind, target: String, n: Int, state: AttachmentState, accepted: Boolean = true) =
        AttachmentPresentation.view(EnterpriseAttachment(f.id(700 + n), f.id(800 + n), f.id(900 + n), owner.operationId, AttachmentTarget(kind, target),
            "ab".repeat(32), 2048, Instant.parse("2026-11-15T12:01:00Z"), state, null, if (state == AttachmentState.WAITING_FOR_RECORD) null else 1),
            if (accepted) f.ack(owner) else null)

    private val start = f.start()
    private val finish = f.finish()
    private fun finished(submitted: Boolean = false): TripView {
        val local = f.local().let { LocalTrip(it.tripId, it.assetId, it.label, false, it.startOperationId, it.startedAt, emptyList(),
            finish.operationId, f.at("2026-11-15T15:00:00Z"), if (submitted) f.id(400) else null) }
        val entries = f.saved(start, finish) + if (submitted) f.saved(f.submit()) else emptyMap()
        return TripPresentation.view(local, entries)
    }

    @Test fun `the conference lists the trip's own readings and this vehicle's records during the trip with their own states`() {
        val inTrip = fueling(500, 510, "2026-11-15T13:00:00Z")
        val accepted = fueling(501, 511, "2026-11-15T14:00:00Z", paid = "50000")
        val before = fueling(502, 512, "2026-11-15T11:59:59Z")
        val after = fueling(503, 513, "2026-11-15T15:00:01Z")
        val otherAsset = fueling(504, 514, "2026-11-15T13:30:00Z", asset = 21)
        val toll = expense(600, 610, "2026-11-15T13:10:00Z")
        val refused = expense(601, 611, "2026-11-15T14:10:00Z", paid = "800")
        val readingId = ((finished().start.fact as TripOperationFact.Start).evidence as TripEvidence.Reading).readingId
        val photos = mapOf(
            start.operationId to listOf(photo(start, DocumentTargetKind.METER_READING, readingId, 1, AttachmentState.STORED)),
            inTrip.operationId to listOf(photo(inTrip, DocumentTargetKind.FUELING, f.id(510), 2, AttachmentState.DECLARED)),
            accepted.operationId to listOf(photo(accepted, DocumentTargetKind.FUELING, f.id(511), 3, AttachmentState.STORED)),
            toll.operationId to listOf(photo(toll, DocumentTargetKind.EXPENSE, f.id(610), 4, AttachmentState.FILE_MISSING)))
        val closing = TripClosings.of(finished(), photos,
            listOf(fv(inTrip), fv(accepted, accepted = true), fv(before), fv(after), fv(otherAsset), fv(inTrip)),
            listOf(ev(toll), ev(refused, "REJECTED")))
        assertEquals(listOf(ClosingItemKind.DEPARTURE_READING, ClosingItemKind.FUELING, ClosingItemKind.EXPENSE, ClosingItemKind.FUELING,
            ClosingItemKind.EXPENSE, ClosingItemKind.FINAL_READING), closing.items.map { it.kind })
        val byRecord = closing.items.associateBy { it.recordId }
        // Foreign vehicle, before the start and after the finish are not this trip's; a duplicate counts once.
        assertEquals(setOf(f.id(510), f.id(511), f.id(610), f.id(611)), closing.items.filter { it.kind == ClosingItemKind.FUELING || it.kind == ClosingItemKind.EXPENSE }
            .map { it.recordId }.toSet())
        assertEquals(ClosingEvidenceState.AVAILABLE, byRecord.getValue(readingId).evidence)
        assertEquals(ClosingEvidenceState.PENDING, byRecord.getValue(f.id(510)).evidence)
        assertEquals(ClosingRecordState.SAVED_ON_DEVICE, byRecord.getValue(f.id(510)).record)
        assertEquals(ClosingEvidenceState.AVAILABLE, byRecord.getValue(f.id(511)).evidence)
        assertEquals(ClosingRecordState.ACCEPTED, byRecord.getValue(f.id(511)).record)
        // A missing file is missing evidence, never available; no photo at all is missing too.
        assertEquals(ClosingEvidenceState.MISSING, byRecord.getValue(f.id(610)).evidence)
        assertEquals(ClosingEvidenceState.MISSING, byRecord.getValue(f.id(611)).evidence)
        assertEquals(ClosingRecordState.REFUSED, byRecord.getValue(f.id(611)).record)
        assertEquals("FORBIDDEN", byRecord.getValue(f.id(611)).code)
        assertEquals(ClosingEvidenceState.MISSING, closing.items.single { it.kind == ClosingItemKind.FINAL_READING }.evidence)
        // Amounts are per item, exactly as entered; the conference has no total.
        assertEquals(listOf(97_000L, 50_000L, 1_250L, 800L), listOf(f.id(510), f.id(511), f.id(610), f.id(611)).map { byRecord.getValue(it).paidCentavos })
        assertNull(byRecord.getValue(readingId).paidCentavos)
        assertFalse(closing.relatedUnavailable)
        assertEquals(ClosingSubmission.READY_TO_MARK, closing.submission)
        assertEquals(1L, closing.revision)
    }

    @Test fun `a photo of another record never counts and an unreadable index is never available`() {
        val inTrip = fueling(500, 510, "2026-11-15T13:00:00Z")
        val toll = expense(600, 610, "2026-11-15T13:10:00Z")
        // The fueling's operation owns a photo targeting the expense: it documents neither item here.
        val foreign = mapOf(inTrip.operationId to listOf(photo(inTrip, DocumentTargetKind.EXPENSE, f.id(610), 1, AttachmentState.STORED)))
        val closing = TripClosings.of(finished(), foreign, listOf(fv(inTrip)), listOf(ev(toll)))
        assertTrue(closing.items.filter { it.kind == ClosingItemKind.FUELING || it.kind == ClosingItemKind.EXPENSE }
            .all { it.evidence == ClosingEvidenceState.MISSING && it.photos == 0 })
        val unreadable = TripClosings.of(finished(), null, listOf(fv(inTrip)), listOf(ev(toll)))
        assertTrue(unreadable.items.all { it.evidence == ClosingEvidenceState.UNREADABLE })
        assertTrue(unreadable.items.none { it.evidence == ClosingEvidenceState.AVAILABLE })
        // Unreadable fuelings or expenses are said, never shown as "nothing to confer".
        val partial = TripClosings.of(finished(), emptyMap(), null, listOf(ev(toll)))
        assertTrue(partial.relatedUnavailable)
        assertEquals(1, partial.items.count { it.kind == ClosingItemKind.EXPENSE })
    }

    @Test fun `submission is offered once for a finished trip and never as a decision`() {
        assertEquals(ClosingSubmission.ALREADY_MARKED, TripClosings.of(finished(submitted = true), emptyMap(), emptyList(), emptyList()).submission)
        assertNull(TripClosings.of(finished(submitted = true), emptyMap(), emptyList(), emptyList()).revision)
        val open = TripPresentation.view(f.local(), f.saved(start))
        val closing = TripClosings.of(open, emptyMap(), emptyList(), emptyList())
        assertEquals(ClosingSubmission.NOT_FINISHED, closing.submission)
        assertNull(closing.revision)
        // An open trip confers everything since its start.
        val late = fueling(505, 515, "2026-11-16T09:00:00Z")
        assertEquals(1, TripClosings.of(open, emptyMap(), listOf(fv(late)), emptyList()).items.count { it.kind == ClosingItemKind.FUELING })
        assertEquals("EnterpriseTripClosing(redacted)", closing.toString())
    }
}
