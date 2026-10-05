package com.anpfuel.domain.contribution

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class ContributionTargetTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    @Test
    fun `valid target keeps uuid and wire fuel`() {
        val target = ContributionTarget.create(
            stationId = stationId,
            fuelProductWire = "GASOLINE_REGULAR",
        )

        assertEquals(stationId, target.stationId)
        assertEquals("GASOLINE_REGULAR", target.fuelProductWire)
    }

    @Test
    fun `legacy cnpj and unknown wire are rejected`() {
        assertThrows(DomainException::class.java) {
            ContributionTarget.create(
                stationId = "04218406000104",
                fuelProductWire = "GASOLINE_REGULAR",
            )
        }
        assertThrows(DomainException::class.java) {
            ContributionTarget.create(
                stationId = stationId,
                fuelProductWire = "GASOLINE",
            )
        }
        assertThrows(DomainException::class.java) {
            ContributionTarget.create(
                stationId = "   ",
                fuelProductWire = "ETHANOL",
            )
        }
    }
}
