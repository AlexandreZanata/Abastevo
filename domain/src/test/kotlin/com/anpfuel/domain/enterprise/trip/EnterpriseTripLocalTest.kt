package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import java.time.Instant
import java.time.ZoneId
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Synthetic producer fixture shared by the EMP-TRIP-005 domain tests. */
internal object TripLocalFixture {
    fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    const val ORIGIN = "https://api.example.invalid"
    val binding: SyncBinding = SyncBinding.of(id(1), ORIGIN, id(2))
    val member = id(3)
    val enrollment = id(4)
    fun at(raw: String): Instant = EnterpriseTime.parseInstant(raw)
    fun envelope(op: Int, at: String) = TripEnvelope(binding, member, enrollment, id(op), TimeDeclaration(at(at), "America/Cuiaba", -14400))
    fun reading(n: Int, value: String) = TripReadingInput(id(60), id(61), id(n), "ODOMETER", value)
    fun start(op: Int = 100, trip: Int = 10, asset: Int = 20, at: String = "2026-11-15T12:00:00Z", value: String = "184320000"): EnterpriseOperation =
        TripOperations.start(envelope(op, at), id(trip), id(asset), id(30), reading(op + 1000, value), null, TripClockReading("boot-3", 1_000))
    fun event(op: Int, trip: Int = 10, type: String = "STOP", text: String? = null, at: String, start: Int = 100): EnterpriseOperation =
        TripOperations.event(envelope(op, at), id(op + 2000), id(trip), type, text, id(start))
    fun finish(op: Int = 300, trip: Int = 10, at: String = "2026-11-15T15:00:00Z", start: Int = 100): EnterpriseOperation =
        TripOperations.finish(envelope(op, at), id(trip), reading(op + 1000, "184610000"), null, TripClockReading("boot-3", 10_801_000), id(start))
    fun submit(op: Int = 400, trip: Int = 10, finish: Int = 300, at: String = "2026-11-15T15:01:00Z"): EnterpriseOperation =
        TripOperations.submit(envelope(op, at), id(trip), 1, id(finish))
    fun local(trip: Int = 10, asset: Int = 20, start: Int = 100, at: String = "2026-11-15T12:00:00Z") =
        LocalTrip(id(trip), id(asset), TripAssetLabel("Caminhão sintético", "FROTA-12", "ABC1D23"), false, id(start), at(at))
    fun ack(operation: EnterpriseOperation, result: String = "ACCEPTED", code: String? = null, revision: Long? = 1,
        received: String = "2026-11-15T16:00:00Z"): OperationAcknowledgement = OperationAcknowledgement.parse(
        ("""{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"$result",""" +
            """"code":${code?.let { "\"$it\"" } ?: "null"},"entity_revision":${revision ?: "null"},"received_at":"$received",""" +
            """"review_item_id":${if (result == "ACCEPTED_FOR_REVIEW") "\"${id(999)}\"" else "null"},"warnings":[]}""").toByteArray(), operation)
    fun saved(vararg operations: EnterpriseOperation) = operations.associate { it.operationId to EnterpriseQueueEntry(it) }
}

class EnterpriseTripLocalTest {
    private val f = TripLocalFixture
    private fun book() = TripBook.empty(f.member, f.enrollment)
    private fun refused(failure: TripLocalFailure, block: () -> Unit) =
        assertEquals(failure, assertThrows(TripLocalException::class.java) { block() }.failure)

    @Test fun `B-BR-EMP-377 one unfinished local trip per asset, freed by its finish or a refused start`() {
        val started = book().withStart(f.local())
        assertEquals(TripLocalState.IN_PROGRESS, started.trip(f.id(10))!!.state)
        refused(TripLocalFailure.ASSET_HAS_UNFINISHED_TRIP) { started.withStart(f.local(trip = 11, start = 101)) }
        // Another asset of the same partition is independent.
        val two = started.withStart(f.local(trip = 12, asset = 21, start = 102))
        assertEquals(2, two.trips.size)
        refused(TripLocalFailure.DUPLICATE) { two.withStart(f.local(trip = 10, asset = 22, start = 103)) }
        refused(TripLocalFailure.DUPLICATE) { two.withStart(f.local(trip = 13, asset = 22, start = 100)) }
        val finished = two.withFinish(f.id(10), f.id(300), f.at("2026-11-15T15:00:00Z"))
        assertEquals(TripLocalState.FINISHED, finished.trip(f.id(10))!!.state)
        assertEquals(3, finished.withStart(f.local(trip = 11, start = 101)).trips.size)
        val refusedStart = two.withStartRefused(f.id(10), f.id(100))
        assertEquals(TripLocalState.START_REFUSED, refusedStart.trip(f.id(10))!!.state)
        assertTrue(refusedStart.withStartRefused(f.id(10), f.id(100)).canonicalBytes.contentEquals(refusedStart.canonicalBytes), "idempotent")
        refused(TripLocalFailure.UNKNOWN_TRIP) { two.withStartRefused(f.id(10), f.id(101)) }
        assertEquals(3, refusedStart.withStart(f.local(trip = 11, start = 101)).trips.size)
    }

