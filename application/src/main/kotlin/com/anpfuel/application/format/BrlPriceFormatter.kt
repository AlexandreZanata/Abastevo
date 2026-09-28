package com.anpfuel.application.format

import com.anpfuel.domain.valueobject.PriceAmount
import java.text.NumberFormat
import java.util.Currency
import java.util.Locale

object BrlPriceFormatter {

    private val locale = Locale.forLanguageTag("pt-BR")
    private val currency = Currency.getInstance("BRL")

    fun format(amount: PriceAmount): String {
        val formatter = NumberFormat.getCurrencyInstance(locale).apply {
            this.currency = currency
            minimumFractionDigits = 2
            maximumFractionDigits = 2
        }
        return formatter.format(amount.value)
    }
}
