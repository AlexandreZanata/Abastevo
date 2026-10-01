package com.anpfuel.app.capture

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P10-T04 minimal capture/confirmation screen behind the capture flag.
 *
 * No CameraX preview is bundled in this slice: capture comes from the
 * system camera intent and compression reuses PhotoFlow budgets; this
 * screen only explains states and confirms price + product/condition
 * with the contributor. Nothing uploads from here (P10-T05 owns the
 * outbox); existing ANP routes stay untouched.
 */
@Composable
fun CaptureScreen(
    onNavigateBack: () -> Unit,
    viewModel: CaptureOcrViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        when (val current = state) {
            CaptureOcrUiState.Disabled -> {
                Text(
                    "Community capture is disabled.",
                    style = MaterialTheme.typography.bodyLarge,
                )
                Button(onClick = onNavigateBack) { Text("Back") }
            }
            CaptureOcrUiState.PermissionDenied -> {
                Text(
                    "Camera permission is required for capture. " +
                        "You can keep contributing without a photo.",
                    style = MaterialTheme.typography.bodyLarge,
                )
                Button(onClick = onNavigateBack) { Text("Back") }
            }
            CaptureOcrUiState.Cancelled -> {
                Text(
                    "Capture cancelled. Nothing was saved.",
                    style = MaterialTheme.typography.bodyLarge,
                )
                Button(onClick = onNavigateBack) { Text("Back") }
            }
            is CaptureOcrUiState.NeedsConfirmation -> {
                Text(
                    if (current.candidates.isEmpty()) {
                        "No price detected. Enter the price manually."
                    } else if (current.lowConfidence) {
                        "Low OCR confidence. Check the price and pick the fuel."
                    } else {
                        "Check the detected price and pick the fuel."
                    },
                    style = MaterialTheme.typography.bodyLarge,
                )
                current.candidates.forEach { candidate ->
                    Text("${candidate.raw} (${candidate.priceMilli} milli-BRL)")
                }
                Button(
                    onClick = {
                        val first = current.candidates.firstOrNull()
                        viewModel.onConfirm(
                            first,
                            FuelProduct.GASOLINE_REGULAR,
                            humanConfirmed = true,
                        )
                    },
                ) { Text("Confirm (explicit)") }
            }
            is CaptureOcrUiState.Confirmed -> {
                Text(
                    "Price confirmed: ${current.candidate.raw}. " +
                        "Upload happens only from the outbox (P10-T05).",
                    style = MaterialTheme.typography.bodyLarge,
                )
                Button(onClick = onNavigateBack) { Text("Done") }
            }
        }
    }
}
