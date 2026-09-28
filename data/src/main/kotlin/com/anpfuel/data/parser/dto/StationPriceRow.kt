package com.anpfuel.data.parser.dto

import java.math.BigDecimal
import java.time.LocalDate

/**
 * Parsed row from the POSTOS REVENDEDORES sheet of a station detail XLSX file.
 */
data class StationPriceRow(
    val cnpj: String,
    val legalName: String?,
    val tradeName: String?,
    val address: String,
    val number: String?,
    val complement: String?,
    val neighborhood: String?,
    val zipCode: String?,
    val municipality: String,
    val stateName: String,
    val brand: String?,
    val productLabel: String,
    val unit: String?,
    val price: BigDecimal,
    val collectedAt: LocalDate?,
)
