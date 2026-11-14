package com.anpfuel.domain.enterprise.sync

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class SyncLimitsTest {
    @Test fun `trusted configuration narrows hard caps and request cannot choose limits`() {
        val small = SyncLimits.trusted(maxOperations = 1, maxBatchBytes = 1024, maxOperationBytes = 512, maxTransportDepth = 4)
        assertEquals(1, small.maxOperations)
        assertEquals(1024, small.maxBatchBytes)
        assertEquals(512, small.maxOperationBytes)
        assertEquals(4, small.maxTransportDepth)
        assertEquals(2, small.maxOperationDepth)
        val invalid = listOf<() -> Unit>(
            { SyncLimits.trusted(maxOperations = 0) }, { SyncLimits.trusted(maxOperations = 51) },
            { SyncLimits.trusted(maxBatchBytes = 0) }, { SyncLimits.trusted(maxBatchBytes = 65537) },
            { SyncLimits.trusted(maxOperationBytes = 0) }, { SyncLimits.trusted(maxOperationBytes = 8193) },
            { SyncLimits.trusted(maxTransportDepth = 3) }, { SyncLimits.trusted(maxTransportDepth = 65) },
            { SyncLimits.trusted(maxBatchBytes = 100, maxOperationBytes = 101) },
        )
        invalid.forEach { assertThrows(SyncProtocolException::class.java, it) }
        val wrapper = """{"schema_version":1,"family_id":"7b0e2a54-0000-4000-8000-000000000001","access_token":"proof","operations":[null,null]}"""
        assertEquals("LIMIT", assertThrows(SyncProtocolException::class.java) { SyncBatch.parse(wrapper.toByteArray(), small) }.code)
        assertEquals(0, SyncBatch.parse(wrapper.replace("null,null", "").toByteArray(), small).operations.size)
        assertThrows(SyncProtocolException::class.java) { SyncBatch.parse(wrapper.replace("\"operations\":", "\"limits\":{},\"operations\":").toByteArray()) }
    }
}
