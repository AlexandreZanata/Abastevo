package com.anpfuel.domain.rule

import com.anpfuel.domain.event.SyncJobOutcome
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.EnumSource

class OnboardingCompletionRuleTest {

    @ParameterizedTest
    @EnumSource(
        value = SyncJobOutcome::class,
        names = ["SUCCESS", "PARTIAL", "NO_NEW_DATA"],
    )
    fun allowsCompletionWhenSummaryImported(outcome: SyncJobOutcome) {
        assertTrue(OnboardingCompletionRule.canMarkComplete(outcome, hasImportedSummary = true))
    }

    @Test
    fun rejectsFailedSyncEvenWithSummaryImported() {
        assertFalse(
            OnboardingCompletionRule.canMarkComplete(
                syncOutcome = SyncJobOutcome.FAILED,
                hasImportedSummary = true,
            ),
        )
    }

    @Test
    fun rejectsCompletionWithoutImportedSummary() {
        assertFalse(
            OnboardingCompletionRule.canMarkComplete(
                syncOutcome = SyncJobOutcome.SUCCESS,
                hasImportedSummary = false,
            ),
        )
    }
}
