package com.anpfuel.app.capture

import androidx.compose.runtime.Composable

internal object DeveloperCaptureMode {
    const val available = false
    @Composable fun Controls(viewModel: CaptureOcrViewModel, onGallery: () -> Unit) {
    }
}
