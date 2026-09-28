package com.anpfuel.app.ui.location
import com.anpfuel.application.usecase.location.CatalogMunicipalityItem
import com.anpfuel.domain.valueobject.DataAvailability
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
class MunicipalitySectionGrouperTest {
    @Test fun groupsByLetter() {
        val s = groupMunicipalitiesBySectionLetter(listOf(
            CatalogMunicipalityItem("ÁGUAS", DataAvailability.HAS_DATA),
            CatalogMunicipalityItem("CURITIBA", DataAvailability.HAS_DATA)))
        assertEquals(listOf('A','C'), s.map { it.letter })
    }
}
