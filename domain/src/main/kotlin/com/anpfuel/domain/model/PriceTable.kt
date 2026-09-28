package com.anpfuel.domain.model

import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Downloadable XLSX file published by ANP for a [SurveyWeek].
 */
data class PriceTable(
    val id: DomainId,
    val surveyWeek: SurveyWeek,
    val tableType: PriceTableType,
    val sourceUrl: String,
    val checksum: String? = null,
) {
    companion object {
        fun create(
            surveyWeek: SurveyWeek,
            tableType: PriceTableType,
            sourceUrl: String,
            checksum: String? = null,
            id: DomainId = DomainId.generate(),
        ): PriceTable = PriceTable(
            id = id,
            surveyWeek = surveyWeek,
            tableType = tableType,
            sourceUrl = sourceUrl,
            checksum = checksum,
        )
    }
}
