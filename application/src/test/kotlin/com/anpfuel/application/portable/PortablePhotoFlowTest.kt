package com.anpfuel.application.portable

import com.anpfuel.application.portable.PhotoFlow.Dims
import com.anpfuel.application.portable.PhotoFlow.EncodeRequest
import com.anpfuel.application.portable.PhotoFlow.PhotoResult
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

private class FakeDecoder(var dims: Dims?) : PhotoDecoder {
    var probes = 0
    override fun probeDims(bytes: ByteArray): Dims? {
        probes++
        return dims
    }
}

private class ScriptedEncoder(private val script: List<ByteArray?>) : PhotoEncoder {
    val attempts = mutableListOf<Int>()
    override fun encode(source: ByteArray, request: EncodeRequest): ByteArray? {
        attempts.add(request.attempt)
        return script.getOrNull(request.attempt - 1)
    }
}

private class FakeCache : PhotoCache {
    val entries = mutableMapOf<String, ByteArray>()
    var swept = 0
    var originalTime: Long? = null
    override fun put(id: String, bytes: ByteArray, capturedAtMillis: Long) {
        entries[id] = bytes
        originalTime = capturedAtMillis
    }
    override fun get(id: String): ByteArray? = entries[id]
    override fun delete(id: String) {
        entries.remove(id)
    }
    override fun sweepExpired(nowMillis: Long): Int {
        swept++
        return 0
    }
}

private class FakePhotoClock(var now: Long = 1_700_000_000_000L) : PhotoClock {
    override fun nowMillis(): Long = now
}

private fun flow(
    dims: Dims? = Dims(4000, 3000),
    script: List<ByteArray?> = listOf(ByteArray(100_000)),
    cache: FakeCache = FakeCache(),
): Triple<PhotoFlow, FakeCache, ScriptedEncoder> {
    val decoder = FakeDecoder(dims)
    val encoder = ScriptedEncoder(script)
    val flow = PhotoFlow(
        PhotoPorts(
            decoder = decoder,
            encoder = encoder,
            cache = cache,
            clock = FakePhotoClock(),
            ids = PhotoIdSource { "entry-1" },
        ),
    )
    return Triple(flow, cache, encoder)
}

class PortablePhotoFlowTest {

    @Test
    fun happyPreparePublishesWithinCap() {
        val (flow, cache, encoder) = flow()
        val result = flow.prepare(ByteArray(10), "image/jpeg")
        val ready = result as PhotoResult.Ready
        assertEquals("entry-1", ready.id)
        assertEquals(100_000L, ready.bytes)
        assertEquals(1, ready.attempts)
        assertEquals(listOf(1), encoder.attempts)
        assertEquals(100_000, cache.entries["entry-1"]?.size)
    }

    @Test
    fun unsupportedIntentRefusesBeforeProbe() {
        val (flow, cache, _) = flow()
        val result = flow.prepare(ByteArray(10), "image/gif")
        assertEquals(PhotoResult.Refused("unsupported-format"), result)
        assertTrue(cache.entries.isEmpty(), "refusals never publish")
    }

    @Test
    fun corruptProbeRefuses() {
        val (flow, cache, _) = flow(dims = null)
        val result = flow.prepare(ByteArray(10), "image/png")
        assertEquals(PhotoResult.Refused("undecodable-input"), result)
        assertTrue(cache.entries.isEmpty())
    }

    @Test
    fun oversizeAttemptsRetryThenRefuseOverBudget() {
        val big = ByteArray(300_000)
        val (flow, cache, encoder) = flow(script = listOf(big, big, big))
        val result = flow.prepare(ByteArray(10), "image/heif")
        assertEquals(PhotoResult.Refused("over-budget"), result)
        assertEquals(listOf(1, 2, 3), encoder.attempts)
        assertTrue(cache.entries.isEmpty(), "interrupted capture leaves nothing")
    }

    @Test
    fun shrinkingAttemptWinsOnSecondTry() {
        val big = ByteArray(300_000)
        val small = ByteArray(140_000)
        val (flow, cache, encoder) = flow(script = listOf(big, small))
        val result = flow.prepare(ByteArray(10), "image/jpeg") as PhotoResult.Ready
        assertEquals(140_000L, result.bytes)
        assertEquals(2, result.attempts)
        assertEquals(listOf(1, 2), encoder.attempts)
    }

    @Test
    fun nullEncodesRefuseEncodeFailed() {
        val (flow, cache, _) = flow(script = listOf(null, null, null))
        val result = flow.prepare(ByteArray(10), "image/jpeg")
        assertEquals(PhotoResult.Refused("encode-failed"), result)
        assertTrue(cache.entries.isEmpty())
    }

    @Test
    fun samplePlanMatchesFrame() {
        val decoder = FakeDecoder(Dims(8000, 6000))
        var seen = -1
        val ports = PhotoPorts(
            decoder = decoder,
            encoder = object : PhotoEncoder {
                override fun encode(source: ByteArray, request: EncodeRequest): ByteArray? {
                    seen = request.sampleSize
                    return ByteArray(100_000)
                }
            },
            cache = FakeCache(),
            clock = FakePhotoClock(),
            ids = PhotoIdSource { "e" },
        )
        PhotoFlow(ports).prepare(ByteArray(3), "image/jpeg")
        assertEquals(8, seen, "8000x6000 plans 8x sample-decode")
    }

    @Test
    fun discardAndSweepDelegateToCache() {
        val (flow, cache, _) = flow()
        flow.prepare(ByteArray(10), "image/jpeg")
        flow.discard("entry-1")
        assertNull(cache.entries["entry-1"])
        flow.discard("ghost")
        assertEquals(0, flow.sweepExpired())
        assertEquals(1, cache.swept)
    }
    @Test fun cropsRetainOriginalAgeAndExpiredInputNeverPublishes() {
        val (flow, cache, encoder) = flow()
        val original = 1_700_000_000_000L - 60_000
        assertTrue(flow.prepareAt(ByteArray(10), "image/jpeg", original) is PhotoResult.Ready)
        assertEquals(original, cache.originalTime)
        flow.discard("entry-1")
        val attempts = encoder.attempts.size
        listOf(0L, 1_700_000_000_001L, 1_700_000_000_000L - 86_400_000).forEach {
            assertEquals(PhotoResult.Refused("photo.expired"), flow.prepareAt(ByteArray(10), "image/jpeg", it))
        }
        assertTrue(cache.entries.isEmpty())
        assertEquals(attempts, encoder.attempts.size)
    }

}
