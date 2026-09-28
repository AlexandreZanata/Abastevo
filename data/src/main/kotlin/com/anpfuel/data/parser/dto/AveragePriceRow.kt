package com.anpfuel.data.parser.dto

import java.math.BigDecimal
import java.time.LocalDate

/**
 * Parsed row from the MUNICIPIOS sheet of a weekly summary XLSX file.
 */
data class AveragePriceRow(
    val startDate: LocalDate,
    val endDate: LocalDate,
    val state: String,
    val municipality: String,
    val productLabel: String,
    val stationCount: Int?,
    val unit: String?,
    val averagePrice: BigDecimal?,
    val standardDeviation: BigDecimal?,
    val minimumPrice: BigDecimal?,
    val maximumPrice: BigDecimal?,
    val variationCoefficient: BigDecimal?,
)
