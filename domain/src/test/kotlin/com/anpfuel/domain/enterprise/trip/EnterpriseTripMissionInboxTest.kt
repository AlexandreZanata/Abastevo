package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Synthetic missions shared by the EMP-TRIP-021 domain, application and data tests. */
object MissionInboxFixture {
    fun id(n: Int) = TripLocalFixture.id(n)
    val member get() = TripLocalFixture.member
    val enrollment get() = TripLocalFixture.enrollment
    val employee = id(30)
    fun at(raw: String) = TripLocalFixture.at(raw)
    fun plan(text: String? = "Cliente Norte, doca 3", unit: String? = null, load: TripMissionLoad = TripMissionLoad.CARGO,
        cargo: String? = "Caixas de peças", passengers: Int? = null) = MissionPlanView(2, text, unit, "Entrega programada", id(80), load,
        cargo, passengers, at("2026-11-15T10:10:00.123456Z"))
    fun item(mission: Int = 70, state: TripMissionState = TripMissionState.ASSIGNED, revision: Long = 3, asset: Int? = 20,
        employee: String = this.employee, answer: TripMissionResponse? = null, reason: String? = null, plan: MissionPlanView? = plan(),
        starts: String = "2026-11-20T11:00:00Z") = MissionInboxItem(id(mission), id(81), asset?.let(::id), "Entrega no cliente Norte",
        at(starts), at(starts).plusSeconds(4 * 3600), state, revision, employee, answer, reason, plan,
        at("2026-11-15T10:00:00.123456Z"), at("2026-11-15T10:30:00.123456Z"))
    fun page(vararg items: MissionInboxItem, truncated: Boolean = false) = MissionInboxPage(items.toList(), truncated)
    fun clock(elapsed: Long, wall: Long = 1_763_200_000_000L + elapsed, boot: String = "boot-3") = EnterpriseDeviceClock(boot, elapsed, wall)
    fun respond(op: Int, mission: Int = 70, revision: Long = 3, response: String = "ACCEPT", reason: String? = null,
        at: String = "2026-11-15T12:00:00Z"): EnterpriseOperation =
        TripMissionOperations.respond(TripLocalFixture.envelope(op, at), id(mission), revision, response, reason)
    fun line(op: EnterpriseOperation): MissionResponseLine = TripMissionOperations.read(op).let {
        MissionResponseLine(it.missionId, it.operationId, it.revision, it.response)
    }
}

class EnterpriseTripMissionInboxTest {
    private val f = MissionInboxFixture
    private val t = TripLocalFixture
    private fun empty() = MissionInbox.empty(f.member, f.enrollment)
    private fun refused(failure: MissionLocalFailure, block: () -> Unit) =
        assertEquals(failure, assertThrows(MissionLocalException::class.java) { block() }.failure)
    private val hour = 3_600_000L

    @Test fun `B-BR-EMP-527 the copy is the member's own list, replaced whole and bound to its employee record`() {
        val inbox = empty().withPage(f.employee, f.page(f.item(), f.item(71, TripMissionState.ACCEPTED, 4, answer = TripMissionResponse.ACCEPT)), f.clock(0))
        assertEquals(listOf(f.id(70), f.id(71)), inbox.items.map { it.missionId })
        assertEquals(f.employee, inbox.employeeId)
        // An item of another assignee refuses the whole page and leaves the copy as it was.
        refused(MissionLocalFailure.FOREIGN) { inbox.withPage(f.employee, f.page(f.item(72), f.item(73, employee = f.id(31))), f.clock(10)) }
        // The membership's employee record never changes (EDR-054): a page for another one is foreign.
        refused(MissionLocalFailure.FOREIGN) { inbox.withPage(f.id(31), f.page(f.item(72, employee = f.id(31))), f.clock(10)) }
        // A later page replaces every item; the device receipt clock moves with it.
        val later = inbox.withPage(f.employee, f.page(f.item(72), truncated = true), f.clock(20))
        assertEquals(listOf(f.id(72)), later.items.map { it.missionId })
        assertTrue(later.truncated)
        assertEquals(20L, later.received!!.elapsedMillis)
        refused(MissionLocalFailure.FULL) { empty().withPage(f.employee, MissionInboxPage((0..MissionInbox.MAX_ITEMS).map { f.item(1000 + it) }, false), f.clock(0)) }
        refused(MissionLocalFailure.DUPLICATE) { empty().withPage(f.employee, f.page(f.item(), f.item()), f.clock(0)) }
    }

