package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** EMP-ROUTE-004 (B-BR-EMP-532/533): only the approved text destination of the member's own mission or trip is handed over. */
class EnterpriseTripNavigationTest {
    private val f = MissionInboxFixture
    private val t = TripLocalFixture
    private fun inbox(vararg items: MissionInboxItem) =
        MissionInboxPresentation.view(MissionInbox.empty(f.member, f.enrollment).withPage(f.employee, f.page(*items), f.clock(0)), emptyMap(), null, f.clock(0))
    private fun unsupported(target: TripNavigationTarget) = (target as TripNavigationTarget.Unsupported).reason
    private fun trip(mission: Int?, trip: Int = 10, start: Int = 100): TripView {
        val operation = TripOperations.start(t.envelope(start, "2026-11-15T12:00:00Z"), t.id(trip), t.id(20), t.id(30), t.reading(start + 1000, "184320000"),
            null, TripClockReading("boot-3", 1_000), missionId = mission?.let(t::id))
        val local = LocalTrip(t.id(trip), t.id(20), TripAssetLabel("Caminhão sintético", "FROTA-12", "ABC1D23"), false, t.id(start),
            t.at("2026-11-15T12:00:00Z"), missionId = mission?.let(t::id))
        return TripPresentation.view(local, mapOf(operation.operationId to EnterpriseQueueEntry(operation)))
    }

    @Test fun `B-BR-EMP-532 a mission hands over exactly its plan's text destination`() {
        val view = inbox(f.item(70, plan = f.plan(text = "Cliente Norte, doca 3")))
        assertEquals("Cliente Norte, doca 3", (TripNavigation.forMission(view, f.id(70)) as TripNavigationTarget.Search).query)
        // A text that looks like coordinates is still only a search query, never parsed into a point.
        val typed = inbox(f.item(70, plan = f.plan(text = "-15.601,-56.097")))
        assertEquals("-15.601,-56.097", (TripNavigation.forMission(typed, f.id(70)) as TripNavigationTarget.Search).query)
    }

    @Test fun `B-BR-EMP-533 unit destinations, missing plans and foreign or missing missions open nothing`() {
        val view = inbox(f.item(70, plan = f.plan(text = null, unit = f.id(82))), f.item(71, plan = null))
        assertEquals(TripNavigationUnsupported.UNIT_DESTINATION, unsupported(TripNavigation.forMission(view, f.id(70))))
        assertEquals(TripNavigationUnsupported.NO_PLAN, unsupported(TripNavigation.forMission(view, f.id(71))))
        assertEquals(TripNavigationUnsupported.NOT_OWN, unsupported(TripNavigation.forMission(view, f.id(79))))
        assertEquals(TripNavigationUnsupported.NOT_OWN, unsupported(TripNavigation.forMission(null, f.id(70))))
    }

    @Test fun `a trip navigates to its own mission's destination and never to another trip's`() {
        val view = inbox(f.item(70, plan = f.plan(text = "Cliente Norte, doca 3")), f.item(71, plan = f.plan(text = "Cliente Sul")))
        val trips = listOf(trip(70), trip(null, trip = 11, start = 101), trip(72, trip = 12, start = 102))
        assertEquals("Cliente Norte, doca 3", (TripNavigation.forTrip(trips, view, t.id(10)) as TripNavigationTarget.Search).query)
        assertEquals(TripNavigationUnsupported.NO_MISSION, unsupported(TripNavigation.forTrip(trips, view, t.id(11))))
        assertEquals(TripNavigationUnsupported.MISSION_NOT_IN_COPY, unsupported(TripNavigation.forTrip(trips, view, t.id(12))))
        // A trip identifier that is not in the visible partition's book (wrong trip) opens nothing.
        assertEquals(TripNavigationUnsupported.NOT_OWN, unsupported(TripNavigation.forTrip(trips, view, t.id(19))))
        assertTrue(TripNavigation.forTrip(trips, view, t.id(10)).toString().contains("redacted"))
    }
}
