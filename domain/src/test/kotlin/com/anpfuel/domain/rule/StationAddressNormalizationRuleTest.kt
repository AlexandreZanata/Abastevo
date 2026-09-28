package com.anpfuel.domain.rule

import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class StationAddressNormalizationRuleTest {

    private val campoGrandeStation = RetailStation.create(
        cnpj = Cnpj.parse("61602199002409"),
        legalName = "COMPANHIA ULTRAGAZ S A",
        tradeName = "ULTRAGAZ",
        address = "RUA AMARO CASTRO LIMA",
        municipality = "CAMPO GRANDE",
        state = BrazilianState.MATO_GROSSO_DO_SUL,
        brand = "ULTRAGAZ",
    )

    @Test
    fun normalizesWhitespaceAndControlCharacters() {
        val normalized = StationAddressNormalizationRule.normalizeAddress(
            "  Rua\u0007Example   100  \n",
        )

        assertEquals("RuaExample 100", normalized)
    }

    @Test
    fun buildsQueryFromAnpAddressWithMunicipalityAndState() {
        val query = StationAddressNormalizationRule.buildNavigationQuery(
            station = campoGrandeStation,
        )

        assertEquals(
            "ULTRAGAZ, RUA AMARO CASTRO LIMA, CAMPO GRANDE - MS, Brazil",
            query,
        )
    }

    @Test
    fun prefersStationMunicipalityOverPreferredLocation() {
        val query = StationAddressNormalizationRule.buildNavigationQuery(
            station = campoGrandeStation,
            preferredMunicipality = "CURITIBA",
            preferredState = BrazilianState.PARANA,
        )

        assertTrue(query.contains("CAMPO GRANDE - MS"))
        assertFalse(query.contains("CURITIBA"))
    }

    @Test
    fun appendsStateWhenAddressAlreadyContainsMunicipality() {
        val station = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = "Example SA",
            tradeName = "Posto Centro",
            address = "Av. Brasil, 1000, Curitiba",
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = "BR",
        )

        val query = StationAddressNormalizationRule.buildNavigationQuery(station = station)

        assertEquals("Posto Centro, Av. Brasil, 1000, Curitiba, PR, Brazil", query)
    }

    @Test
    fun fallsBackToDisplayNameWhenAddressIsInsufficient() {
        val station = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = "Example SA",
            tradeName = "Posto Centro",
            address = "N/A",
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = "BR",
        )

        val query = StationAddressNormalizationRule.buildNavigationQuery(station = station)

        assertEquals("Posto Centro, CURITIBA - PR, Brazil", query)
    }

    @Test
    fun doesNotDuplicateStationNameWhenAlreadyInAddress() {
        val station = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = "POSTO BR CENTRO LTDA",
            tradeName = "Posto BR Centro",
            address = "Posto BR Centro, Rua XV, 100",
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = "BR",
        )

        val query = StationAddressNormalizationRule.buildNavigationQuery(station = station)

        assertEquals("Posto BR Centro, Rua XV, 100, CURITIBA - PR, Brazil", query)
    }

    @Test
    fun keepsExistingBrazilSuffix() {
        val station = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = null,
            tradeName = "Posto Sul",
            address = "Rua A, Porto Alegre - RS, Brazil",
            municipality = "PORTO ALEGRE",
            state = BrazilianState.RIO_GRANDE_DO_SUL,
            brand = null,
        )

        val query = StationAddressNormalizationRule.buildNavigationQuery(station = station)

        assertEquals("Posto Sul, Rua A, Porto Alegre - RS, Brazil", query)
    }

    @Test
    fun buildsGeocodingQueryWithoutStationNameAndWithFullStateName() {
        val query = StationAddressNormalizationRule.buildGeocodingQuery(
            station = campoGrandeStation,
        )

        assertEquals(
            "RUA AMARO CASTRO LIMA, CAMPO GRANDE, Mato Grosso do Sul, Brazil",
            query,
        )
    }

    @Test
    fun geocodingQueryUsesFullStateNameWhenAddressContainsMunicipality() {
        val station = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = "Example SA",
            tradeName = "Posto Centro",
            address = "Av. Brasil, 1000, Curitiba",
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = "BR",
        )

        val query = StationAddressNormalizationRule.buildGeocodingQuery(station = station)

        assertEquals("Av. Brasil, 1000, Curitiba, Paraná, Brazil", query)
    }

    @Test
    fun geocodingQueryFallsBackToMunicipalityAndFullStateWhenAddressIsInsufficient() {
        val station = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = "Example SA",
            tradeName = "Posto Centro",
            address = "N/A",
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = "BR",
        )

        val query = StationAddressNormalizationRule.buildGeocodingQuery(station = station)

        assertEquals("CURITIBA, Paraná, Brazil", query)
    }
}
