package com.anpfuel.domain.enterprise.tracking

/** The counted inputs of one track summary. */
data class TrackEvidence(val expectedSeconds: Long, val gapSeconds: Long, val acceptedFixes: Long, val coarse: Boolean)

/** The quality of one track summary with its permille coverage, null for zero expected time. */
class TrackClassification internal constructor(val quality: TrackQuality, val coveragePermille: Long?)

/** The track quality rule (B-BR-EMP-427, BR-GPS-026). */
object TrackQualityRules {
    /** Bounds the expected recording time (EDR-276); the instant profile spans less than 3.2×10¹¹ s. */
    const val MAX_EXPECTED_SECONDS: Long = 1_000_000_000_000L

    /**
     * Coverage = floor((expected − gap) × 1000 / expected); INSUFFICIENT without expected time, with fewer than two
     * accepted fixes or below 500; APPROXIMATE with any coarse period or below 900; otherwise PRECISE.
     */
    fun classify(evidence: TrackEvidence): TrackClassification {
        val (expected, gap, fixes, coarse) = evidence
        if (expected < 0 || gap < 0 || fixes < 0 || expected > MAX_EXPECTED_SECONDS || gap > expected) throw invalid()
        if (expected == 0L) return TrackClassification(TrackQuality.INSUFFICIENT, null)
        val coverage = (expected - gap) * 1000 / expected
        val quality = when {
            fixes < 2 || coverage < 500 -> TrackQuality.INSUFFICIENT
            coarse || coverage < 900 -> TrackQuality.APPROXIMATE
            else -> TrackQuality.PRECISE
        }
        return TrackClassification(quality, coverage)
    }
}
