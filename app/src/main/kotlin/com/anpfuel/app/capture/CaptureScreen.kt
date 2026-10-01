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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P10-T04 minimal capture/confirmation screen behind the capture flag.
 *
 * P10-T08 i18n: every user-visible copy comes from resources (en +
 * seven locales); nothing is hardcoded in English anymore.
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
                    stringResource(R.string.capture_disabled),
                    style = MaterialTheme.typography.bodyLarge,
                )
                Button(onClick = onNavigateBack) { Text(stringResource(R.string.action_back)) }
            }
            CaptureOcrUiState.PermissionDenied -> {
                Text(
                    stringResource(R.string.capture_permission_required),
                    style = MaterialTheme.typography.bodyLarge,
                )
                Button(onClick = onNavigateBack) { Text(stringResource(R.string.action_back)) }
            }
            CaptureOcrUiState.Cancelled -> {
                Text(
                    stringResource(R.string.capture_cancelled),
                    style = MaterialTheme.typography.bodyLarge,
                )
                Button(onClick = onNavigateBack) { Text(stringResource(R.string.action_back)) }
            }
            is CaptureOcrUiState.NeedsConfirmation -> {
                Text(
                    if (current.candidates.isEmpty()) {
                        stringResource(R.string.capture_no_price)
                    } else if (current.lowConfidence) {
                        stringResource(R.string.capture_low_confidence)
                    } else {
                        stringResource(R.string.capture_check_price)
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
                ) { Text(stringResource(R.string.capture_confirm_explicit)) }
            }
            is CaptureOcrUiState.Confirmed -> {
                Text(
                    stringResource(R.string.capture_confirmed_outbox, current.candidate.raw),
                    style = MaterialTheme.typography.bodyLarge,
                )
                Button(onClick = onNavigateBack) { Text(stringResource(R.string.capture_done)) }
            }
        }
    }
}
