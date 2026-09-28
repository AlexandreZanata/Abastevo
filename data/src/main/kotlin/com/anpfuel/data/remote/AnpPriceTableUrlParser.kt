package com.anpfuel.data.remote

import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import java.util.UUID

/**
 * Parses ANP XLSX URLs and file names into domain [PriceTable] metadata (BR-001, UC-001).
 * Supports canonical and legacy gov.br filename variants (Phase 12.2.5).
 */
object AnpPriceTableUrlParser {

    private val CANONICAL_PATTERN = Regex(
        """arquivos-lpc/\d{4}/(resumo[_-]semanal|revendas)[_-]lpc[_-](\d{4}-\d{2}-\d{2})[-_](\d{4}-\d{2}-\d{2})(?:-\d+)?\.xlsx""",
        RegexOption.IGNORE_CASE,
    )

    private val LEGACY_HYPHEN_PATTERN = Regex(
        """arquivos-lpc/\d{4}/(resumo[_-]semanal|revendas)[_-]lpc[_-](\d{4}-\d{2}-\d{2})[-_](\d{4}-\d{2}-\d{2})\.xlsx""",
        RegexOption.IGNORE_CASE,
    )

    private val DOT_DATE_PATTERN = Regex(
        """arquivos-lpc/\d{4}/(resumo[_-]semanal|revendas)[_-]lpc[_-](\d{4})\.(\d{2})\.(\d{2})[_-](\d{4})\.(\d{2})\.(\d{2})\.xlsx""",
        RegexOption.IGNORE_CASE,
    )

    private val COMPACT_DATE_PATTERN = Regex(
        """arquivos-lpc/\d{4}/(resumo[_-]semanal|revendas)[_-]lpc[_-](\d{2})(\d{2})(\d{4})-a-(\d{2})(\d{2})(\d{4})\.xlsx""",
        RegexOption.IGNORE_CASE,
    )

    private val LPC_FILE_PATTERN = Regex(
        """arquivos-lpc/\d{4}/.*\.xlsx""",
        RegexOption.IGNORE_CASE,
    )

    fun isPriceTableUrl(url: String): Boolean =
        CANONICAL_PATTERN.containsMatchIn(url) ||
            LEGACY_HYPHEN_PATTERN.containsMatchIn(url) ||
            DOT_DATE_PATTERN.containsMatchIn(url) ||
            COMPACT_DATE_PATTERN.containsMatchIn(url) ||
            LPC_FILE_PATTERN.containsMatchIn(url)

    fun toPriceTable(sourceUrl: String): PriceTable? {
        val normalizedUrl = sourceUrl.trim()
        val parsed = parseUrlMetadata(normalizedUrl) ?: return null

        return try {
            val surveyWeek = SurveyWeek.fromIsoDates(parsed.startDate, parsed.endDate)
            PriceTable.create(
                id = priceTableId(normalizedUrl),
                surveyWeek = surveyWeek,
                tableType = parsed.tableType,
                sourceUrl = normalizedUrl,
            )
        } catch (_: Exception) {
            null
        }
    }

    fun inferTableTypeFromLinkText(linkText: String): PriceTableType? {
        val normalized = linkText.lowercase()
        return when {
            normalized.contains("médios semanais") || normalized.contains("medios semanais") ->
                PriceTableType.WEEKLY_SUMMARY
            normalized.contains("posto revendedor") -> PriceTableType.STATION_DETAIL
            else -> null
        }
    }

    private fun parseUrlMetadata(sourceUrl: String): ParsedUrlMetadata? {
        CANONICAL_PATTERN.find(sourceUrl)?.let { match ->
            val tableType = tableTypeFromToken(match.groupValues[1]) ?: return null
            return ParsedUrlMetadata(
                tableType = tableType,
                startDate = match.groupValues[2],
                endDate = match.groupValues[3],
            )
        }

        LEGACY_HYPHEN_PATTERN.find(sourceUrl)?.let { match ->
            val tableType = tableTypeFromToken(match.groupValues[1]) ?: return null
            return ParsedUrlMetadata(
                tableType = tableType,
                startDate = match.groupValues[2],
                endDate = match.groupValues[3],
            )
        }

        DOT_DATE_PATTERN.find(sourceUrl)?.let { match ->
            val tableType = tableTypeFromToken(match.groupValues[1]) ?: return null
            return ParsedUrlMetadata(
                tableType = tableType,
                startDate = "${match.groupValues[2]}-${match.groupValues[3]}-${match.groupValues[4]}",
                endDate = "${match.groupValues[5]}-${match.groupValues[6]}-${match.groupValues[7]}",
            )
        }

        COMPACT_DATE_PATTERN.find(sourceUrl)?.let { match ->
            val tableType = tableTypeFromToken(match.groupValues[1]) ?: return null
            return ParsedUrlMetadata(
                tableType = tableType,
                startDate = "${match.groupValues[4]}-${match.groupValues[3]}-${match.groupValues[2]}",
                endDate = "${match.groupValues[7]}-${match.groupValues[6]}-${match.groupValues[5]}",
            )
        }

        return null
    }

    private fun tableTypeFromToken(typeToken: String): PriceTableType? =
        when (typeToken.lowercase().replace("-", "_")) {
            "resumo_semanal" -> PriceTableType.WEEKLY_SUMMARY
            "revendas" -> PriceTableType.STATION_DETAIL
            else -> null
        }

    private data class ParsedUrlMetadata(
        val tableType: PriceTableType,
        val startDate: String,
        val endDate: String,
    )

    private fun priceTableId(sourceUrl: String): DomainId =
        DomainId.from(UUID.nameUUIDFromBytes(sourceUrl.toByteArray()).toString())
}
