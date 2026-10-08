package com.anpfuel.data.remote
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
class ReleaseDevelopmentPhotoCaptureTest {
    @Test fun `release cannot create development request even for staging`() {
        assertFalse(DevelopmentPhotoCapture.available)
        assertThrows(UnsupportedOperationException::class.java) { DevelopmentPhotoCapture.request(ApiEnvironment.STAGING.origin,"station","capture") }
    }
}
