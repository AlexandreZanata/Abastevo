package com.anpfuel.domain.occurrence

private val canonicalId = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
private val calendarDay = Regex("[0-9]{4}-[0-9]{2}-[0-9]{2}")
private val ufCode = Regex("[A-Z]{2}")

private fun validDay(day: String): Boolean = try {
    java.time.LocalDate.parse(day).toString() == day
} catch (_: Exception) { false }

/** Typed source scope: source-global facts or one current company station.
 * Raw scope strings never cross this boundary; unknown scopes are refused,
 * never coerced. */
sealed interface OccurrenceScope {
    data object SourceGlobal : OccurrenceScope
    data class CurrentCompany(val stationId: String) : OccurrenceScope {
        init { require(canonicalId.matches(stationId)) }
    }
}

/** Source revision vector: the exact revision an item was read from.
 * Binds cache entries, cursors and ETags; a changed vector invalidates. */
data class SourceRevision(
    val source: String,
    val stream: String,
    val edition: String,
    val revisionId: String,
) {
    init {
        require(source.isNotEmpty() && stream.isNotEmpty() && edition.isNotEmpty())
        require(canonicalId.matches(revisionId))
    }
}

data class OccurrenceLease(
    val revision: SourceRevision?,
    val available: Boolean,
    val stale: Boolean,
    val coverageNote: String = "",
)

/** One enforcement source record, source-verbatim. Unknown enum values
 * (subject kinds, future statuses) are retained as raw strings, never
 * dropped or crashed on. No identifiers beyond the source wording. */
data class OccurrenceRecord(
    val id: String,
    val nativeKey: String,
    val subjectKind: String,
    val uf: String,
    val municipality: String,
    val segment: String,
    val documentDay: String,
    val dateBasis: String,
    val documentNumber: String,
    val procedure: String,
    val result: String,
    val evidenceRef: String,
) {
    init {
        require(canonicalId.matches(id))
        require(nativeKey.isNotEmpty() && nativeKey.length <= 512)
        require(ufCode.matches(uf))
        require(municipality.isNotEmpty() && documentNumber.isNotEmpty())
        require(calendarDay.matches(documentDay) && validDay(documentDay) && dateBasis == "document_day")
    }
}

/** One distinctly associated station with its bound occurrence count.
 * Display data only; a retired station never appears here (fenced live
 * by the API). Stations without prices are included. */
data class OccurrenceStation(
    val id: String,
    val displayName: String,
    val municipality: String?,
    val state: String?,
    val status: String,
    val recordCount: Long,
) {
    init {
        require(canonicalId.matches(id))
        require(displayName.isNotEmpty() && status == "active" && recordCount >= 1)
        require(state == null || ufCode.matches(state))
    }
}

data class OccurrencePage<T>(
    val items: List<T>,
    val nextCursor: String?,
    val lease: OccurrenceLease,
    val stale: Boolean = false,
    val lastCheckedAt: String = "",
)

data class RecordCoverage(
    val observations: Long,
    val cases: Long,
    val firstDay: String?,
    val lastDay: String?,
    val dateBasis: String,
)

data class StationCoverage(
    val stations: Long,
    val records: Long,
)
