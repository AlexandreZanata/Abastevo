package com.anpfuel.application.usecase.quality

import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.QualityGateway

/** Pin every paginated read to the summary's frozen source revision. */
class ReadQuality(private val gateway: QualityGateway) {
    suspend operator fun invoke(selection: QualitySelection, cursor: String? = null): QualitySnapshot {
        val summary = gateway.summary(selection.stationId, selection.fuel, selection.revisionId)
        check(summary.stationId == selection.stationId)
        val lease = summary.lease
        check(selection.revisionId == null || lease.revisionId == selection.revisionId)
        if (!lease.available) return QualitySnapshot(summary, QualityPage(emptyList(), null, lease))
        val revision = checkNotNull(lease.revisionId)
        val page = gateway.assays(selection.stationId, selection.fuel, revision, cursor)
        check(page.lease?.revisionId == revision)
        check(page.items.all { it.fuel == selection.fuel })
        return QualitySnapshot(summary.copy(lease=lease.copy(stale=lease.stale || page.lease?.stale==true)), page)
    }
}
