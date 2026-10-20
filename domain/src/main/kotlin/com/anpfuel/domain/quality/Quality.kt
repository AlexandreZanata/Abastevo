package com.anpfuel.domain.quality

private val canonicalId = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")

val QUALITY_FUELS = setOf("ETHANOL", "GASOLINE_REGULAR", "GASOLINE_ADDITIVED", "GASOLINE_PREMIUM_GRADE", "DIESEL_S500", "DIESEL_S10", "CNG", "LPG_P13")

data class QualitySelection(val stationId: String, val fuel: String, val revisionId: String? = null) {
    init {
        require(canonicalId.matches(stationId))
        require(fuel in QUALITY_FUELS)
        require(revisionId == null || canonicalId.matches(revisionId))
    }
}

data class QualityLease(
    val source: String,
    val stream: String,
    val edition: String,
    val revisionId: String?,
    val available: Boolean,
    val stale: Boolean,
    val publishedAt: String? = null,
    val status: String = "unavailable",
    val current: Boolean = false,
    val coverageNote: String = "",
)

data class QualityAssayCell(val fuel: String, val conformity: String, val count: Long, val lastObserved: String)

data class QualityDimensions(
    val fuel: String,
    val samples: Long,
    val visits: Long,
    val conforming: Long,
    val nonconforming: Long,
    val unresolved: Long,
    val oldestObserved: String,
    val newestObserved: String,
    val scoreState: String,
)

data class QualitySummary(
    val stationId: String,
    val lease: QualityLease,
    val cells: List<QualityAssayCell> = emptyList(),
    val dimensions: List<QualityDimensions> = emptyList(),
    val dimensionsAvailable: Boolean = false,
    val methodology: String = "",
    val generatedAt: String = "",
)

data class QualityAssay(
    val id: String,
    val sampleRef: String,
    val assay: String,
    val observedOn: String,
    val fuel: String,
    val result: String,
    val unit: String,
    val conformity: String,
    val sourceObservedAt: String? = null,
)

data class QualityPage<T>(val items: List<T>, val nextCursor: String?, val lease: QualityLease? = null)
data class QualitySnapshot(val summary: QualitySummary, val assays: QualityPage<QualityAssay>)

data class QualityRevision(
    val id: String,
    val edition: String,
    val status: String,
    val provenance: String,
    val current: Boolean,
) {
    val selectable: Boolean get() = provenance == "VERIFIED" && status in setOf("published", "superseded")
}

data class QualitySample(val sampleRef: String, val assays: List<QualityAssay>, val lease: QualityLease)
data class QualityResource(val url: String, val sha256: String, val member: String)
data class QualityProvenance(val lease: QualityLease, val resources: List<QualityResource>)

class QualityReadUnavailable(val disabled: Boolean = false) : Exception("quality read unavailable")
