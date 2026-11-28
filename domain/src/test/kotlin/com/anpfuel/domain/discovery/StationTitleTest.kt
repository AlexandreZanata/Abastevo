package com.anpfuel.domain.discovery

import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/** B-BR-STATION-NAME-01: the legal name shows once, as the secondary line, never again inside the title. */
class StationTitleTest {

    @Test
    fun `drops a trailing parenthesized legal name already shown below the title`() {
        assertEquals(
            "SMILE POSTO E CONVENIENCIA",
            stationTitle("SMILE POSTO E CONVENIENCIA (BRESCANSIN & BRESCANSIN LTDA)", "BRESCANSIN & BRESCANSIN LTDA"),
        )
    }

    @Test
    fun `ignores case, spacing and punctuation differences in the legal name`() {
        assertEquals(
            "Posto Paraíso",
            stationTitle("Posto Paraíso  ( Isamar Comercio de Combustiveis e Lubrificantes Ltda. )", "ISAMAR COMERCIO DE COMBUSTIVEIS E LUBRIFICANTES LTDA"),
        )
    }

    @Test
    fun `keeps a parenthesis that is not the legal name`() {
        assertEquals("POSTO CENTRAL (BR-163)", stationTitle("POSTO CENTRAL (BR-163)", "CENTRAL COMBUSTIVEIS LTDA"))
    }

    @Test
    fun `keeps the title when no legal name is known or nothing would remain`() {
        assertEquals("POSTO X (ACME LTDA)", stationTitle("POSTO X (ACME LTDA)", null))
        assertEquals("(ACME LTDA)", stationTitle("(ACME LTDA)", "ACME LTDA"))
    }

    @Test
    fun `server stations and retail stations use the clean title and keep the legal line`() {
        val server = ServerStation.create(
            stationId = "00000000-0000-4000-8000-000000000001",
            displayName = "SMILE POSTO E CONVENIENCIA (BRESCANSIN & BRESCANSIN LTDA)",
            locationQuality = StationLocationQuality.UNKNOWN,
            latitude = null,
            longitude = null,
            cnpjNormalized = null,
            municipalityCode = "5107925",
            state = "MT",
            currentRevisionId = null,
            legalName = "BRESCANSIN & BRESCANSIN LTDA",
        )
        assertEquals("SMILE POSTO E CONVENIENCIA", server.displayName)
        assertEquals("BRESCANSIN & BRESCANSIN LTDA", server.secondaryLegalName)
        assertEquals(true, server.matchesName("brescansin"))

        val retail = RetailStation.create(
            cnpj = Cnpj.parse("11222333000181"),
            legalName = "BRESCANSIN & BRESCANSIN LTDA",
            tradeName = "SMILE POSTO E CONVENIENCIA (BRESCANSIN & BRESCANSIN LTDA)",
            address = "AVENIDA TANCREDO NEVES, 2219",
            municipality = "SORRISO",
            state = BrazilianState.MATO_GROSSO,
            brand = "IPIRANGA",
        )
        assertEquals("SMILE POSTO E CONVENIENCIA", retail.displayName())
        assertEquals("BRESCANSIN & BRESCANSIN LTDA", retail.secondaryLegalName)
    }
}
