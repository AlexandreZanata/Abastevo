package com.anpfuel.data.remote

import io.mockk.*
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/**
 * Debug-only development-receipt behavior: the debug
 * [DevelopmentPhotoCapture] accepts owned staging without GPS, while
 * release refuses (see `ReleaseDevelopmentPhotoCaptureTest`). These
 * tests live in `testDebug` because CI runs `./gradlew test` across
 * both variants and shared `src/test` must stay variant-agnostic.
 */
class PhotoCaptureDevReceiptTest {
    private val station = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private val transport = mockk<PhotoProofTransport>()
    private val now = System.currentTimeMillis()
    private fun receipt() = JSONObject().put("capture_id", "589a13ba-1f78-41dc-a4ab-d63a511c73da")
        .put("station_id", station).put("issued_at", Instant.ofEpochMilli(now - 1000).toString())
        .put("camera_expires_at", Instant.ofEpochMilli(now + 119000).toString())
        .put("expires_at", Instant.ofEpochMilli(now + 86399000).toString()).put("policy_version", "photo-capture-v1")
    private fun prepare(response: JSONObject) {
        every { transport.origin } returns "https://example.invalid"
        every { transport.ownerScope() } returns "anonymous:synthetic"
        coEvery { transport.identityScope() } returns "anonymous:synthetic"
        coEvery { transport.post(any(), any(), any()) } returns response
    }
    @Test fun `development authorization restricted to owned staging has no GPS and distinct policy`() = runTest {
        prepare(receipt().put("policy_version", "photo-capture-ui-test-v1"))
        val client = PhotoCaptureHttpClient(transport)
        assertThrows(IllegalArgumentException::class.java) { kotlinx.coroutines.runBlocking { client.authorizeDevelopmentPreview(station, "capture") } }
        every { transport.origin } returns ApiEnvironment.STAGING.origin
        val body = slot<JSONObject>()
        coEvery { transport.post(any(), any(), capture(body)) } returns receipt().put("policy_version", "photo-capture-ui-test-v1")
        assertTrue(client.authorizeDevelopmentPreview(station, "capture").developmentPreview)
        assertTrue(body.captured.getBoolean("development_preview")); assertFalse(body.captured.has("location"))
        coEvery { transport.post(any(), any(), any()) } returns receipt()
        assertThrows(java.io.IOException::class.java) { kotlinx.coroutines.runBlocking { client.authorizeDevelopmentPreview(station, "capture") } }
    }
    @Test fun `server clock slightly ahead of phone still yields a usable development receipt`() = runTest {
        val at = System.currentTimeMillis()
        val skewed = receipt().put("policy_version", "photo-capture-ui-test-v1")
            .put("issued_at", Instant.ofEpochMilli(at + 5000).toString())
            .put("camera_expires_at", Instant.ofEpochMilli(at + 125000).toString())
            .put("expires_at", Instant.ofEpochMilli(at + 86405000).toString())
        every { transport.origin } returns ApiEnvironment.STAGING.origin
        every { transport.ownerScope() } returns "anonymous:synthetic"
        coEvery { transport.identityScope() } returns "anonymous:synthetic"
        coEvery { transport.post(any(), any(), any()) } returns skewed
        assertTrue(PhotoCaptureHttpClient(transport).authorizeDevelopmentPreview(station, "capture").developmentPreview)
        val forged = receipt().put("policy_version", "photo-capture-ui-test-v1")
            .put("issued_at", Instant.ofEpochMilli(at + 61000).toString())
            .put("camera_expires_at", Instant.ofEpochMilli(at + 181000).toString())
            .put("expires_at", Instant.ofEpochMilli(at + 86461000).toString())
        coEvery { transport.post(any(), any(), any()) } returns forged
        assertThrows(java.io.IOException::class.java) { kotlinx.coroutines.runBlocking { PhotoCaptureHttpClient(transport).authorizeDevelopmentPreview(station, "capture") } }
    }
}
