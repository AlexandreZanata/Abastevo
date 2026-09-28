package com.anpfuel.domain.rule

import com.anpfuel.domain.event.SyncJobOutcome

/**
 * UC-002 — Onboarding completes only after the first successful summary import is available locally.
 */
object OnboardingCompletionRule {

    fun canMarkComplete(
        syncOutcome: SyncJobOutcome,
        hasImportedSummary: Boolean,
    ): Boolean {
        if (!hasImportedSummary) {
            return false
        }
        return syncOutcome in COMPLETABLE_OUTCOMES
    }

    private val COMPLETABLE_OUTCOMES = setOf(
        SyncJobOutcome.SUCCESS,
        SyncJobOutcome.PARTIAL,
        SyncJobOutcome.NO_NEW_DATA,
    )
}
