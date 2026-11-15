package com.anpfuel.domain.enterprise.trip

import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

/** EDR-228: mission values never print their content; commands return new values. */
class EnterpriseTripMissionTest {
    private val org = "5e000000-0000-4000-8000-000000000001"
    private fun id(n: Int) = "5e000000-0000-4000-8000-" + n.toString().padStart(12, '0')

    @Test fun valuesAreRedactedAndCommandsDoNotMutate() {
        val start = Instant.parse("2026-11-20T11:00:00Z")
        val mission = TripMission.create(org, TripMissionInput(MissionRef(org, id(91)), UnitRef(org, id(95)), null, "Segredo sintético",
            TripMissionWindow(start, start.plusSeconds(3600))))
        val member = MembershipRef(org, id(41))
        val assigned = mission.assign(org, 1, TripMissionAssignee(EmployeeRef(org, id(31)), member))
        val declined = assigned.respond(org, 2, member, "DECLINE", "Motivo privado")
        assertEquals(TripMissionState.DRAFT, mission.state)
        assertEquals(TripMissionState.ASSIGNED, assigned.state)
        assertEquals(TripMissionState.DECLINED, declined.state)
        assertEquals(3L, declined.revision)
        for (value in listOf<Any>(declined, declined.answer!!, declined.assignee!!, member)) {
            val printed = value.toString()
            assertFalse(printed.contains("Segredo") || printed.contains("Motivo") || printed.contains(id(41)), printed)
        }
        val failure = assertThrows(TripMissionException::class.java) { declined.respond(org, 3, member, "ACCEPT", null) }
        assertEquals(TripMissionFailure.INVALID_TRANSITION, failure.failure)
    }
}
