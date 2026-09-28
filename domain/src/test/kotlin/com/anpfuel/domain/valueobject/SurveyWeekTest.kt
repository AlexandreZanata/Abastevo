package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test
import java.time.LocalDate

class SurveyWeekTest {

    @Test
    fun validWeekWithinSevenDaysPasses() {
        val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

        assertEquals(LocalDate.parse("2026-06-07"), week.startDate)
        assertEquals(LocalDate.parse("2026-06-13"), week.endDate)
        assertEquals(7, week.inclusiveDayCount)
    }

    @Test
    fun singleDayWeekPasses() {
        val week = SurveyWeek(LocalDate.parse("2026-06-07"), LocalDate.parse("2026-06-07"))

        assertEquals(1, week.inclusiveDayCount)
    }

    @Test
    fun startAfterEndThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            SurveyWeek(
                startDate = LocalDate.parse("2026-06-13"),
                endDate = LocalDate.parse("2026-06-07"),
            )
        }
    }

    @Test
    fun rangeGreaterThanSevenDaysThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            SurveyWeek(
                startDate = LocalDate.parse("2026-06-01"),
                endDate = LocalDate.parse("2026-06-09"),
            )
        }
    }
}
