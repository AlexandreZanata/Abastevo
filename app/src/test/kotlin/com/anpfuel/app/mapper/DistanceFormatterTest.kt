package com.anpfuel.app.mapper

import java.util.Locale
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class DistanceFormatterTest {

    private val ptBr = Locale.forLanguageTag("pt-BR")
    private val en = Locale.ENGLISH

    @Test
    fun formatsSubKilometerDistancesInMeters() {
        assertEquals("320 m", DistanceFormatter.format(320.4, en))
        assertEquals("999 m", DistanceFormatter.format(999.0, en))
    }

    @Test
    fun formatsKilometerDistancesWithOneDecimal() {
        assertEquals("1.2 km", DistanceFormatter.format(1_240.0, en))
        assertEquals("8.0 km", DistanceFormatter.format(8_020.0, en))
    }

    @Test
    fun usesLocaleDecimalSeparator() {
        assertEquals("1,2 km", DistanceFormatter.format(1_240.0, ptBr))
    }
}
