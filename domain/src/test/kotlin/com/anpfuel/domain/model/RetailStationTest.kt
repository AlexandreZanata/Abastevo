package com.anpfuel.domain.model

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class RetailStationTest {

    @Test
    fun displayNamePrefersTradeNameOverLegalName() {
        val station = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = "COMPANHIA EXEMPLO SA",
            tradeName = "POSTO EXEMPLO",
            address = "RUA A",
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = "IPIRANGA",
        )

        assertEquals("POSTO EXEMPLO", station.displayName())
    }

    @Test
    fun matchesLocationIsCaseInsensitiveForMunicipality() {
        val station = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = null,
            tradeName = null,
            address = "RUA A",
            municipality = "Curitiba",
            state = BrazilianState.PARANA,
            brand = null,
        )

        assertTrue(station.matchesLocation(BrazilianState.PARANA, "CURITIBA"))
        assertFalse(station.matchesLocation(BrazilianState.SAO_PAULO, "CURITIBA"))
    }
}
