package com.anpfuel.data.parser

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import java.time.LocalDate

class AnpDateMapperTest {

    @Test
    fun mapsExcelSerialDateFromAnpSample() {
        val date = AnpDateMapper.toLocalDate(46180)

        assertEquals(LocalDate.parse("2026-06-07"), date)
    }

    @Test
    fun mapsSecondSampleSerialDate() {
        val date = AnpDateMapper.toLocalDate(46186)

        assertEquals(LocalDate.parse("2026-06-13"), date)
    }
}
