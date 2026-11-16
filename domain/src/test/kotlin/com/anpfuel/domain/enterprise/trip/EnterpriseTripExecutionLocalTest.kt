package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** EMP-TRIP-021 (B-BR-EMP-529/530, EDR-344): the trip-book v2 execution rules, its v1 reading and the lifecycle records. */
class EnterpriseTripExecutionLocalTest {
    private val f = TripLocalFixture
    private fun book() = TripBook.empty(f.member, f.enrollment)
    private fun refused(failure: TripLocalFailure, block: () -> Unit) =
        assertEquals(failure, assertThrows(TripLocalException::class.java) { block() }.failure)
    private fun mission(trip: Int = 10, asset: Int = 20, start: Int = 100, mission: Int = 70) =
        LocalTrip(f.id(trip), f.id(asset), TripAssetLabel("Caminhão sintético", "FROTA-12", "ABC1D23"), false, f.id(start),
            f.at("2026-11-15T12:00:00Z"), missionId = f.id(mission))
    private fun lifecycle(op: Int, command: TripCommand, revision: Long, reason: String? = null, at: String, trip: Int = 10, start: Int = 100) =
        TripOperations.lifecycle(f.envelope(op, at), command, f.id(trip), revision, reason, f.id(start))

    @Test fun `B-BR-EMP-530 pause, resume and cancel follow the local state and the revision counts the earlier commands`() {
        val running = book().withStart(f.local())
        assertEquals(1L, running.trip(f.id(10))!!.nextRevision)
        refused(TripLocalFailure.NOT_PAUSED) { running.withResume(f.id(10), f.id(500), f.at("2026-11-15T13:00:00Z")) }
        refused(TripLocalFailure.UNKNOWN_TRIP) { running.withPause(f.id(11), f.id(500), f.at("2026-11-15T13:00:00Z")) }
        refused(TripLocalFailure.BEFORE_START) { running.withPause(f.id(10), f.id(500), f.at("2026-11-15T11:00:00Z")) }
        val paused = running.withPause(f.id(10), f.id(500), f.at("2026-11-15T13:00:00Z"))
        assertEquals(TripLocalState.PAUSED, paused.trip(f.id(10))!!.state)
        assertEquals(2L, paused.trip(f.id(10))!!.nextRevision)
        refused(TripLocalFailure.NOT_RUNNING) { paused.withPause(f.id(10), f.id(501), f.at("2026-11-15T13:10:00Z")) }
        refused(TripLocalFailure.DUPLICATE) { paused.withResume(f.id(10), f.id(500), f.at("2026-11-15T13:10:00Z")) }
        // Events are recorded while paused; the asset stays held.
        val noted = paused.withEvent(f.id(10), f.id(200), f.at("2026-11-15T13:05:00Z"))
        refused(TripLocalFailure.ASSET_HAS_UNFINISHED_TRIP) { noted.withStart(f.local(trip = 11, start = 101)) }
        val resumed = noted.withResume(f.id(10), f.id(501), f.at("2026-11-15T13:30:00Z"))
        assertEquals(TripLocalState.IN_PROGRESS, resumed.trip(f.id(10))!!.state)
        assertEquals(3L, resumed.trip(f.id(10))!!.nextRevision)
        assertEquals(listOf(TripLifecycleEntry(TripCommand.PAUSE, f.id(500)), TripLifecycleEntry(TripCommand.RESUME, f.id(501))),
            resumed.trip(f.id(10))!!.lifecycle)
        assertEquals(listOf(f.id(100), f.id(200), f.id(500), f.id(501)), resumed.trip(f.id(10))!!.operationIds)
        // A finish is recorded from a paused trip as well.
        val pausedAgain = resumed.withPause(f.id(10), f.id(502), f.at("2026-11-15T14:00:00Z"))
        val finished = pausedAgain.withFinish(f.id(10), f.id(300), f.at("2026-11-15T15:00:00Z"))
        assertEquals(TripLocalState.FINISHED, finished.trip(f.id(10))!!.state)
        refused(TripLocalFailure.TRIP_NOT_OPEN) { finished.withCancel(f.id(10), f.id(503), f.at("2026-11-15T15:10:00Z")) }
        refused(TripLocalFailure.TRIP_NOT_OPEN) { finished.withResume(f.id(10), f.id(503), f.at("2026-11-15T15:10:00Z")) }
    }

