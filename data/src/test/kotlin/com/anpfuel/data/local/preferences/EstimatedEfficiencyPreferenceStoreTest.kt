package com.anpfuel.data.local.preferences

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class EstimatedEfficiencyPreferenceStoreTest {

    @Test
    fun `set get and clear are vehicle keyed`() {
        val backing = mutableMapOf<String, String>()
        val store = EstimatedEfficiencyPreferenceStore(
            read = { backing[it] },
            write = { key, value -> if (value == null) backing.remove(key) else backing[key] = value },
        )
        assertNull(store.get("v1"))
        store.set("v1", "12.5")
        store.set("v2", "9.0")
        assertEquals("12.5", store.get("v1"))
        store.clear("v1")
        assertNull(store.get("v1"))
        assertEquals("9.0", store.get("v2"))
        assertEquals("km_per_l|v1", EstimatedEfficiencyPreferenceStore.keyFor("v1"))
    }
}
