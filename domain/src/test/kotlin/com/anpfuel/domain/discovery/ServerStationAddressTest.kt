package com.anpfuel.domain.discovery

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ServerStationAddressTest {
    private fun station(
        address: Map<String, String> = emptyMap(),
        legal: String? = null,
        source: StationAddressSource? = null,
    ) = ServerStation.create(
        stationId = "123e4567-e89b-12d3-a456-426614174000",
        displayName = "[P25-TEST] Posto Exemplo",
        locationQuality = StationLocationQuality.REVIEWED,
        latitude = -12.12345,
        longitude = -51.54321,
        cnpjNormalized = null,
        municipalityCode = "5103403",
        state = "MT",
        currentRevisionId = null,
        address = address,
        legalName = legal,
        addressSource = source,
    )

    @Test fun `formats street number district and city without emitting raw data or codes`() {
        val s = station(mapOf(
            "raw" to "[P25-TEST] Duplicated address",
            "street" to "Av. [P25-TEST] Exemplo", "number" to "123",
            "district" to "Bairro Teste", "municipio" to "Cidade Teste",
            "municipio_ibge" to "5103403", "uf" to "MT", "latitude" to "-12.12345",
        ), source = StationAddressSource.ANP)
        assertEquals("Av. [P25-TEST] Exemplo, 123 – Bairro Teste, Cidade Teste/MT", s.addressLine)
        assertEquals(s.addressLine, s.addressLabel)
        assertFalse(s.hasApproximateAddress)
    }

    @Test fun `uses legacy text aliases or named locality and never coordinates as an address`() {
        val alias = station(mapOf("logradouro" to "[P25-TEST] Rua A", "numero" to "S/N", "municipio" to "Cidade Teste"))
        assertEquals("[P25-TEST] Rua A, S/N – Cidade Teste/MT", alias.addressLine)
        assertEquals("[P25-TEST] Rua A, 9", station(mapOf("raw" to "[P25-TEST] Rua A, 9")).addressLine)
        val city = station(mapOf("raw" to "-12.12345, -51.54321", "municipio" to "Cidade Teste"), source = StationAddressSource.MUNICIPALITY)
        assertEquals("Cidade Teste/MT", city.addressLine)
        assertTrue(city.hasApproximateAddress)
        assertEquals("", station(mapOf("raw" to "lat: -12.12345, lon: -51.54321", "municipio" to "5103403")).addressLine)
        assertEquals("", station(mapOf("municipio_ibge" to "5103403", "coordinates" to "-12.12345, -51.54321")).addressLine)
    }

    @Test fun `preserves legal fact and finds both names while suppressing redundant subtitle`() {
        val same = station(legal = " [p25-test]   POSTO\u00a0Exemplo ")
        assertNull(same.secondaryLegalName)
        assertEquals(" [p25-test]   POSTO\u00a0Exemplo ", same.legalName)
        val distinct = station(legal = " [P25-TEST] Pessoa Jurídica LTDA ")
        assertEquals("[P25-TEST] Pessoa Jurídica LTDA", distinct.secondaryLegalName)
        assertTrue(distinct.matchesName("posto   exemplo"))
        assertTrue(distinct.matchesName("JURÍDICA"))
        assertFalse(distinct.matchesName(" "))
        assertFalse(distinct.matchesName("5103403"))
    }

    @Test fun `does not claim unknown source or missing address is approximate`() {
        val address = mapOf("street" to "[P25-TEST] Rua")
        assertFalse(station(address).hasApproximateAddress)
        assertFalse(station(source = StationAddressSource.RFB).hasApproximateAddress)
        for (source in listOf(StationAddressSource.RFB, StationAddressSource.OSM, StationAddressSource.CNEFE)) {
            assertTrue(station(address, source = source).hasApproximateAddress)
        }
        assertNull(StationAddressSource.fromWire("future"))
        assertNull(StationAddressSource.fromWire(null))
    }
}
