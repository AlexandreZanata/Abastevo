package com.anpfuel.domain.enterprise.tracking

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.trip.TripState
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertSame
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** EMP-GPS-002 local tracking book and decision rules (B-BR-EMP-495..499, EDR-321..324). Synthetic identifiers only. */
class TrackingBookTest {
    private fun id(n: Int) = "9b000000-0000-4000-8000-%012d".format(n)
    private val org = id(1)
    private val member = id(31)
    private val other = id(32)
    private val enrollment = id(41)
    private val trip = id(21)
    private val session = id(11)
    private val tripStart = id(600)
    private val binding = SyncBinding.of(id(51), "https://api.example.invalid", org)
    private fun at(minutes: Long): Instant = Instant.parse("2026-11-15T10:00:00Z").plusSeconds(minutes * 60)
    private fun envelope(op: Int, minutes: Long) = TrackingEnvelope(binding, member, enrollment, id(op),
        TimeDeclaration(at(minutes), "America/Cuiaba", -14_400))
    private fun facts(state: TripState = TripState.IN_PROGRESS, driver: String = member) = TrackingDecisions.facts(org, trip, driver, state)
    private fun start(op: Int, minutes: Long, permission: String = "PRECISE", visible: Boolean = true, origin: String = "TRIP_SCREEN",
        deps: List<String> = listOf(tripStart)) =
        TrackingOperations.read(TrackingOperations.start(envelope(op, minutes), session, trip, origin, permission, visible, deps))
    private fun resume(op: Int, minutes: Long, previous: String, permission: String = "PRECISE") =
        TrackingOperations.read(TrackingOperations.resume(envelope(op, minutes), session, trip, "TRIP_SCREEN", permission, true, listOf(previous)))
    private fun event(type: String, op: Int, minutes: Long, cause: TrackingCause, deps: List<String>) =
        TrackingOperations.read(TrackingOperations.event(type, envelope(op, minutes), session, trip, cause.name, deps))

    /** Applies operations in order like the producer: decide, then record. */
    private fun apply(local: LocalTrackingSession?, fact: TrackingOperationFact, trip: TrackingTripFacts = facts(),
        policy: TrackingPolicy? = TrackingPolicy.EXPECTED): LocalTrackingSession {
        val existing = local?.let { TrackingDecisions.rebuild(org, member, it) }
        val next = TrackingDecisions.decide(org, member, fact, trip, existing, policy)
        return TrackingDecisions.record(local, next, fact.operationId)
    }

    @Test fun `a first start opens and records at one instant, every row naming its operation`() {
        val recorded = apply(null, start(701, 1))
        assertEquals(listOf(TrackingSessionState.NOT_STARTED, TrackingSessionState.RECORDING), recorded.transitions.map { it.state })
        assertEquals(listOf(id(701), id(701)), recorded.transitions.map { it.operationId })
        assertEquals(listOf(at(1), at(1)), recorded.transitions.map { it.at })
        assertEquals(LocationPermission.PRECISE, recorded.transitions.last().permission)
        assertEquals(listOf(id(701)), recorded.operationIds)
        assertEquals(TrackingPolicy.EXPECTED, recorded.policy)
        assertEquals(TrackingSessionState.RECORDING, TrackingDecisions.rebuild(org, member, recorded).state)
    }

    @Test fun `a denied or hidden start is a recorded attempt, a retry starts the same session`() {
        val denied = apply(null, start(701, 1, permission = "NONE"))
        assertEquals(TrackingSessionState.NOT_STARTED, denied.state)
        assertEquals(TrackingCause.PERMISSION_DENIED, denied.transitions.last().cause)
        val hidden = apply(denied, start(702, 2, visible = false, deps = listOf(id(701))))
        assertEquals(TrackingCause.NOTIFICATION_BLOCKED, hidden.transitions.last().cause)
        assertEquals(TrackingSessionState.NOT_STARTED, hidden.state)
        val started = apply(hidden, start(703, 3, permission = "APPROXIMATE", deps = listOf(id(702))))
        assertEquals(TrackingSessionState.RECORDING, started.state)
        assertEquals(listOf(id(701), id(702), id(703)), started.operationIds)
        assertTrue(TrackingDecisions.rebuild(org, member, started).coarse)
    }