    @Test fun `B-BR-EMP-528 only an assigned mission at the copy's revision is answered once from this device`() {
        val inbox = empty().withPage(f.employee, f.page(f.item(), f.item(71, TripMissionState.ACCEPTED, 4, answer = TripMissionResponse.ACCEPT)), f.clock(0))
        refused(MissionLocalFailure.UNKNOWN_MISSION) { inbox.withResponse(f.line(f.respond(600, mission = 79))) }
        refused(MissionLocalFailure.NOT_ANSWERABLE) { inbox.withResponse(f.line(f.respond(600, mission = 71, revision = 4))) }
        refused(MissionLocalFailure.NOT_ANSWERABLE) { inbox.withResponse(f.line(f.respond(600, revision = 2))) }
        val answered = inbox.withResponse(f.line(f.respond(600)))
        refused(MissionLocalFailure.ALREADY_ANSWERED) { answered.withResponse(f.line(f.respond(601, response = "DECLINE", reason = "Folga"))) }
        refused(MissionLocalFailure.DUPLICATE) { answered.withResponse(f.line(f.respond(600, mission = 70, revision = 3))) }
        // The answer survives a refresh; a later revision may be answered again.
        val replanned = answered.withPage(f.employee, f.page(f.item(revision = 5)), f.clock(10))
        assertEquals(1, replanned.responses.size)
        assertEquals(2, replanned.withResponse(f.line(f.respond(601, revision = 5, response = "DECLINE", reason = "Folga"))).responses.size)
    }

    @Test fun `the mission-inbox codec is canonical, strict and bound to its membership and enrollment`() {
        val inbox = empty().withPage(f.employee, f.page(f.item(), f.item(71, TripMissionState.DECLINED, 4, asset = null,
            answer = TripMissionResponse.DECLINE, reason = "Folga \"marcada\" \\ ok", plan = f.plan(null, f.id(82), TripMissionLoad.PASSENGERS, null, 4)),
            f.item(72, TripMissionState.IN_PROGRESS, 6, plan = null), truncated = true), f.clock(5, boot = "boot-9"))
            .withResponse(f.line(f.respond(600)))
        val decoded = MissionInbox.decode(inbox.canonicalBytes)
        assertArrayEquals(inbox.canonicalBytes, decoded.canonicalBytes)
        val declined = decoded.item(f.id(71))!!
        assertNull(declined.assetId); assertEquals("Folga \"marcada\" \\ ok", declined.answerReason)
        assertEquals(TripMissionLoad.PASSENGERS, declined.plan!!.load); assertEquals(4, declined.plan!!.passengerCount)
        assertEquals(f.id(82), declined.plan!!.destinationUnitId)
        assertEquals(f.at("2026-11-15T10:10:00.123456Z"), decoded.item(f.id(70))!!.plan!!.plannedAt)
        assertEquals("boot-9", decoded.received!!.boot); assertTrue(decoded.truncated)
        assertEquals(listOf(f.id(600)), decoded.responses.map { it.operationId })
        val blank = String(empty().canonicalBytes)
        assertEquals("""{"employee_id":null,"enrollment_id":"${f.enrollment}","format":"mission-inbox","membership_id":"${f.member}",""" +
            """"missions":[],"received":null,"responses":[],"truncated":false,"version":1}""", blank)
        val text = String(inbox.canonicalBytes)
        for (bad in listOf(blank.replace("\"version\":1", "\"version\":2"), blank.replace("mission-inbox", "mission-list"),
            blank.replace("}", ",\"extra\":1}"), blank.replace(f.member, f.member.uppercase()), "{}", "",
            text.replace("\"state\":\"ASSIGNED\"", "\"state\":\"STARTED\""), text.replace("\"load\":\"CARGO\"", "\"load\":\"NONE\""),
            text.replace("\"revision\":3", "\"revision\":0"))) {
            assertThrows(IllegalArgumentException::class.java, { MissionInbox.decode(bad.toByteArray()) }, bad)
        }
        refused(MissionLocalFailure.FOREIGN) { inbox.requireBinding(f.id(5), f.enrollment) }
        refused(MissionLocalFailure.FOREIGN) { inbox.requireBinding(f.member, f.id(6)) }
        assertArrayEquals(inbox.canonicalBytes, inbox.requireBinding(f.member, f.enrollment).canonicalBytes)
    }

