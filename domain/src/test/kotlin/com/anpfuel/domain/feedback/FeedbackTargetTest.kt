package com.anpfuel.domain.feedback

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class FeedbackTargetTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    @Test
    fun `valid signed target keeps uuid wire and account`() {
        val target = FeedbackTarget.create(
            stationId = stationId,
            fuelProductWire = "GASOLINE_REGULAR",
            accountId = "acc-123",
        )

        assertEquals(stationId, target.stationId)
        assertEquals("GASOLINE_REGULAR", target.fuelProductWire)
        assertTrue(target.isSignedIn)
    }

    @Test
    fun `blank account is an honest guest`() {
        val target = FeedbackTarget.create(
            stationId = stationId,
            fuelProductWire = "ETHANOL",
            accountId = "   ",
        )

        assertTrue(!target.isSignedIn)
    }

    @Test
    fun `legacy cnpj and unknown wire are rejected`() {
        assertThrows(DomainException::class.java) {
            FeedbackTarget.create(
                stationId = "04218406000104",
                fuelProductWire = "GASOLINE_REGULAR",
                accountId = "acc-123",
            )
        }
        assertThrows(DomainException::class.java) {
            FeedbackTarget.create(
                stationId = stationId,
                fuelProductWire = "GASOLINE",
                accountId = "acc-123",
            )
        }
        assertThrows(DomainException::class.java) {
            FeedbackTarget.create(
                stationId = "   ",
                fuelProductWire = "ETHANOL",
                accountId = "",
            )
        }
    }
}
