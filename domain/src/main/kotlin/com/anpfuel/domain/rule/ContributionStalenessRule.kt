package com.anpfuel.domain.rule

import com.anpfuel.domain.portable.PortablePhoto

/**
 * P10-T05 capture-age labelling (MIGRATION_PLAN outbox, BUC-003).
 *
 * Old queued data is labelled, never promoted to "now": a capture older
 * than the 24 h transient lifetime or more than 5 minutes in the future
 * (unreliable clock) is HISTORICAL-ONLY. Fresh captures stay FRESH.
 * This rule never mutates the payload; the worker/gateway carries the
 * label so an old photo is never relabelled as fresh on retry.
 */
object ContributionStalenessRule {

    const val FUTURE_SKEW_MILLIS: Long = 5L * 60L * 1000L

    fun isHistorical(capturedAtMillis: Long, nowMillis: Long): Boolean {
        if (capturedAtMillis - nowMillis > FUTURE_SKEW_MILLIS) return true
        return PortablePhoto.isTransientExpired(capturedAtMillis, nowMillis)
    }

    fun labelFor(capturedAtMillis: Long, nowMillis: Long): String =
        if (isHistorical(capturedAtMillis, nowMillis)) "historical" else "fresh"
}