    @Test fun `item invariants follow the own list contract`() {
        // An accepted or declined mission carries the matching answer; an assigned one none; the window is ordered.
        for (bad in listOf({ f.item(state = TripMissionState.ACCEPTED) }, { f.item(state = TripMissionState.DECLINED, answer = TripMissionResponse.ACCEPT) },
            { f.item(answer = TripMissionResponse.ACCEPT) }, { f.item(revision = 0) },
            { f.item(plan = f.plan(load = TripMissionLoad.PASSENGERS, cargo = "x", passengers = 2)) },
            { f.item(plan = f.plan(text = null, unit = null)) }, { f.item(plan = f.plan(text = "CPF 123.456.789-09")) },
            { MissionInboxItem(f.id(70), f.id(81), null, "  ", f.at("2026-11-20T11:00:00Z"), f.at("2026-11-20T12:00:00Z"),
                TripMissionState.ASSIGNED, 3, f.employee, null, null, null, f.at("2026-11-15T10:00:00Z"), f.at("2026-11-15T10:00:00Z")) },
            { MissionInboxItem(f.id(70), f.id(81), null, "Ok", f.at("2026-11-20T11:00:00Z"), f.at("2026-11-20T11:00:00Z"),
                TripMissionState.ASSIGNED, 3, f.employee, null, null, null, f.at("2026-11-15T10:00:00Z"), f.at("2026-11-15T10:00:00Z")) })) {
            assertThrows(IllegalArgumentException::class.java) { bad() }
        }
    }

    @Test fun `BR-OFFL-016 the age comes from the device receipt clock and a stale or unknown age is labelled`() {
        assertEquals(0L, MissionInboxAge.ageMillis(f.clock(1_000), f.clock(1_000)))
        assertEquals(2 * hour, MissionInboxAge.ageMillis(f.clock(1_000), f.clock(1_000 + 2 * hour, wall = 5)))
        // After a reboot the wall clock decides; a clock moved back makes the age unknown.
        assertEquals(hour, MissionInboxAge.ageMillis(f.clock(1_000, wall = 10 * hour), f.clock(5, wall = 11 * hour, boot = "boot-4")))
        assertNull(MissionInboxAge.ageMillis(f.clock(1_000, wall = 10 * hour), f.clock(5, wall = 9 * hour, boot = "boot-4")))
        assertNull(MissionInboxAge.ageMillis(f.clock(1_000), f.clock(500)))
        val inbox = empty().withPage(f.employee, f.page(f.item()), f.clock(0))
        val fresh = MissionInboxPresentation.view(inbox, emptyMap(), null, f.clock(23 * hour))
        assertTrue(fresh.refreshed); assertFalse(fresh.stale); assertEquals(23 * hour, fresh.ageMillis)
        assertTrue(MissionInboxPresentation.view(inbox, emptyMap(), null, f.clock(24 * hour)).stale)
        val unknown = MissionInboxPresentation.view(inbox, emptyMap(), null, f.clock(5, wall = 0, boot = "boot-4"))
        assertTrue(unknown.stale); assertNull(unknown.ageMillis)
        val never = MissionInboxPresentation.view(null, emptyMap(), null, f.clock(0))
        assertFalse(never.refreshed); assertTrue(never.stale); assertTrue(never.missions.isEmpty())
    }

