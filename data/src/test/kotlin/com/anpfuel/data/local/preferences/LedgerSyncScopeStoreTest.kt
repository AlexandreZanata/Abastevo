package com.anpfuel.data.local.preferences

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class LedgerSyncScopeStoreTest {
    @Test fun `device and datasets default on and persist independently on reopen`() {
        val map = mutableMapOf<String, Boolean>()
        fun store() = LedgerSyncScopeStore({ key, default -> map[key] ?: default }, { key, value -> map[key] = value })
        val first = store()
        assertTrue(first.isDeviceEnabled())
        assertTrue(first.isDatasetEnabled("refuel"))
        first.setDeviceEnabled(false)
        first.setDatasetEnabled("refuel", false)
        assertFalse(store().isDeviceEnabled())
        assertFalse(store().isDatasetEnabled("refuel"))
        assertTrue(store().isDatasetEnabled("expense"))
        first.setDatasetEnabled("refuel", true)
        assertTrue(store().isDatasetEnabled("refuel"))
    }
}