    @Test fun `stop, interruption, resume, reduction and the finish replay exactly`() {
        var s = apply(null, start(701, 1))
        s = apply(s, event(TrackingOperations.INTERRUPT, 702, 5, TrackingCause.STOPPED_BY_DRIVER, listOf(id(701))))
        assertEquals(TrackingSessionState.INTERRUPTED, s.state)
        s = apply(s, resume(703, 6, id(702)))
        s = apply(s, event(TrackingOperations.REDUCE, 704, 7, TrackingCause.PRECISION_REDUCED, listOf(id(703))))
        assertEquals(LocationPermission.APPROXIMATE, TrackingDecisions.rebuild(org, member, s).precision)
        s = apply(s, event(TrackingOperations.INTERRUPT, 705, 8, TrackingCause.PROCESS_STOPPED, listOf(id(704))))
        s = apply(s, resume(706, 9, id(705)))
        s = apply(s, event(TrackingOperations.END, 707, 10, TrackingCause.TRIP_COMPLETED, listOf(id(706), id(900))), facts(TripState.COMPLETED))
        assertEquals(TrackingSessionState.ENDED, s.state)
        assertEquals((701..707).map(::id), s.operationIds)
        val rebuilt = TrackingDecisions.rebuild(org, member, s)
        assertEquals(s.transitions.size, rebuilt.history.size)
        // Precise 1..5, 6..7 and 9..10; approximate 7..8 after the reduction.
        assertEquals(listOf(LocationPermission.PRECISE, LocationPermission.PRECISE, LocationPermission.APPROXIMATE, LocationPermission.PRECISE),
            rebuilt.recordingPeriods.map { it.precision })
        assertTrue(rebuilt.recordingPeriods.all { it.end != null })
    }

    @Test fun `the domain refuses what the device must never store`() {
        val recording = apply(null, start(701, 1))
        // Implicit and personal origins, another member's trip, a trip not in progress, policy OFF or unknown, a duplicate session.
        val refused = listOf<() -> Unit>(
            { apply(null, start(701, 1, origin = "BOOT")) },
            { apply(null, start(701, 1, origin = "WORKER")) },
            { apply(null, start(701, 1, origin = "PERSONAL")) },
            { apply(null, start(701, 1), facts(driver = other)) },
            { apply(null, start(701, 1), facts(TripState.COMPLETED)) },
            { apply(null, start(701, 1), policy = TrackingPolicy.OFF) },
            { apply(null, start(701, 1), policy = null) },
            { apply(recording, resume(702, 2, id(701))) },
            { apply(recording, event(TrackingOperations.INTERRUPT, 702, 0, TrackingCause.PROCESS_STOPPED, listOf(id(701)))) },
            { apply(recording, event(TrackingOperations.END, 702, 2, TrackingCause.TRIP_COMPLETED, listOf(id(701)))) },
            { apply(recording, event(TrackingOperations.INTERRUPT, 702, 2, TrackingCause.OWNER_CHANGED, listOf(id(701))), facts(TripState.COMPLETED)) },
        )
        for (attempt in refused) assertThrows(Exception::class.java) { attempt() }
        // A second session for the same trip opened over the existing one is refused (BR-GPS-008).
        val second = TrackingOperations.read(TrackingOperations.start(envelope(709, 2), id(12), trip, "TRIP_SCREEN", "PRECISE", true, listOf(tripStart)))
        assertThrows(TrackingDomainException::class.java) {
            TrackingDecisions.decide(org, member, second, facts(), TrackingDecisions.rebuild(org, member, recording), TrackingPolicy.EXPECTED)
        }
        // Another responsible member: the old owner interrupts with OWNER_CHANGED and collects nothing afterwards.
        val handed = TrackingDecisions.decide(org, member, event(TrackingOperations.INTERRUPT, 702, 3, TrackingCause.OWNER_CHANGED, listOf(id(701))),
            facts(driver = other), TrackingDecisions.rebuild(org, member, recording), null)
        assertEquals(TrackingSessionState.INTERRUPTED, handed.state)
        assertNull(handed.collecting(org, facts(driver = other)))
        assertThrows(TrackingDomainException::class.java) {
            TrackingDecisions.decide(org, member, resume(703, 4, id(702)), facts(driver = other), handed, null)
        }
    }

