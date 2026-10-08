package com.anpfuel.app.capture
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Test
class ReleaseDevelopmentCaptureTest {
    @Test fun `developer capture availability is compiled off in release`() { assertFalse(DeveloperCaptureMode.available) }
}
