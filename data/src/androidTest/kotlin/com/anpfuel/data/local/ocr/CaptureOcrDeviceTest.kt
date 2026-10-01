package com.anpfuel.data.local.ocr

import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * P10-T04 — Device OCR adapter check (runs on `connectedDebugAndroidTest`).
 *
 * The JVM suites own the parsing matrix; this device test proves the same
 * adapter resolves through the Android graph and replays the frozen
 * vectors on a real device. Camera/ML Kit engine measurement belongs to
 * the P10-T08 device pass, never claimed here.
 */
@RunWith(AndroidJUnit4::class)
class CaptureOcrDeviceTest {

    @Test
    fun localOcrReplaysFrozenVectorsOnDevice() {
        val ocr = LocalRegexPriceOcr()
        val single = ocr.candidatesFromText("R$ 5,89")
        assertEquals(1, single.size)
        assertEquals(5890L, single[0].priceMilli)

        val multi = ocr.candidatesFromText("R$ 6,19\nR$ 5,89")
        assertEquals(2, multi.size)
        assertEquals(6190L, multi[0].priceMilli)
        assertEquals(5890L, multi[1].priceMilli)
        assertTrue(ocr.candidatesFromText("").isEmpty())
    }
}
