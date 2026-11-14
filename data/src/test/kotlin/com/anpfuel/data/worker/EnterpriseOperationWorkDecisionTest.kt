package com.anpfuel.data.worker

import androidx.work.ExistingWorkPolicy
import androidx.work.ListenableWorker
import com.anpfuel.data.enterprise.EnterpriseOperationWorkNext
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** The delivery worker's WorkManager outcome for one invocation (EDR-122). */
class EnterpriseOperationWorkDecisionTest {
    private val scheduled = mutableListOf<Long>()
    private fun decide(next: EnterpriseOperationWorkNext, accepted: Boolean = true) =
        EnterpriseOperationWorkDecision.result(next) { scheduled += it; accepted }

    @Test fun idleOrSignedOutWorkSucceedsWithoutAnyReschedule() {
        assertEquals(ListenableWorker.Result.success(), decide(EnterpriseOperationWorkNext.Done))
        assertTrue(scheduled.isEmpty())
    }

    @Test fun unknownFailureKeepsSystemBackoff() {
        assertEquals(ListenableWorker.Result.retry(), decide(EnterpriseOperationWorkNext.Backoff))
        assertTrue(scheduled.isEmpty())
    }

    @Test fun knownDueTimeEnqueuesABoundedDelayedContinuationInsteadOfBlindBackoff() {
        assertEquals(ListenableWorker.Result.success(), decide(EnterpriseOperationWorkNext.Continue(120_000)))
        assertEquals(ListenableWorker.Result.success(), decide(EnterpriseOperationWorkNext.Continue(Long.MAX_VALUE)))
        assertEquals(ListenableWorker.Result.success(), decide(EnterpriseOperationWorkNext.Continue(-5)))
        assertEquals(listOf(120_000L, 21_600_000L, 0L), scheduled)
    }

    @Test fun aContinuationThatCannotBeEnqueuedFallsBackToBackoff() {
        assertEquals(ListenableWorker.Result.retry(), decide(EnterpriseOperationWorkNext.Continue(1_000), accepted = false))
        assertEquals(ListenableWorker.Result.retry(), EnterpriseOperationWorkDecision.result(EnterpriseOperationWorkNext.Continue(1_000)) {
            error("private scheduler diagnostic")
        })
    }

    @Test fun aRunningContinuationAppendsItsSuccessorAndOtherRunsReplaceAPendingOne() {
        assertEquals(ExistingWorkPolicy.APPEND_OR_REPLACE, EnterpriseOperationWorkDecision.policy(fromContinuation = true))
        assertEquals(ExistingWorkPolicy.REPLACE, EnterpriseOperationWorkDecision.policy(fromContinuation = false))
    }
}
