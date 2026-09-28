package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class DomainIdTest {

    @Test
    fun generateCreatesUniqueIds() {
        val first = DomainId.generate()
        val second = DomainId.generate()

        assertNotEquals(first, second)
    }

    @Test
    fun forSurveyWeekIsDeterministicForSameWeek() {
        val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

        val first = DomainId.forSurveyWeek(week)
        val second = DomainId.forSurveyWeek(week)

        assertEquals(first, second)
    }

    @Test
    fun blankValueThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            DomainId.from("   ")
        }
    }
}
