package com.anpfuel.domain.repository

import com.anpfuel.domain.quality.*

/** Anonymous official reads only; private report commands use a separate port. */
interface QualityGateway {
    suspend fun summary(stationId: String, fuel: String, revisionId: String?): QualitySummary
    suspend fun assays(stationId: String, fuel: String, revisionId: String, cursor: String?): QualityPage<QualityAssay>
    suspend fun revisions(cursor: String?): QualityPage<QualityRevision>
    suspend fun sample(sampleRef: String, revisionId: String): QualitySample
    suspend fun provenance(revisionId: String): QualityProvenance
}
