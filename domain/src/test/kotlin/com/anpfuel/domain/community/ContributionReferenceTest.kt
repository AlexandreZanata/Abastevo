package com.anpfuel.domain.community

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ContributionReferenceTest {
    private val station="123e4567-e89b-12d3-a456-426614174000"
    @Test fun `public action target requires canonical identity and a positive revision`() {
        assertEquals(2L,ContributionReference(station,2).decisionSequence)
        assertThrows(IllegalArgumentException::class.java){ContributionReference("station-name",2)}
        assertThrows(IllegalArgumentException::class.java){ContributionReference(station,0)}
        assertThrows(IllegalArgumentException::class.java){ContributionReference(station,-1)}
    }
    @Test fun `nominal station address does not require coordinates or retain a mutable source map`() {
        val address=mutableMapOf("street" to "Synthetic Street","number" to "1")
        val value=com.anpfuel.domain.discovery.ServerStation.create(station,"Synthetic station",com.anpfuel.domain.discovery.StationLocationQuality.UNKNOWN,null,null,null,"3550308","SP",null,address)
        address["street"]="Changed input"
        assertNull(value.latitude);assertNull(value.longitude)
        assertEquals("Synthetic Street",value.address["street"])
        assertTrue(value.addressLabel.contains("Synthetic Street"))
    }
}
