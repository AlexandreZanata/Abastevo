package com.anpfuel.domain.model

/**
 * Local storage footprint for imported ANP data (UC-008).
 */
data class StorageUsage(
    val summaryRowCount: Int,
    val stationRowCount: Int,
    val importedWeekCount: Int,
)