    @Test fun `events, finish and submission follow the local state and never precede the start`() {
        val book = book().withStart(f.local(at = "2026-11-15T12:00:00Z"))
        refused(TripLocalFailure.UNKNOWN_TRIP) { book.withEvent(f.id(11), f.id(200), f.at("2026-11-15T13:00:00Z")) }
        refused(TripLocalFailure.BEFORE_START) { book.withEvent(f.id(10), f.id(200), f.at("2026-11-15T11:59:59.999Z")) }
        refused(TripLocalFailure.BEFORE_START) { book.withFinish(f.id(10), f.id(300), f.at("2026-11-15T11:00:00Z")) }
        refused(TripLocalFailure.NOT_FINISHED) { book.withSubmission(f.id(10), f.id(400)) }
        val logged = book.withEvent(f.id(10), f.id(200), f.at("2026-11-15T12:00:00Z")).withEvent(f.id(10), f.id(201), f.at("2026-11-15T13:00:00Z"))
        refused(TripLocalFailure.DUPLICATE) { logged.withEvent(f.id(10), f.id(201), f.at("2026-11-15T13:00:00Z")) }
        assertEquals(listOf(f.id(200), f.id(201)), logged.trip(f.id(10))!!.eventOperationIds)
        val finished = logged.withFinish(f.id(10), f.id(300), f.at("2026-11-15T15:00:00Z"))
        refused(TripLocalFailure.TRIP_NOT_OPEN) { finished.withFinish(f.id(10), f.id(301), f.at("2026-11-15T16:00:00Z")) }
        refused(TripLocalFailure.TRIP_NOT_OPEN) { finished.withEvent(f.id(10), f.id(202), f.at("2026-11-15T14:00:00Z")) }
        val submitted = finished.withSubmission(f.id(10), f.id(400))
        refused(TripLocalFailure.ALREADY_SUBMITTED) { submitted.withSubmission(f.id(10), f.id(401)) }
        assertEquals(listOf(f.id(100), f.id(200), f.id(201), f.id(300), f.id(400)), submitted.trip(f.id(10))!!.operationIds)
        val refusedTrip = book.withStartRefused(f.id(10), f.id(100))
        refused(TripLocalFailure.TRIP_NOT_OPEN) { refusedTrip.withEvent(f.id(10), f.id(200), f.at("2026-11-15T13:00:00Z")) }
        refused(TripLocalFailure.TRIP_NOT_OPEN) { refusedTrip.withFinish(f.id(10), f.id(300), f.at("2026-11-15T13:00:00Z")) }
    }

