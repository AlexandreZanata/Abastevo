package com.anpfuel.data.local.preferences

import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class LedgerSyncAckMemoryTest {
    private val map = mutableMapOf<String, Long>()
    private fun store() = LedgerSyncAckMemory({ key, default -> map[key] ?: default }, map::containsKey,
        { revKey, revision, timeKey, time -> map[revKey] = revision; map[timeKey] = time }, { keys -> keys.forEach { map.remove(it) } }, { map.keys.toSet() })
    @Test fun `ack is absent until recorded and survives reopen with exact revision and time`() = runTest {
        assertNull(store().lastAck("a", "o", "refuel"))
        store().recordAck("a", "o", "refuel", 7, 123)
        assertEquals(7L, store().lastAck("a", "o", "refuel")!!.revision)
        assertEquals(123L, store().lastAck("a", "o", "refuel")!!.atMillis)
        assertNull(store().lastAck("a", "o", "expense"))
    }
    @Test fun `clear removes only exact owner and origin and empty keys are ignored`() = runTest {
        store().recordAck("a", "o", "refuel", 1, 2)
        store().recordAck("b", "o", "refuel", 3, 4)
        store().recordAck("a", "other", "refuel", 5, 6)
        store().recordAck("", "o", "refuel", 1, 2)
        store().recordAck("a", "o", "", 1, 2)
        store().clearOwner("a", "o")
        assertNull(store().lastAck("a", "o", "refuel"))
        assertNotNull(store().lastAck("b", "o", "refuel"))
        assertNotNull(store().lastAck("a", "other", "refuel"))
        assertEquals(4, map.size)
    }
}
