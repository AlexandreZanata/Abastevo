package com.anpfuel.app.mapper

import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import java.text.NumberFormat
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Currency
import java.util.Locale

object PriceFormatter {

    private val currency = Currency.getInstance("BRL")
    private val brazilianRealLocale: Locale = Locale.forLanguageTag("pt-BR")

    /**
     * Formats ANP fuel prices in Brazilian Real (BRL) regardless of UI locale.
     */
    fun formatAmount(amount: PriceAmount?, @Suppress("UNUSED_PARAMETER") locale: Locale): String? {
        amount ?: return null
        val formatter = NumberFormat.getCurrencyInstance(brazilianRealLocale).apply {
            this.currency = currency
            minimumFractionDigits = 2
            maximumFractionDigits = 2
        }
        return formatter.format(amount.value)
    }
}

object SurveyWeekFormatter {

    fun formatRange(week: SurveyWeek, locale: Locale): String {
        val formatter = DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM).withLocale(locale)
        val separator = if (locale.language == "pt") " a " else " – "
        return "${week.startDate.format(formatter)}$separator${week.endDate.format(formatter)}"
    }

    /**
     * Shorter week range for constrained UI (top bar chip, history rows).
     * Same month: "Jun 7–13, 2026" (en) / "7–13 jun 2026" (pt).
     */
    fun formatRangeCompact(week: SurveyWeek, locale: Locale): String {
        val start = week.startDate
        val end = week.endDate
        if (start == end) {
            return formatRange(week, locale)
        }

        val dayFormatter = DateTimeFormatter.ofPattern("d", locale)
        val monthFormatter = DateTimeFormatter.ofPattern("MMM", locale)
        val monthDayFormatter = DateTimeFormatter.ofPattern("MMM d", locale)

        return when {
            start.year == end.year && start.month == end.month -> {
                if (locale.language == "pt") {
                    "${start.format(dayFormatter)}–${end.format(dayFormatter)} " +
                        "${start.format(monthFormatter)} ${start.year}"
                } else {
                    "${start.format(monthFormatter)} ${start.format(dayFormatter)}–${end.format(dayFormatter)}, ${start.year}"
                }
            }

            start.year == end.year -> {
                if (locale.language == "pt") {
                    "${start.format(dayFormatter)} ${start.format(monthFormatter)} – " +
                        "${end.format(dayFormatter)} ${end.format(monthFormatter)} ${end.year}"
                } else {
                    "${start.format(monthDayFormatter)} – ${end.format(monthDayFormatter)}, ${end.year}"
                }
            }

            else -> {
                if (locale.language == "pt") {
                    "${start.format(dayFormatter)} ${start.format(monthFormatter)} ${start.year} – " +
                        "${end.format(dayFormatter)} ${end.format(monthFormatter)} ${end.year}"
                } else {
                    formatRange(week, locale)
                }
            }
        }
    }
}

object LastSyncFormatter {

    fun format(instant: Instant?, locale: Locale, zoneId: ZoneId = ZoneId.systemDefault()): String? {
        instant ?: return null
        val formatter = DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT)
            .withLocale(locale)
            .withZone(zoneId)
        return formatter.format(instant)
    }
}