    @Test fun `a cancellation closes the trip and frees its asset on this device`() {
        val paused = book().withStart(f.local()).withPause(f.id(10), f.id(500), f.at("2026-11-15T13:00:00Z"))
        val cancelled = paused.withCancel(f.id(10), f.id(501), f.at("2026-11-15T13:10:00Z"))
        val trip = cancelled.trip(f.id(10))!!
        assertEquals(TripLocalState.CANCELLED, trip.state)
        refused(TripLocalFailure.TRIP_NOT_OPEN) { cancelled.withEvent(f.id(10), f.id(200), f.at("2026-11-15T13:20:00Z")) }
        refused(TripLocalFailure.TRIP_NOT_OPEN) { cancelled.withFinish(f.id(10), f.id(300), f.at("2026-11-15T13:20:00Z")) }
        refused(TripLocalFailure.TRIP_NOT_OPEN) { cancelled.withCancel(f.id(10), f.id(502), f.at("2026-11-15T13:20:00Z")) }
        refused(TripLocalFailure.NOT_FINISHED) { cancelled.withSubmission(f.id(10), f.id(400)) }
        assertEquals(2, cancelled.withStart(f.local(trip = 11, start = 101)).trips.size)
        // A running trip is cancelled directly as well.
        assertEquals(TripLocalState.CANCELLED, book().withStart(f.local()).withCancel(f.id(10), f.id(501), f.at("2026-11-15T13:10:00Z"))
            .trip(f.id(10))!!.state)
        // A refused start stays refused whatever was recorded after it.
        assertEquals(TripLocalState.START_REFUSED, paused.withStartRefused(f.id(10), f.id(100)).trip(f.id(10))!!.state)
    }

    @Test fun `B-BR-EMP-529 one live local trip per mission, freed only by a refused start`() {
        val started = book().withStart(mission())
        assertEquals(f.id(10), started.missionTrip(f.id(70))!!.tripId)
        assertNull(started.missionTrip(f.id(71)))
        refused(TripLocalFailure.MISSION_ALREADY_STARTED) { started.withStart(mission(trip = 11, asset = 21, start = 101)) }
        // A finished or cancelled trip still executed the mission; only a refused start frees it.
        val finished = started.withFinish(f.id(10), f.id(300), f.at("2026-11-15T15:00:00Z"))
        refused(TripLocalFailure.MISSION_ALREADY_STARTED) { finished.withStart(mission(trip = 11, asset = 21, start = 101)) }
        val cancelled = started.withCancel(f.id(10), f.id(500), f.at("2026-11-15T13:00:00Z"))
        refused(TripLocalFailure.MISSION_ALREADY_STARTED) { cancelled.withStart(mission(trip = 11, asset = 21, start = 101)) }
        val refusedStart = started.withStartRefused(f.id(10), f.id(100))
        assertNull(refusedStart.missionTrip(f.id(70)))
        assertEquals(f.id(11), refusedStart.withStart(mission(trip = 11, asset = 21, start = 101)).missionTrip(f.id(70))!!.tripId)
    }

