package com.anpfuel.data.remote

import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeFormatterBuilder
import java.time.format.ResolverStyle
import java.time.temporal.ChronoField

internal object AnpBrazilianDateParser {

    val formatter: DateTimeFormatter = DateTimeFormatterBuilder()
        .appendValue(ChronoField.DAY_OF_MONTH, 1, 2, java.time.format.SignStyle.NOT_NEGATIVE)
        .appendLiteral('/')
        .appendValue(ChronoField.MONTH_OF_YEAR, 1, 2, java.time.format.SignStyle.NOT_NEGATIVE)
        .appendLiteral('/')
        .appendValue(ChronoField.YEAR, 4)
        .toFormatter()
        .withResolverStyle(ResolverStyle.STRICT)

    fun parseDate(text: String): LocalDate? =
        try {
            LocalDate.parse(text.trim(), formatter)
        } catch (_: Exception) {
            null
        }
}
