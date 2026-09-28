package com.anpfuel.data.parser

import java.time.LocalDate

/**
 * Maps Excel serial dates from ANP XLSX files to [LocalDate].
 */
object AnpDateMapper {

    private val EXCEL_EPOCH = LocalDate.of(1899, 12, 30)

    fun toLocalDate(serial: Number): LocalDate =
        EXCEL_EPOCH.plusDays(serial.toLong())

    fun toLocalDate(serial: String): LocalDate =
        toLocalDate(serial.toDouble())

    fun parseCellValue(value: String?): LocalDate? {
        if (value.isNullOrBlank()) return null
        return runCatching { toLocalDate(value) }.getOrNull()
    }
}
