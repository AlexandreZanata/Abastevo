package com.anpfuel.app.ui.map

import com.anpfuel.data.map.PACKAGE_DOWNLOADING
import com.anpfuel.data.map.PACKAGE_FAILED
import com.anpfuel.data.map.PACKAGE_PENDING
import com.anpfuel.data.map.PACKAGE_READY
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/** B-BR-MAP-DL-PROGRESS-01: every stored package status shows its real phase. */
class MapPackagePhaseTest {

    @Test
    fun `each store status maps to its phase`() {
        assertEquals(MapPackagePhase.QUEUED, mapPackagePhase(PACKAGE_PENDING))
        assertEquals(MapPackagePhase.DOWNLOADING, mapPackagePhase(PACKAGE_DOWNLOADING))
        assertEquals(MapPackagePhase.READY, mapPackagePhase(PACKAGE_READY))
        assertEquals(MapPackagePhase.FAILED, mapPackagePhase(PACKAGE_FAILED))
    }

    @Test
    fun `a finished package is never shown as queued`() {
        // Regression: the screen once matched "COMPLETE"/"VERIFIED" while the store writes READY.
        assertEquals(MapPackagePhase.READY, mapPackagePhase(PACKAGE_READY))
    }

    @Test
    fun `unknown statuses read as queued`() {
        assertEquals(MapPackagePhase.QUEUED, mapPackagePhase("COMPLETE"))
        assertEquals(MapPackagePhase.QUEUED, mapPackagePhase(""))
    }
}
