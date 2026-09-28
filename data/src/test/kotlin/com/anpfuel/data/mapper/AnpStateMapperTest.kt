package com.anpfuel.data.mapper

import com.anpfuel.domain.valueobject.BrazilianState
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class AnpStateMapperTest {

    @Test
    fun mapsAllBrazilianStateAbbreviations() {
        BrazilianState.entries.forEach { state ->
            assertEquals(state, AnpStateMapper.toBrazilianState(state.abbreviation))
        }
    }

    @Test
    fun mapsAllAnpPortugueseStateLabels() {
        AnpStateMapper.supportedPortugueseLabels().forEach { label ->
            val state = requireNotNull(AnpStateMapper.toBrazilianState(label)) {
                "Missing mapping for ANP label: $label"
            }
            assertEquals(state.abbreviation, AnpStateMapper.toAbbreviation(label))
        }
    }

    @Test
    fun mapsDistritoFederalLabelUsedByAnpSpreadsheets() {
        assertEquals(
            BrazilianState.DISTRICT_FEDERAL,
            AnpStateMapper.toBrazilianState("DISTRITO FEDERAL"),
        )
        assertEquals("DF", AnpStateMapper.toAbbreviation("Distrito Federal"))
    }
}