    @Test fun `the approved offline matrix decides each mission's eligible actions`() {
        val inbox = empty().withPage(f.employee, f.page(f.item(70), f.item(71, TripMissionState.ACCEPTED, 4, answer = TripMissionResponse.ACCEPT,
            starts = "2026-11-21T11:00:00Z"), f.item(72, TripMissionState.DECLINED, 4, answer = TripMissionResponse.DECLINE, reason = "Folga",
            starts = "2026-11-22T11:00:00Z"), f.item(73, TripMissionState.IN_PROGRESS, 5, answer = TripMissionResponse.ACCEPT,
            starts = "2026-11-19T11:00:00Z"), f.item(74, TripMissionState.CANCELLED, 6, starts = "2026-11-23T11:00:00Z")), f.clock(0))
        val view = MissionInboxPresentation.view(inbox, emptyMap(), TripBook.empty(f.member, f.enrollment), f.clock(10))
        // Window order.
        assertEquals(listOf(f.id(73), f.id(70), f.id(71), f.id(72), f.id(74)), view.missions.map { it.missionId })
        val byId = view.missions.associateBy { it.missionId }
        assertEquals(setOf(MissionAction.ACCEPT, MissionAction.DECLINE), byId.getValue(f.id(70)).actions)
        assertEquals(setOf(MissionAction.START_TRIP), byId.getValue(f.id(71)).actions)
        assertTrue(byId.getValue(f.id(72)).actions.isEmpty())
        assertTrue(byId.getValue(f.id(73)).actions.isEmpty())
        assertTrue(byId.getValue(f.id(74)).actions.isEmpty())
        assertEquals(MissionAnswerState.NONE, byId.getValue(f.id(70)).answer)
    }

    @Test fun `B-BR-EMP-531 a saved answer is shown saved until its own receipt and an unconfirmed acceptance blocks the planned start`() {
        val accept = f.respond(600)
        val inbox = empty().withPage(f.employee, f.page(f.item()), f.clock(0)).withResponse(f.line(accept))
        val saved = MissionInboxPresentation.view(inbox, t.saved(accept), TripBook.empty(f.member, f.enrollment), f.clock(10)).missions.single()
        assertEquals(MissionAnswerState.SAVED, saved.answer)
        assertEquals(TripMissionState.ASSIGNED, saved.state)
        assertTrue(saved.actions.isEmpty())
        assertEquals(MissionLocalFailure.NOT_CONFIRMED, saved.startBlocked)
        // Its ACCEPTED receipt at the next revision proves the acceptance without a refresh; the start is offered.
        val receipt = mapOf(accept.operationId to EnterpriseQueueEntry(accept, acknowledgement = t.ack(accept, revision = 4)))
        val confirmed = MissionInboxPresentation.view(inbox, receipt, TripBook.empty(f.member, f.enrollment), f.clock(10)).missions.single()
        assertEquals(MissionAnswerState.ACCEPTED, confirmed.answer)
        assertEquals(TripMissionState.ACCEPTED, confirmed.state)
        assertEquals(setOf(MissionAction.START_TRIP), confirmed.actions)
        assertNull(confirmed.startBlocked)
        // A decline's receipt is "recorded", never accepted, and offers nothing.
        val decline = f.respond(601, response = "DECLINE", reason = "Folga")
        val declinedInbox = empty().withPage(f.employee, f.page(f.item()), f.clock(0)).withResponse(f.line(decline))
        val declined = MissionInboxPresentation.view(declinedInbox, mapOf(decline.operationId to EnterpriseQueueEntry(decline,
            acknowledgement = t.ack(decline, revision = 4))), null, f.clock(10)).missions.single()
        assertEquals(MissionAnswerState.DECLINE_RECORDED, declined.answer); assertEquals(TripMissionState.DECLINED, declined.state)
        assertTrue(declined.actions.isEmpty())
        // A missing entry is refused, never guessed.
        assertThrows(IllegalArgumentException::class.java) { MissionInboxPresentation.view(inbox, emptyMap(), null, f.clock(10)) }
    }

