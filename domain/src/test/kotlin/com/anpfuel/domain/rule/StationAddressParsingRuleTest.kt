package com.anpfuel.domain.rule

import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class StationAddressParsingRuleTest {

    private fun stationWith(address: String) = RetailStation.create(
        cnpj = Cnpj.parse("12345678000195"),
        legalName = "POSTO TESTE LTDA",
        tradeName = "POSTO TESTE",
        address = address,
        municipality = "CURITIBA",
        state = BrazilianState.PARANA,
        brand = "BR",
    )

    @Test
    fun parsesStandardStreetNumberAndNeighborhood() {
        val station = stationWith("AV. BRASIL, 1000, CENTRO")
        val parsed = StationAddressParsingRule.parse(station)

        assertEquals("AVENIDA BRASIL", parsed.street)
        assertEquals("1000", parsed.number)
        assertEquals("CENTRO", parsed.neighborhood)
        assertFalse(StationAddressParsingRule.isHighwayAddress(parsed))
    }

    @Test
    fun parsesAddressWithSnNumberToken() {
        val station = stationWith("RUA DAS FLORES, S/N, JARDIM BOTANICO")
        val parsed = StationAddressParsingRule.parse(station)

        assertEquals("RUA DAS FLORES", parsed.street)
        assertEquals("S/N", parsed.number)
        assertEquals("JARDIM BOTANICO", parsed.neighborhood)
    }

    @Test
    fun parsesAddressWithoutNeighborhood() {
        val station = stationWith("R. MARECHAL DEODORO, 500")
        val parsed = StationAddressParsingRule.parse(station)

        assertEquals("RUA MARECHAL DEODORO", parsed.street)
        assertEquals("500", parsed.number)
        assertNull(parsed.neighborhood)
    }

    @Test
    fun parsesAddressWithoutNumber() {
        val station = stationWith("ROD. BR-116, KM 100")
        val parsed = StationAddressParsingRule.parse(station)

        assertEquals("RODOVIA BR-116", parsed.street)
        assertNull(parsed.number)
        assertEquals("KM 100", parsed.neighborhood)
        assertTrue(StationAddressParsingRule.isHighwayAddress(parsed))
    }

    @Test
    fun expandsCommonStreetTypeAbbreviations() {
        assertEquals("AVENIDA PAULISTA", StationAddressParsingRule.expandStreetType("AV. PAULISTA"))
        assertEquals("RUA SETE DE SETEMBRO", StationAddressParsingRule.expandStreetType("R. SETE DE SETEMBRO"))
        assertEquals("ESTRADA DOS BANDEIRANTES", StationAddressParsingRule.expandStreetType("EST. DOS BANDEIRANTES"))
        assertEquals("ALAMEDA SANTOS", StationAddressParsingRule.expandStreetType("AL. SANTOS"))
        assertEquals("PRACA DA SE", StationAddressParsingRule.expandStreetType("PC. DA SE"))
    }
}
