package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class ContributionDraftTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    private fun valid() = ContributionDraft.create(
        clientSubmissionId = "cmd-1",
        stationId = stationId,
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        amountMilliBrl = 5890L,
        currency = "BRL",
        unit = "BRL/L",
        conditionKind = "STANDARD",
        capturedAtMillis = 1_000_000L,
    )

    @Test
    fun `valid draft keeps exact milli and lowercases station`() {
        val draft = valid()

        assertEquals(5890L, draft.amountMilliBrl)
        assertEquals(stationId, draft.stationId)
    }

    @Test
    fun `blank submission id fails`() {
        assertThrows(DomainException::class.java) {
            ContributionDraft.create(
                clientSubmissionId = "  ",
                stationId = stationId,
                fuelProduct = FuelProduct.ETHANOL,
                amountMilliBrl = 5890L,
                currency = "BRL",
                unit = "BRL/L",
                conditionKind = "STANDARD",
                capturedAtMillis = 1_000_000L,
            )
        }
    }

    @Test
    fun `malformed station and milli out of range fail`() {
        assertThrows(DomainException::class.java) {
            ContributionDraft.create(
                clientSubmissionId = "cmd-1",
                stationId = "not-a-uuid",
                fuelProduct = FuelProduct.ETHANOL,
                amountMilliBrl = 5890L,
                currency = "BRL",
                unit = "BRL/L",
                conditionKind = "STANDARD",
                capturedAtMillis = 1_000_000L,
            )
        }
        assertThrows(DomainException::class.java) {
            ContributionDraft.create(
                clientSubmissionId = "cmd-1",
                stationId = stationId,
                fuelProduct = FuelProduct.ETHANOL,
                amountMilliBrl = 0L,
                currency = "BRL",
                unit = "BRL/L",
                conditionKind = "STANDARD",
                capturedAtMillis = 1_000_000L,
            )
        }
    }

    @Test
    fun `non-BRL currency and blank condition fail`() {
        assertThrows(DomainException::class.java) {
            ContributionDraft.create(
                clientSubmissionId = "cmd-1",
                stationId = stationId,
                fuelProduct = FuelProduct.ETHANOL,
                amountMilliBrl = 5890L,
                currency = "USD",
                unit = "BRL/L",
                conditionKind = "STANDARD",
                capturedAtMillis = 1_000_000L,
            )
        }
        assertThrows(DomainException::class.java) {
            ContributionDraft.create(
                clientSubmissionId = "cmd-1",
                stationId = stationId,
                fuelProduct = FuelProduct.ETHANOL,
                amountMilliBrl = 5890L,
                currency = "BRL",
                unit = "BRL/L",
                conditionKind = "  ",
                capturedAtMillis = 1_000_000L,
            )
        }
    }
}
