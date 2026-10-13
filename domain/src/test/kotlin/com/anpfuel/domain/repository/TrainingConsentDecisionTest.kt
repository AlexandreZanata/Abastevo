package com.anpfuel.domain.repository

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class TrainingConsentDecisionTest {
    @Test fun `OFF dominates a late grant and ambiguous retries keep the body`() {
        val on=TrainingConsentLocalState().grant("on", "v1", "digest",100)
        val off=on.revoke("off")
        assertFalse(off.granted)
        assertEquals(off,off.acknowledge(on.pending!!,TrainingConsentReceipt("GRANTED",101,"v1",false)))
        val bound=off.bindRevocation("off",102)
        assertEquals(103L,bound.pending!!.revision)
        assertEquals(bound,bound.bindRevocation("off",999))
        assertEquals(1000L,bound.bindRevocation("off",999,true).pending!!.revision)
    }
    @Test fun `pending grants never rebase or override OFF`() {
        val pending=TrainingConsentLocalState().grant("on","v1","digest",10)
        assertEquals(pending,pending.grant("new","v1","digest",100))
        assertThrows(TrainingConsentException::class.java) { pending.grant("new","v2","digest",100) }
        assertThrows(TrainingConsentException::class.java) { pending.revoke("off").grant("new","v1","digest",100) }
    }
    @Test fun `only exact receipts acknowledge and malformed states refuse`() {
        val pending=TrainingConsentLocalState().grant("on","v1","digest",0)
        assertThrows(TrainingConsentException::class.java) { pending.acknowledge(pending.pending!!,TrainingConsentReceipt("REVOKED",1,"v1",false)) }
        assertThrows(TrainingConsentException::class.java) { pending.acknowledge(pending.pending!!,TrainingConsentReceipt("GRANTED",1,"v2",false)) }
        assertEquals(pending,pending.acknowledge(pending.pending!!,TrainingConsentReceipt("GRANTED",2,"v1",false)))
        assertTrue(pending.acknowledge(pending.pending!!,TrainingConsentReceipt("GRANTED",1,"v1",true)).granted)
    }
    @Test fun `retry bounds saturate without overflow and revisions cannot wrap`() {
        var state=TrainingConsentLocalState().revoke("off")
        repeat(50) { state=state.failed(state.pending!!,1000,7200) }
        assertEquals(16,state.attempts);assertEquals(7201000L,state.retryAtMillis)
        assertEquals(Long.MAX_VALUE,state.failed(state.pending!!,Long.MAX_VALUE-1,7200).retryAtMillis)
        assertThrows(TrainingConsentException::class.java) { state.bindRevocation("off",Long.MAX_VALUE) }
        assertThrows(IllegalArgumentException::class.java) { state.failed(state.pending!!,-1,0) }
    }
    @Test fun `server rollback cannot manufacture a higher fresh ON revision`() {
        val state=TrainingConsentLocalState(100,true)
        assertThrows(TrainingConsentException::class.java) { state.grant("on","v1","digest",0) }
        assertEquals(101L,state.grant("on","v1","digest",100).pending!!.revision)
    }

}