    @Test fun `an answer the company did not confirm is explained after a refresh`() {
        val accept = f.respond(600)
        val answered = empty().withPage(f.employee, f.page(f.item(), f.item(71, starts = "2026-11-21T11:00:00Z")), f.clock(0))
            .withResponse(f.line(accept))
        // The planner re-planned and assigned again (revision 5): the saved answer is not confirmed and the new revision is answerable.
        val changed = answered.withPage(f.employee, f.page(f.item(revision = 5), f.item(71, starts = "2026-11-21T11:00:00Z")), f.clock(20))
        val view = MissionInboxPresentation.view(changed, t.saved(accept), null, f.clock(30)).missions.first { it.missionId == f.id(70) }
        assertEquals(MissionAnswerState.NOT_CONFIRMED_CHANGED, view.answer)
        assertEquals(setOf(MissionAction.ACCEPT, MissionAction.DECLINE), view.actions)
        // Reassigned to another member: the mission leaves the list and its unconfirmed answer stays visible with no action.
        val removed = answered.withPage(f.employee, f.page(f.item(71, starts = "2026-11-21T11:00:00Z")), f.clock(20))
        val gone = MissionInboxPresentation.view(removed, t.saved(accept), null, f.clock(30)).missions
        assertEquals(listOf(f.id(71), f.id(70)), gone.map { it.missionId })
        assertEquals(MissionAnswerState.NOT_CONFIRMED_REMOVED, gone.last().answer)
        assertNull(gone.last().item); assertNull(gone.last().state); assertTrue(gone.last().actions.isEmpty())
        // Once its ACCEPTED receipt exists, an answer for a mission that left the list is not repeated.
        val accepted = mapOf(accept.operationId to EnterpriseQueueEntry(accept, acknowledgement = t.ack(accept, revision = 4)))
        assertEquals(listOf(f.id(71)), MissionInboxPresentation.view(removed, accepted, null, f.clock(30)).missions.map { it.missionId })
    }

    @Test fun `a planned trip on this device is opened, never started twice, and a refused start offers it again`() {
        val inbox = empty().withPage(f.employee, f.page(f.item(71, TripMissionState.ACCEPTED, 4, answer = TripMissionResponse.ACCEPT)), f.clock(0))
        val trip = LocalTrip(f.id(10), f.id(20), TripAssetLabel("Caminhão sintético", "FROTA-12", "ABC1D23"), false, f.id(100),
            f.at("2026-11-15T12:00:00Z"), missionId = f.id(71))
        val book = TripBook.empty(f.member, f.enrollment).withStart(trip)
        val started = MissionInboxPresentation.view(inbox, emptyMap(), book, f.clock(10)).missions.single()
        assertEquals(setOf(MissionAction.OPEN_TRIP), started.actions)
        assertEquals(MissionLocalFailure.ALREADY_STARTED, started.startBlocked)
        assertEquals(f.id(10), started.tripId)
        val again = MissionInboxPresentation.view(inbox, emptyMap(), book.withStartRefused(f.id(10), f.id(100)), f.clock(10)).missions.single()
        assertEquals(setOf(MissionAction.START_TRIP), again.actions)
        assertNull(again.tripId)
    }
}
