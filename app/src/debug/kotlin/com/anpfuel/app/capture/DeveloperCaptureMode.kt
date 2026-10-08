package com.anpfuel.app.capture

import androidx.compose.runtime.Composable

internal object DeveloperCaptureMode {
    const val available = true
    @Composable fun Controls(viewModel: CaptureOcrViewModel, onGallery: () -> Unit) {
        DeveloperCaptureControls(viewModel, onGallery)
    }
}
