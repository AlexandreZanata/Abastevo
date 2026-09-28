package com.anpfuel.domain.state

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class DataReadinessStateTest {

    @Test
    fun enumContainsAllUserVisibleStates() {
        assertEquals(6, DataReadinessState.entries.size)
        assertEquals(
            setOf(
                DataReadinessState.EMPTY,
                DataReadinessState.SYNCING,
                DataReadinessState.PARTIAL,
                DataReadinessState.READY,
                DataReadinessState.STALE,
                DataReadinessState.ERROR,
            ),
            DataReadinessState.entries.toSet(),
        )
    }
}