    @Test fun `the trip-book codec is canonical, strict and bound to its membership and enrollment`() {
        val label = TripAssetLabel("Van \"sintética\" \\ ãé", "F-1", "")
        val full = book().withStart(LocalTrip(f.id(10), f.id(20), label, true, f.id(100), f.at("2026-11-15T12:00:00.000000123Z")))
            .withEvent(f.id(10), f.id(200), f.at("2026-11-15T13:00:00Z")).withFinish(f.id(10), f.id(300), f.at("2026-11-15T15:00:00Z"))
            .withSubmission(f.id(10), f.id(400)).withStart(f.local(trip = 11, asset = 21, start = 101)).withStartRefused(f.id(11), f.id(101))
        val decoded = TripBook.decode(full.canonicalBytes)
        assertArrayEquals(full.canonicalBytes, decoded.canonicalBytes)
        val trip = decoded.trip(f.id(10))!!
        assertEquals(label, trip.label); assertTrue(trip.reviewWarningAccepted); assertEquals(f.at("2026-11-15T12:00:00.000000123Z"), trip.startedAt)
        assertEquals(f.at("2026-11-15T15:00:00Z"), trip.finishedAt); assertEquals(f.id(400), trip.submitOperationId)
        assertTrue(decoded.trip(f.id(11))!!.startRefused)
        // EMP-TRIP-021 (EDR-344): the index is written as version 2; a version 1 index is still read (below).
        val empty = String(book().canonicalBytes)
        assertEquals("""{"enrollment_id":"${f.enrollment}","format":"trip-book","membership_id":"${f.member}","trips":[],"version":2}""", empty)
        for (bad in listOf(empty.replace("\"version\":2", "\"version\":3"), empty.replace("trip-book", "trip-list"),
            empty.replace("}", ",\"extra\":1}"), empty.replace(f.member, f.member.uppercase()), "{}", "[]", "")) {
            assertThrows(IllegalArgumentException::class.java, { TripBook.decode(bad.toByteArray()) }, bad)
        }
        assertEquals(full.canonicalBytes.size, full.requireBinding(f.member, f.enrollment).canonicalBytes.size)
        refused(TripLocalFailure.FOREIGN) { full.requireBinding(f.id(5), f.enrollment) }
        refused(TripLocalFailure.FOREIGN) { full.requireBinding(f.member, f.id(6)) }
    }

    @Test fun `the book is bounded instead of growing past its limits`() {
        var book = book().withStart(f.local())
        for (i in 0 until TripBook.MAX_EVENTS) book = book.withEvent(f.id(10), f.id(10_000 + i), f.at("2026-11-15T13:00:00Z"))
        refused(TripLocalFailure.FULL) { book.withEvent(f.id(10), f.id(20_000), f.at("2026-11-15T13:00:00Z")) }
        assertEquals(TripBook.MAX_EVENTS, TripBook.decode(book.canonicalBytes).trip(f.id(10))!!.eventOperationIds.size)
    }

    @Test fun `B-BR-EMP-378 declarations keep the device instant, an accepted zone and its offset`() {
        val sao = TripDeclarations.at(1_763_208_000_123, ZoneId.of("America/Sao_Paulo"))
        assertEquals(Instant.ofEpochMilli(1_763_208_000_123), sao.occurredAt)
        assertEquals("America/Sao_Paulo", sao.entryZone); assertEquals(-10_800, sao.deviceOffsetSeconds)
        // An alias the orgtime zone profile refuses falls back to UTC with the device's real offset.
        val alias = TripDeclarations.at(1_763_208_000_000, ZoneId.of("GMT+03:00"))
        assertEquals("UTC", alias.entryZone); assertEquals(10_800, alias.deviceOffsetSeconds)
        assertEquals("UTC", TripDeclarations.at(0, ZoneId.of("UTC")).entryZone)
    }

