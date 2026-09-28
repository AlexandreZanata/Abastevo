package com.anpfuel.app.ui.location

import com.anpfuel.application.usecase.location.CatalogMunicipalityItem
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class LocationPickerFilterTest {

    @Test
    fun emptyQueryReturnsAllStates() {
        val states = listOf(BrazilianState.PARANA, BrazilianState.SAO_PAULO)

        val filtered = LocationPickerFilter.filterStates(states, "  ", stateLabel = { "Ignored" })

        assertEquals(states, filtered)
    }

    @Test
    fun filtersStatesByAbbreviationAndLocalizedName() {
        val states = BrazilianState.entries.toList()
        val stateLabel: (BrazilianState) -> String = { state ->
            when (state) {
                BrazilianState.MATO_GROSSO -> "Mato Grosso"
                BrazilianState.SAO_PAULO -> "São Paulo"
                else -> state.name.replace('_', ' ')
            }
        }

        assertEquals(
            listOf(BrazilianState.MATO_GROSSO),
            LocationPickerFilter.filterStates(states, "mt", stateLabel),
        )
        assertEquals(
            listOf(BrazilianState.SAO_PAULO),
            LocationPickerFilter.filterStates(states, "sao paul", stateLabel),
        )
    }

    @Test
    fun filtersMunicipalitiesAccentInsensitive() {
        val municipalities = listOf(
            municipality("ACORIZAL"),
            municipality("ALTA FLORESTA"),
            municipality("SÃO PAULO"),
        )

        val filtered = LocationPickerFilter.filterMunicipalities(municipalities, "sao paul")

        assertEquals(listOf(municipality("SÃO PAULO")), filtered)
    }

    @Test
    fun filtersMunicipalitiesByPartialName() {
        val municipalities = listOf(
            municipality("ALTO ARAGUAIA"),
            municipality("ALTO BOA VISTA"),
            municipality("CURITIBA"),
        )

        val filtered = LocationPickerFilter.filterMunicipalities(municipalities, "alto")

        assertEquals(2, filtered.size)
        assertTrue(filtered.all { it.municipality.startsWith("ALTO") })
    }

    private fun municipality(name: String): CatalogMunicipalityItem =
        CatalogMunicipalityItem(
            municipality = name,
            dataAvailability = DataAvailability.HAS_DATA,
        )
}
