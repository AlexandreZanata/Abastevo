package com.anpfuel.domain.discovery

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class StationAddressFormatterTest {

    @Test
    fun `street number and district read like an address, without codes or duplicates`() {
        // Shape of a registry row on staging (JSONB keys, uppercase source text).
        val address = mapOf(
            "district" to "CENTRO",
            "municipio_ibge" to "5107925",
            "number" to "2049",
            "postal_code" to "78890000",
            "raw" to "AVENIDA NATALINO JOAO BRESCANSIN, 2049",
            "street" to "AVENIDA NATALINO JOAO BRESCANSIN",
            "complement" to "",
            "uf" to "MT",
        )

        assertEquals("Avenida Natalino Joao Brescansin, 2049 · Centro", StationAddressFormatter.format(address))
    }

    @Test
    fun `connectors stay lowercase and roman numerals stay upper`() {
        val address = mapOf("street" to "RUA DAS FLORES DE SAO JOAO II", "district" to "JARDIM DO LAGO")

        assertEquals("Rua das Flores de Sao Joao II · Jardim do Lago", StationAddressFormatter.format(address))
    }

    @Test
    fun `raw address is the fallback street and is not repeated with the number`() {
        val address = mapOf("raw" to "AV PERIMETRAL SUDESTE, 10493", "number" to "10493", "uf" to "MT")

        assertEquals("Av Perimetral Sudeste, 10493", StationAddressFormatter.format(address))
    }

    @Test
    fun `portuguese source keys are understood`() {
        val address = mapOf("logradouro" to "AV PAULISTA", "numero" to "1000", "bairro" to "BELA VISTA", "municipio" to "SAO PAULO")

        assertEquals("Av Paulista, 1000 · Bela Vista", StationAddressFormatter.format(address))
    }

    @Test
    fun `only a city name falls back to city and state, never to codes`() {
        assertEquals("Sorriso · MT", StationAddressFormatter.format(mapOf("municipio" to "SORRISO", "uf" to "MT", "municipio_ibge" to "5107925")))
        assertEquals("", StationAddressFormatter.format(mapOf("municipio_ibge" to "5107925", "postal_code" to "78890000")))
    }

    @Test
    fun `source punctuation, hyphenated names and acronyms read cleanly`() {
        val address = mapOf("street" to "AVENIDA ZILDA ARNS,", "number" to "3422", "district" to "LOTEAMENTO JUSCELINO KUBITSCHEK JK")
        assertEquals("Avenida Zilda Arns, 3422 · Loteamento Juscelino Kubitschek JK", StationAddressFormatter.format(address))

        assertEquals("Rua Panambi, 350 · Centro-Sul", StationAddressFormatter.format(mapOf("street" to "RUA PANAMBI ,", "number" to "350", "district" to "CENTRO-SUL")))
        assertEquals("Rodovia BR 163", StationAddressFormatter.format(mapOf("street" to "RODOVIA BR 163")))
    }
    @Test
    fun `coordinate and code literals are never rendered as addresses`() {
        for (raw in listOf("-12.12345, -51.54321", "lat: -12.12345, lon: -51.54321", "geo:-12.1,-51.2", "5103403/MT")) {
            assertEquals("", StationAddressFormatter.format(mapOf("raw" to raw)))
        }
        assertEquals("Cidade Teste · MT", StationAddressFormatter.format(
            mapOf("raw" to "-12.12345, -51.54321", "municipio" to "CIDADE TESTE", "uf" to "MT"),
        ))
        assertEquals("Rua Teste", StationAddressFormatter.format(
            mapOf("street" to "RUA TESTE", "raw" to "-12.12345, -51.54321"),
        ))
    }
}