    @Test fun `trip-book v2 is canonical and strict and every v1 index is read unchanged`() {
        val full = book().withStart(mission()).withPause(f.id(10), f.id(500), f.at("2026-11-15T13:00:00Z"))
            .withResume(f.id(10), f.id(501), f.at("2026-11-15T13:10:00Z")).withEvent(f.id(10), f.id(200), f.at("2026-11-15T13:20:00Z"))
            .withCancel(f.id(10), f.id(502), f.at("2026-11-15T14:00:00Z")).withStart(f.local(trip = 11, asset = 21, start = 101))
        val decoded = TripBook.decode(full.canonicalBytes)
        assertArrayEquals(full.canonicalBytes, decoded.canonicalBytes)
        assertEquals(f.id(70), decoded.trip(f.id(10))!!.missionId)
        assertEquals(listOf(TripCommand.PAUSE, TripCommand.RESUME, TripCommand.CANCEL), decoded.trip(f.id(10))!!.lifecycle.map { it.command })
        assertEquals(TripLocalState.CANCELLED, decoded.trip(f.id(10))!!.state)
        assertNull(decoded.trip(f.id(11))!!.missionId)
        val text = String(full.canonicalBytes)
        assertTrue(text.contains(""""lifecycle":[{"command":"PAUSE","operation_id":"${f.id(500)}"}"""), text)
        assertTrue(text.contains(""""mission_id":"${f.id(70)}""""), text)
        // The EMP-TRIP-005 v1 index (exact bytes as written by that version) is read with no mission and no command.
        val v1 = """{"enrollment_id":"${f.enrollment}","format":"trip-book","membership_id":"${f.member}","trips":[{"asset_id":"${f.id(20)}",""" +
            """"event_operation_ids":["${f.id(200)}"],"finish_operation_id":null,"finished_at":null,"label":{"fleet_number":"FROTA-12",""" +
            """"name":"Caminhão sintético","plate":"ABC1D23"},"review_warning_accepted":false,"start_operation_id":"${f.id(100)}",""" +
            """"start_refused":false,"started_at":"2026-11-15T12:00:00Z","submit_operation_id":null,"trip_id":"${f.id(10)}"}],"version":1}"""
        val upgraded = TripBook.decode(v1.toByteArray())
        val trip = upgraded.trip(f.id(10))!!
        assertEquals(listOf(f.id(100), f.id(200)), trip.operationIds)
        assertNull(trip.missionId); assertTrue(trip.lifecycle.isEmpty()); assertEquals(TripLocalState.IN_PROGRESS, trip.state)
        assertTrue(String(upgraded.canonicalBytes).endsWith(""""version":2}"""))
        // A v1 index carrying a v2 field, or a v2 index missing one, is refused, never guessed.
        for (bad in listOf(v1.replace("\"trip_id\"", "\"mission_id\":null,\"trip_id\""), text.replace("\"mission_id\":null,", ""),
            text.replace("\"command\":\"PAUSE\"", "\"command\":\"FINISH\""), text.replace("\"command\":\"PAUSE\"", "\"command\":\"STOP\""),
            v1.replace("\"version\":1", "\"version\":0"))) {
            assertThrows(IllegalArgumentException::class.java, { TripBook.decode(bad.toByteArray()) }, bad)
        }
    }

    @Test fun `lifecycle records carry their own sync state and the mission comes from the start's reference`() {
        val start = TripOperations.start(f.envelope(100, "2026-11-15T12:00:00Z"), f.id(10), f.id(20), f.id(30), f.reading(1100, "184320000"),
            null, TripClockReading("boot-3", 1_000), missionId = f.id(70))
        val pause = lifecycle(500, TripCommand.PAUSE, 1, at = "2026-11-15T13:00:00Z")
        val resume = lifecycle(501, TripCommand.RESUME, 2, "Retomada", at = "2026-11-15T13:30:00Z")
        val cancel = lifecycle(502, TripCommand.CANCEL, 3, "Cliente cancelou", at = "2026-11-15T14:00:00Z")
        val local = mission().let { book().withStart(it) }.withPause(f.id(10), f.id(500), f.at("2026-11-15T13:00:00Z"))
            .withResume(f.id(10), f.id(501), f.at("2026-11-15T13:30:00Z")).withCancel(f.id(10), f.id(502), f.at("2026-11-15T14:00:00Z"))
        val entries = f.saved(start, resume, cancel) + mapOf(pause.operationId to EnterpriseQueueEntry(pause, acknowledgement = f.ack(pause, revision = 2)))
        val view = TripPresentation.view(local.trip(f.id(10))!!, entries)
        assertEquals(f.id(70), view.missionId)
        assertEquals(listOf(TripCommand.PAUSE, TripCommand.RESUME, TripCommand.CANCEL), view.lifecycle.map { (it.fact as TripOperationFact.Lifecycle).command })
        assertEquals(listOf(EnterpriseSyncStatus.ACCEPTED, EnterpriseSyncStatus.SAVED_ON_DEVICE, EnterpriseSyncStatus.SAVED_ON_DEVICE),
            view.lifecycle.map { it.sync })
        assertEquals(listOf(1L, 2L, 3L), view.lifecycle.map { (it.fact as TripOperationFact.Lifecycle).revision })
        // A held command shows its reason; the start's own review keeps the later records waiting.
        val held = entries + mapOf(cancel.operationId to EnterpriseQueueEntry(cancel,
            acknowledgement = f.ack(cancel, "ACCEPTED_FOR_REVIEW", "STALE_REVISION", null)))
        assertEquals("STALE_REVISION", TripPresentation.view(local.trip(f.id(10))!!, held).lifecycle.last().code)
        val heldStart = entries + mapOf(start.operationId to EnterpriseQueueEntry(start,
            acknowledgement = f.ack(start, "ACCEPTED_FOR_REVIEW", "ORPHAN_REFERENCE", null)))
        val waiting = TripPresentation.view(local.trip(f.id(10))!!, heldStart)
        assertEquals("ORPHAN_REFERENCE", waiting.start.code)
        assertTrue(waiting.waitingForStartReview)
        // A record of another trip, or another command than indexed, is refused.
        val otherTrip = lifecycle(500, TripCommand.PAUSE, 1, at = "2026-11-15T13:00:00Z", trip = 11)
        assertThrows(IllegalArgumentException::class.java) {
            TripPresentation.view(local.trip(f.id(10))!!, entries + mapOf(otherTrip.operationId to EnterpriseQueueEntry(otherTrip)))
        }
        val otherCommand = lifecycle(500, TripCommand.RESUME, 1, at = "2026-11-15T13:00:00Z")
        assertThrows(IllegalArgumentException::class.java) {
            TripPresentation.view(local.trip(f.id(10))!!, entries + mapOf(otherCommand.operationId to EnterpriseQueueEntry(otherCommand)))
        }
    }

    @Test fun `decoders refuse what the builders never produce`() {
        val pause = lifecycle(500, TripCommand.PAUSE, 1, at = "2026-11-15T13:00:00Z")
        val raw = String(pause.canonicalBytes)
        fun reseal(text: String): EnterpriseOperation = EnterpriseOperation.seal(
            text.replace(Regex(",\"payload_digest\":\"sha256:[0-9a-f]{64}\""), "").toByteArray(), f.binding)
        fun failure(op: EnterpriseOperation) = assertThrows(TripOperationException::class.java) { TripOperations.read(op) }.failure
        // A reference or an attachment is a recognised but unsupported form; two dependencies or no revision are malformed.
        assertEquals(TripOperationFailure.UNSUPPORTED, failure(reseal(raw.replace("\"soft_references\":[]",
            "\"soft_references\":[{\"id\":\"${f.id(70)}\",\"type\":\"mission\"}]"))))
        assertEquals(TripOperationFailure.MALFORMED, failure(reseal(raw.replace("\"depends_on\":[\"${f.id(100)}\"]",
            "\"depends_on\":[\"${f.id(100)}\",\"${f.id(101)}\"]"))))
        assertEquals(TripOperationFailure.MALFORMED, failure(reseal(raw.replace("\"expected_revision\":1", "\"expected_revision\":null"))))
        assertEquals(TripOperationFailure.MALFORMED, failure(reseal(raw.replace("\"entity\":{\"expected_revision\":1,\"id\":",
            "\"entity\":{\"expected_revision\":1,\"id\":").replace("\"type\":\"trip\"}", "\"type\":\"trip_event\"}"))))
        // A start with another reference type, or two references, stays unsupported.
        val start = String(f.start().canonicalBytes)
        assertEquals(TripOperationFailure.UNSUPPORTED, failure(reseal(start.replace("\"soft_references\":[]",
            "\"soft_references\":[{\"id\":\"${f.id(70)}\",\"type\":\"fueling\"}]"))))
        assertEquals(TripOperationFailure.UNSUPPORTED, failure(reseal(start.replace("\"soft_references\":[]",
            "\"soft_references\":[{\"id\":\"${f.id(70)}\",\"type\":\"mission\"},{\"id\":\"${f.id(71)}\",\"type\":\"mission\"}]"))))
        // An answer with a dependency or a reference is malformed (ParseRespond).
        val answer = String(TripMissionOperations.respond(f.envelope(600, "2026-11-15T12:00:00Z"), f.id(70), 2, "ACCEPT", null).canonicalBytes)
        for (bad in listOf(answer.replace("\"depends_on\":[]", "\"depends_on\":[\"${f.id(100)}\"]"),
            answer.replace("\"soft_references\":[]", "\"soft_references\":[{\"id\":\"${f.id(71)}\",\"type\":\"mission\"}]"),
            answer.replace("\"type\":\"mission\"}", "\"type\":\"trip\"}"))) {
            assertEquals(TripOperationFailure.MALFORMED,
                assertThrows(TripOperationException::class.java) { TripMissionOperations.read(reseal(bad)) }.failure, bad)
        }
        assertThrows(TripOperationException::class.java) { TripMissionOperations.read(pause) }
    }
}
