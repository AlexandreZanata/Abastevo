package com.anpfuel.domain.repository

import com.anpfuel.domain.occurrence.*

/** Bounded, anonymous official occurrence reads. Every call carries its
 * source revision vector; private reports never enter this gateway. */
interface OccurrenceGateway {
    suspend fun records(
        uf: String? = null,
        municipality: String? = null,
        segment: String? = null,
        subjectKind: String? = null,
        fromDay: String? = null,
        toDay: String? = null,
        limit: Int = 20,
        cursor: String? = null,
        revisionId: String? = null,
    ): OccurrencePage<OccurrenceRecord>

    suspend fun recordCoverage(revisionId: String? = null): Pair<RecordCoverage, OccurrenceLease>

    suspend fun stations(
        state: String? = null,
        segment: String? = null,
        fromDay: String? = null,
        toDay: String? = null,
        limit: Int = 20,
        cursor: String? = null,
        revisionId: String? = null,
    ): OccurrencePage<OccurrenceStation>

    suspend fun stationCoverage(revisionId: String? = null): Pair<StationCoverage, OccurrenceLease>
}
