package com.anpfuel.app.community

import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith

/**
 * P10-T06 — Device presentation check (runs on `connectedDebugAndroidTest`).
 *
 * The JVM suites own the UNKNOWN/DISPUTED/STALE matrix; this device test
 * proves the same pure display resolves through the Android graph and
 * replays the frozen labels on a real device. Backend fetch and ML Kit
 * measurement belong to the P10-T08 device pass, never claimed here.
 */
@RunWith(AndroidJUnit4::class)
class CommunityPriceDisplayDeviceTest {

    @Test
    fun displayReplaysFrozenLabelsOnDevice() {
        assertEquals("R$ 5,890", CommunityPriceDisplay.formatMilliBrl(5890L))
        assertEquals("STANDARD", CommunityPriceDisplay.formatCondition("STANDARD", null))
        assertEquals(
            "DISPUTED",
            CommunityPriceDisplay.availabilityLabel(CommunityAvailability.DISPUTED),
        )
        assertEquals(
            "STALE",
            CommunityPriceDisplay.freshnessLabel(CommunityFreshness.STALE),
        )
    }
}