    @Test fun `the book is canonical, strict, bound and keeps one session per trip`() {
        val empty = TrackingBook.empty(member, enrollment)
        assertNull(empty.policy)
        val read = TrackingPolicyRead(TrackingPolicy.OPTIONAL, 3, "aviso-2026.1", at(-60), at(0))
        val recorded = apply(null, start(701, 1))
        val book = empty.withPolicy(read).withSession(recorded).withCheckpoint(session, TrackingCheckpoint("boot-1", at(2)))
        val decoded = TrackingBook.decode(book.canonicalBytes)
        assertArrayEquals(book.canonicalBytes, decoded.canonicalBytes)
        assertEquals(TrackingPolicy.OPTIONAL, decoded.policy!!.policy)
        assertEquals("aviso-2026.1", decoded.policy!!.noticeVersion)
        assertEquals(TrackingCheckpoint("boot-1", at(2)), decoded.session(trip)!!.checkpoint)
        assertEquals(listOf(id(701)), decoded.sessionById(session)!!.operationIds)
        assertSame(decoded.requireBinding(member, enrollment), decoded)
        assertThrows(IllegalArgumentException::class.java) { decoded.requireBinding(other, enrollment) }
        // Unknown version, an extra key and a non-canonical spelling are refused, never guessed.
        val text = String(book.canonicalBytes)
        for (tampered in listOf(text.replace("\"version\":1", "\"version\":2"), text.replace("{\"enrollment_id\"", "{\"x\":1,\"enrollment_id\""),
            text.replace("\"format\":\"tracking-book\"", "\"format\" : \"tracking-book\""), text.replace("RECORDING", "PAUSED"))) {
            assertThrows(Exception::class.java) { TrackingBook.decode(tampered.toByteArray()) }
        }
        // A second session for the same trip and a foreign trip or policy for an existing session are refused.
        val otherSession = LocalTrackingSession(id(12), trip, TrackingPolicy.EXPECTED, recorded.transitions.map {
            StoredTrackingTransition(it.state, it.cause, it.actor, it.permission, it.at, id(801)) })
        assertThrows(IllegalArgumentException::class.java) { book.withSession(otherSession) }
        assertThrows(IllegalArgumentException::class.java) {
            book.withSession(LocalTrackingSession(session, id(22), TrackingPolicy.EXPECTED, recorded.transitions))
        }
        // A policy read is consistent: OFF has no notice, an enabled policy has one and a revision.
        assertThrows(IllegalArgumentException::class.java) { TrackingPolicyRead(TrackingPolicy.OFF, 1, "aviso", at(0), at(0)) }
        assertThrows(IllegalArgumentException::class.java) { TrackingPolicyRead(TrackingPolicy.EXPECTED, 0, "aviso", at(0), at(0)) }
        assertThrows(IllegalArgumentException::class.java) { TrackingPolicyRead(TrackingPolicy.EXPECTED, 1, "Aviso X", at(0), at(0)) }
        TrackingPolicyRead(TrackingPolicy.OFF, 0, null, null, at(0))
    }

    @Test fun `a final refusal stops the session once and a tampered history does not replay`() {
        val recorded = apply(null, start(701, 1))
        val book = TrackingBook.empty(member, enrollment).withSession(recorded)
        val refused = book.withRefusal(session, id(701))
        assertTrue(refused.sessionById(session)!!.refused)
        assertArrayEquals(refused.canonicalBytes, refused.withRefusal(session, id(701)).canonicalBytes)
        assertArrayEquals(book.canonicalBytes, book.withRefusal(session, id(999)).canonicalBytes, "an unknown operation changes nothing")
        assertFalse(book.sessionById(session)!!.refused)
        val tampered = LocalTrackingSession(session, trip, TrackingPolicy.EXPECTED, recorded.transitions.mapIndexed { i, t ->
            StoredTrackingTransition(t.state, t.cause, t.actor, if (i == 1) LocationPermission.NONE else t.permission, t.at, t.operationId) })
        assertThrows(Exception::class.java) { TrackingDecisions.rebuild(org, member, tampered) }
        assertEquals("EnterpriseTrackingBook(redacted)", book.toString())
        assertEquals("EnterpriseLocalTrackingSession(redacted)", recorded.toString())
        assertFalse(EnterpriseTime.formatInstant(at(1)) in book.toString())
    }
}
