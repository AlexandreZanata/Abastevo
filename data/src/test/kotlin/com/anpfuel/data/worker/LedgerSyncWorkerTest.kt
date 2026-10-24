package com.anpfuel.data.worker

import androidx.work.ListenableWorker
import com.anpfuel.application.usecase.ledger.LedgerDispatchOutcome
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class LedgerSyncWorkerMappingTest {

    @Test
    fun `outcomes map to worker results without data loss`() {
        assertEquals(ListenableWorker.Result.success(), LedgerSyncWorker.toWorkResult(LedgerDispatchOutcome.DONE))
        assertEquals(ListenableWorker.Result.retry(), LedgerSyncWorker.toWorkResult(LedgerDispatchOutcome.RETRY))
        assertEquals(ListenableWorker.Result.failure(), LedgerSyncWorker.toWorkResult(LedgerDispatchOutcome.REFUSED))
    }
}