    @Test fun `B-BR-EMP-379 each record shows only its own receipt and acceptance is never early`() {
        val start = f.start(); val stop = f.event(200, at = "2026-11-15T13:00:00Z"); val note = f.event(201, type = "NOTE", text = "Calibrado", at = "2026-11-15T12:30:00Z")
        val finish = f.finish(); val submit = f.submit()
        val trip = f.local().let { TripBook.empty(f.member, f.enrollment).withStart(it) }.withEvent(f.id(10), stop.operationId, f.at("2026-11-15T13:00:00Z"))
            .withEvent(f.id(10), note.operationId, f.at("2026-11-15T12:30:00Z"))
        val open = TripPresentation.view(trip.trip(f.id(10))!!, f.saved(start, stop, note))
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, open.start.sync)
        assertEquals(listOf(note.operationId, stop.operationId), open.events.map { it.fact.operationId }, "occurrence order, not creation order")
        assertNull(open.finish); assertNull(open.submission); assertEquals(TripReviewLine.NOT_READY, open.review); assertFalse(open.waitingForStartReview)
        val done = trip.withFinish(f.id(10), finish.operationId, f.at("2026-11-15T15:00:00Z")).withSubmission(f.id(10), submit.operationId).trip(f.id(10))!!
        val entries = f.saved(start, stop, note, finish, submit).toMutableMap()
        val ready = TripPresentation.view(done, entries)
        assertEquals(TripReviewLine.READY_TO_SEND, ready.review)
        assertTrue(listOf(ready.start, ready.finish!!, ready.submission!!).all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE })
        // Only the start answered: everything else is still on the device.
        entries[start.operationId] = EnterpriseQueueEntry(start, acknowledgement = f.ack(start))
        val partial = TripPresentation.view(done, entries)
        assertEquals(EnterpriseSyncStatus.ACCEPTED, partial.start.sync)
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, partial.finish!!.sync)
        assertEquals(TripReviewLine.READY_TO_SEND, partial.review)
        for (op in listOf(stop, note)) entries[op.operationId] = EnterpriseQueueEntry(op, acknowledgement = f.ack(op))
        entries[finish.operationId] = EnterpriseQueueEntry(finish, acknowledgement = f.ack(finish, revision = 2))
        entries[submit.operationId] = EnterpriseQueueEntry(submit, acknowledgement = f.ack(submit))
        val accepted = TripPresentation.view(done, entries)
        assertTrue((listOf(accepted.start, accepted.finish!!, accepted.submission!!) + accepted.events).all { it.sync == EnterpriseSyncStatus.ACCEPTED })
        assertEquals(TripReviewLine.AWAITING_APPROVAL, accepted.review)
        assertEquals(f.at("2026-11-15T16:00:00Z"), accepted.finish!!.receivedAt)
        entries[submit.operationId] = EnterpriseQueueEntry(submit, acknowledgement = f.ack(submit, "REJECTED", "FORBIDDEN", null))
        val refusedSubmission = TripPresentation.view(done, entries)
        assertEquals(TripReviewLine.SUBMISSION_REFUSED, refusedSubmission.review)
        assertEquals("FORBIDDEN", refusedSubmission.submission!!.code)
    }

    @Test fun `a held start shows its reason and its waiting children, a missing entry is refused`() {
        val start = f.start(); val stop = f.event(200, at = "2026-11-15T13:00:00Z"); val finish = f.finish()
        val trip = TripBook.empty(f.member, f.enrollment).withStart(f.local()).withEvent(f.id(10), stop.operationId, f.at("2026-11-15T13:00:00Z"))
            .withFinish(f.id(10), finish.operationId, f.at("2026-11-15T15:00:00Z")).trip(f.id(10))!!
        for (code in listOf("ASSET_BUSY", "ASSET_UNAVAILABLE", "PERMISSION_CHANGED", "TRIP_ALREADY_CLOSED")) {
            val entries = f.saved(start, stop, finish).toMutableMap()
            entries[start.operationId] = EnterpriseQueueEntry(start, acknowledgement = f.ack(start, "ACCEPTED_FOR_REVIEW", code, null))
            val held = TripPresentation.view(trip, entries)
            assertEquals(EnterpriseSyncStatus.NEEDS_REVIEW, held.start.sync); assertEquals(code, held.start.code)
            assertTrue(held.waitingForStartReview)
            assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, held.finish!!.sync)
        }
        val refusedEntries = f.saved(start, stop, finish).toMutableMap()
        refusedEntries[start.operationId] = EnterpriseQueueEntry(start, acknowledgement = f.ack(start, "REJECTED", "MALFORMED", null))
        val refusedView = TripPresentation.view(trip, refusedEntries)
        assertEquals(EnterpriseSyncStatus.REJECTED, refusedView.start.sync); assertFalse(refusedView.waitingForStartReview)
        assertThrows(IllegalArgumentException::class.java) { TripPresentation.view(trip, f.saved(start, finish)) }
        // An entry whose operation is not this trip's record is refused, never shown.
        assertThrows(IllegalArgumentException::class.java) {
            TripPresentation.view(trip, f.saved(start, finish) + (stop.operationId to EnterpriseQueueEntry(f.event(200, trip = 11, at = "2026-11-15T13:00:00Z"))))
        }
    }

    @Test fun `trips are listed newest start first`() {
        val a = f.start(); val b = f.start(op = 101, trip = 11, asset = 21, at = "2026-11-15T14:00:00Z")
        val book = TripBook.empty(f.member, f.enrollment).withStart(f.local()).withStart(f.local(trip = 11, asset = 21, start = 101, at = "2026-11-15T14:00:00Z"))
        assertEquals(listOf(f.id(11), f.id(10)), TripPresentation.views(book, f.saved(a, b)).map { it.trip.tripId })
    }
}
