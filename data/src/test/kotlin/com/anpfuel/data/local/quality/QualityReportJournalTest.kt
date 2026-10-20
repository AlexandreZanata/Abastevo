package com.anpfuel.data.local.quality

import com.anpfuel.application.portable.PortableOutbox
import com.anpfuel.data.local.dao.QualityReportOutboxDao
import com.anpfuel.data.local.entity.*
import io.mockk.*
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** Runs real transaction orchestration; production SQL has separate real SQLite tests. */
class QualityReportJournalTest {
    private val gate=QualityOwnerGateEntity("a".repeat(64),"synthetic-owner","https://synthetic.invalid",false,100,1000)
    private val row=QualityReportOutboxEntity(gate.ownerKey,"eeeeeeee-5555-4555-8555-000000000001","a".repeat(64),"synthetic-sealed",100,900,"QUEUED",0,100,"",0)
    private fun dao():QualityReportOutboxDao {
        val dao=mockk<QualityReportOutboxDao>(relaxed=true)
        coEvery {dao.activate(any(),any())} coAnswers {callOriginal()}
        coEvery {dao.freeze(any(),any())} coAnswers {callOriginal()}
        coEvery {dao.complete(any(),any(),any(),any())} coAnswers {callOriginal()}
        coEvery {dao.failed(any(),any(),any(),any(),any())} coAnswers {callOriginal()}
        coEvery {dao.erase(any(),any(),any(),any())} coAnswers {callOriginal()}
        coEvery {dao.gate(gate.ownerKey)} returns gate
        coEvery {dao.touch(any(),any())} returns 1
        coEvery {dao.pending(any())} returns null
        coEvery {dao.gatesCount()} returns 0
        coEvery {dao.pendingCount()} returns 0
        return dao
    }
    @Test fun `same command reuses frozen ciphertext and foreign intent cannot replace it`()=runTest {
        val dao=dao()
        coEvery {dao.pending(gate.ownerKey)} returns row
        assertEquals(row,dao.freeze(row.copy(sealedCommand="new-ciphertext"),100))
        assertNull(dao.freeze(row.copy(commandId="eeeeeeee-5555-4555-8555-000000000002"),100))
        assertNull(dao.freeze(row.copy(payloadHash="b".repeat(64)),100))
        coVerify(exactly=0){dao.insertPending(any())}
    }
    @Test fun `pending gate and rollback budgets reject without evicting foreign live data`()=runTest {
        val dao=dao()
        coEvery {dao.pendingCount()} returns 25
        assertNull(dao.freeze(row,100))
        coEvery {dao.gate(any())} returns null
        coEvery {dao.gatesCount()} returns 64
        assertFalse(dao.activate(gate,100))
        coEvery {dao.gate(any())} returns gate
        coEvery {dao.touch(any(),any())} returns 0
        assertNull(dao.freeze(row,100));assertFalse(dao.activate(gate,100))
        coVerify(exactly=0){dao.insertPending(any())};coVerify(exactly=0){dao.insertGate(any())}
        assertEquals(25,PortableOutbox.MAX_DISPATCH_ATTEMPTS)
    }
    @Test fun `erasure creates an absent blocked epoch before purging so late activation cannot recreate intent`()=runTest {
        val dao=dao()
        coEvery {dao.gate(any())} returns null
        dao.erase(gate.ownerLocal,100,1000,gate.copy(blocked=true))
        coVerifyOrder {dao.gate(gate.ownerKey);dao.gatesCount();dao.insertGate(gate.copy(blocked=true));dao.blockOwner(gate.ownerLocal,100,1000);dao.eraseRows(gate.ownerLocal)}
    }
    @Test fun `completion and failure advance the clock fence and refuse rollback`()=runTest {
        val dao=dao()
        coEvery {dao.touch(gate.ownerKey,200)} returns 1
        coEvery {dao.touch(gate.ownerKey,199)} returns 0
        coEvery {dao.acknowledge(any(),any(),any(),any())} returns 1
        coEvery {dao.failClaim(any(),any(),any(),any(),any())} returns 1
        assertTrue(dao.complete(gate.ownerKey,row.commandId,"claim",200))
        assertFalse(dao.complete(gate.ownerKey,row.commandId,"claim",199))
        assertTrue(dao.failed(gate.ownerKey,row.commandId,"claim",200,300))
        assertFalse(dao.failed(gate.ownerKey,row.commandId,"claim",199,300))
        coVerify(exactly=0){dao.acknowledge(any(),any(),any(),199)}
        coVerify(exactly=0){dao.failClaim(any(),any(),any(),199,any())}
    }

}
