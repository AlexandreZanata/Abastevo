package com.anpfuel.domain.enterprise.attachment

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class ReceiptMoneyReadingTest {
    @Test fun readsOnlyExplicitAmountsAndKeepsTheirOwnConfidence() {
        val result = ReceiptMoneyReading.read(sequenceOf(
            ReceiptTextLine("Subtotal R$ 1.281,56", 0.98f),
            ReceiptTextLine("TOTAL A PAGAR: R$ 1.234,56", 0.93f),
            ReceiptTextLine("Desconto: 47,00", 0.71f),
            ReceiptTextLine("Troco R$ 4,00", 0.99f),
        ))
        assertEquals("1234,56", result.paid?.decimal)
        assertEquals(93, result.paid?.confidencePercent)
        assertEquals("47,00", result.discount?.decimal)
        assertEquals(71, result.discount?.confidencePercent)
    }

    @Test fun fallsBackToManualForContradictoryUnreadableOrInvalidValues() {
        val result = ReceiptMoneyReading.read(sequenceOf(
            ReceiptTextLine("Total 75,00", 0.30f), ReceiptTextLine("Valor total 15,00", 0.96f),
            ReceiptTextLine("Desconto 999.999.999.999,99", 0.99f),
        ))
        assertNull(result.paid)
        assertNull(result.discount)
        assertNull(ReceiptMoneyReading.read(sequenceOf(ReceiptTextLine("129.9Q", 0.98f))).paid)
        assertNull(ReceiptMoneyReading.read(sequenceOf(ReceiptTextLine("TOTAL 250.00", 0.98f))).paid)
    }

    @Test fun suggestsTheAmountToPayInsteadOfTheGrossTotalOfADiscountedReceipt() {
        val result = ReceiptMoneyReading.read(sequenceOf(
            ReceiptTextLine("Valor total R$ 250,00", 0.97f),
            ReceiptTextLine("Descontos R$ 10,00", 0.95f),
            ReceiptTextLine("Valor a pagar R$ 240,00", 0.91f),
        ))
        assertEquals("240,00", result.paid?.decimal)
        assertEquals(91, result.paid?.confidencePercent)
        assertEquals("10,00", result.discount?.decimal)
    }

    @Test fun leavesThePaidAmountManualWhenTheGrossTotalCannotBeReconciled() {
        val grossWithDiscount = ReceiptMoneyReading.read(sequenceOf(
            ReceiptTextLine("Valor total R$ 250,00", 0.97f), ReceiptTextLine("Desconto R$ 10,00", 0.95f),
        ))
        assertNull(grossWithDiscount.paid)
        assertEquals("10,00", grossWithDiscount.discount?.decimal)

        val contradictory = ReceiptMoneyReading.read(sequenceOf(
            ReceiptTextLine("Valor total R$ 250,00", 0.97f), ReceiptTextLine("Desconto R$ 10,00", 0.95f),
            ReceiptTextLine("Valor a pagar R$ 245,00", 0.91f),
        ))
        assertNull(contradictory.paid)
        assertNull(contradictory.discount)
    }

    @Test fun repeatsConvergeWithoutClaimingUnavailableConfidence() {
        val repeated = ReceiptMoneyReading.read(sequenceOf(
            ReceiptTextLine("Total Pago 20,10", 0.0f), ReceiptTextLine("Total 20,10", 0.92f),
        ))
        assertEquals("20,10", repeated.paid?.decimal)
        assertNull(repeated.paid?.confidencePercent)
    }
}
