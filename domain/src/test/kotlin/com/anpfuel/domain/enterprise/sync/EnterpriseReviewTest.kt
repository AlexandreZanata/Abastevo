package com.anpfuel.domain.enterprise.sync

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseReviewTest {
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-%012d".format(n)
    private val digest = "sha256:" + "a".repeat(64)
    private fun value() = ReviewResolutionSnapshot(id(101), digest, id(102), id(103), 2, "ACCEPTED", "Private exact reason 😀", "2026-11-14T12:13:14.123456789Z", Long.MAX_VALUE)
    private fun input(selectors: List<ReviewSelector> = listOf(ReviewSelector(id(101), digest))) = ReviewResultsRequestSnapshot(id(901), id(9), "private-\"proof", id(3), id(4), selectors)
    private fun source() = ReviewSourceBinding(SyncBinding.of(id(1), "https://api.example.invalid", id(2)), id(3), id(4))
    @Test fun resolutionIsSeparateExactAndDefensivelyEncoded() {
        val v = ReviewResolution.create(value())
        assertEquals(Long.MAX_VALUE, v.snapshot().entityRevision)
        assertArrayEquals(v.canonicalBytes, ReviewResolution.parse(v.canonicalBytes).canonicalBytes)
        val bytes = v.canonicalBytes; bytes[0] = 0
        assertEquals('{'.code.toByte(), v.canonicalBytes[0])
        assertFalse(v.toString().contains("Private exact"))
        assertFalse(v.snapshot().toString().contains("Private exact"))
    }
    @Test fun refusalRequiresItsOwnReasonAndExplicitNullEntityRevision() {
        val v = ReviewResolution.create(value().copy(outcome = "REFUSED", entityRevision = null))
        assertEquals("REFUSED", ReviewResolution.parse(v.canonicalBytes).snapshot().outcome)
        assertThrows(SyncProtocolException::class.java) { ReviewResolution.create(value().copy(outcome = "REFUSED", entityRevision = null, reason = "")) }
        assertThrows(SyncProtocolException::class.java) { ReviewResolution.create(value().copy(outcome = "REJECTED")) }
    }
    @Test fun unicodeBoundsAndReceiptMustRemainExact() {
        assertEquals("😀".repeat(280), ReviewResolution.create(value().copy(reason = "😀".repeat(280))).snapshot().reason)
        for (bad in listOf(value().copy(reason = "😀".repeat(281)), value().copy(reason = "a\nb"), value().copy(reason = "\uD800"),
            value().copy(resolvedAt = "0001-01-01T00:00:00Z"), value().copy(resolvedAt = "2026-11-14T08:13:14-04:00"), value().copy(reviewRevision = 1))) {
            assertThrows(SyncProtocolException::class.java) { ReviewResolution.create(bad) }
        }
    }
    @Test fun requestCapturesOriginalSelectorsAndExactSourceWithoutProofLeak() {
        val selectors = mutableListOf(ReviewSelector(id(101), digest))
        val request = ReviewResultsRequest.create(input(selectors))
        selectors.clear()
        assertEquals(1, request.snapshot().selectors.size)
        request.validateSource(source())
        assertArrayEquals(request.canonicalBytes, ReviewResultsRequest.parse(request.canonicalBytes).canonicalBytes)
        assertFalse(request.toString().contains("private-"))
        assertFalse(request.snapshot().toString().contains("private-"))
        assertThrows(SyncProtocolException::class.java) { request.validateSource(ReviewSourceBinding(source().binding, id(3), id(99))) }
    }
    @Test fun completeResponseBindsOriginalReviewBeforePublishingAnySlots() {
        val selector = ReviewSelector(id(101), digest)
        val request = ReviewResultsRequest.create(input())
        val response = ReviewResultsResponse.create(request, source(), listOf(ReviewResult(selector, ReviewResolution.create(value()))))
        assertEquals(1, ReviewResultsResponse.parse(response.canonicalBytes, request, source(), listOf(ReviewExpectation(selector, id(102)))).results.size)
        assertThrows(SyncProtocolException::class.java) { ReviewResultsResponse.parse(response.canonicalBytes, request, source(), listOf(ReviewExpectation(selector, id(999)))) }
    }
    @Test fun fiftyUnicodeOutcomesUseTheirOwnBoundWithoutChangingV1() {
        val selectors = (1..50).map { ReviewSelector(id(it), digest) }
        val request = ReviewResultsRequest.create(input(selectors))
        val values = selectors.map { ReviewResult(it, ReviewResolution.create(value().copy(operationId = it.operationId, reason = "😀".repeat(280)))) }
        val response = ReviewResultsResponse.create(request, source(), values)
        assertTrue(response.canonicalBytes.size > 65536)
        assertTrue(response.canonicalBytes.size <= ReviewResultsResponse.MAX_BYTES)
        assertEquals(50, ReviewResultsResponse.parse(response.canonicalBytes, request, source(), selectors.map { ReviewExpectation(it, id(102)) }).results.size)
        assertThrows(SyncProtocolException::class.java) { CanonicalJson.parse(response.canonicalBytes) }
    }
}
