package com.anpfuel.domain.enterprise.attachment

import com.anpfuel.domain.enterprise.EnterpriseValueException
import com.anpfuel.domain.enterprise.EnterpriseValues
import kotlin.math.roundToInt

/** Transient input from the on-device recognizer. */
class ReceiptTextLine(val text: String, val confidence: Float?) {
    override fun toString(): String = "ReceiptTextLine(redacted)"
}

class ReceiptMoneySuggestion(val decimal: String, val confidencePercent: Int?) {
    override fun toString(): String = "ReceiptMoneySuggestion(redacted)"
}

/** EMP-DOC-009 suggestions for explicit BRL amounts in a synthetic receipt. */
class ReceiptMoneyReading private constructor(val paid: ReceiptMoneySuggestion?, val discount: ReceiptMoneySuggestion?) {
    override fun toString(): String = "ReceiptMoneyReading(redacted)"

    companion object {
        private const val AMOUNT = "([0-9]{1,11}|[0-9]{1,3}(?:\\.[0-9]{3}){1,3}),([0-9]{2})"
        private const val TAIL = "\\s*:?\\s*(?:R\\$\\s*)?$AMOUNT"

        // An NFC-e prints the item total before discount as "Valor total" and the
        // net amount as "Valor a pagar"; only the net amount is what was paid.
        private val payable = Regex("(?:(?:valor|total)\\s+a\\s+pagar|(?:valor|total)\\s+pago)$TAIL", RegexOption.IGNORE_CASE)
        private val gross = Regex("(?:valor\\s+)?total$TAIL", RegexOption.IGNORE_CASE)
        private val discount = Regex("descontos?(?:\\s+total)?$TAIL", RegexOption.IGNORE_CASE)

        fun read(lines: Sequence<ReceiptTextLine>): ReceiptMoneyReading {
            val bounded = lines.take(256).filter { it.text.length <= 128 }.toList()
            val payableField = readField(bounded, payable)
            val grossField = readField(bounded, gross)
            val discountField = readField(bounded, discount)
            if (payableField == null) {
                // A gross total is the paid amount only when nothing was discounted.
                return ReceiptMoneyReading(grossField.takeIf { discountField == null }, discountField)
            }
            val reconciles = grossField == null || discountField == null ||
                centavos(grossField) - centavos(discountField) == centavos(payableField)
            return if (reconciles) ReceiptMoneyReading(payableField, discountField) else ReceiptMoneyReading(null, null)
        }

        private fun centavos(suggestion: ReceiptMoneySuggestion): Long =
            EnterpriseValues.parseCentavos(suggestion.decimal.replace(",", "").trimStart('0').ifEmpty { "0" })

        private fun readField(lines: List<ReceiptTextLine>, field: Regex): ReceiptMoneySuggestion? {
            val candidates = lines.mapNotNull { line ->
                val match = field.matchEntire(line.text.replace('\u00a0', ' ').trim()) ?: return@mapNotNull null
                val integer = match.groupValues[1].replace(".", "").trimStart('0').ifEmpty { "0" }
                val fractions = match.groupValues[2]
                val exact = (integer + fractions).trimStart('0').ifEmpty { "0" }
                try { EnterpriseValues.parseCentavos(exact) }
                catch (_: EnterpriseValueException) { return@mapNotNull null }
                val confidence = line.confidence?.takeIf { it > 0f && it <= 1f }?.let { (it * 100).roundToInt() }
                ReceiptMoneySuggestion("$integer,$fractions", confidence)
            }
            val value = candidates.map { it.decimal }.distinct().singleOrNull() ?: return null
            val confidence = if (candidates.any { it.confidencePercent == null }) null
                else candidates.mapNotNull { it.confidencePercent }.minOrNull()
            return ReceiptMoneySuggestion(value, confidence)
        }
    }
}
