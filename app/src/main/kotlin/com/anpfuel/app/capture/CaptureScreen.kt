package com.anpfuel.app.capture

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.community.CommunityPriceDisplay
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.domain.portable.PortablePriceOcr
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P10-T04 minimal capture/confirmation screen behind the capture flag.
 *
 * P21-T02 editable review: the contributor selects one OCR candidate (or
 * types the price manually for empty/low-confidence sets), picks fuel and
 * condition explicitly, and confirms with one explicit tap. Nothing
 * auto-confirms, nothing picks a minimum, and nothing uploads from here
 * (P10-T05 owns the outbox); existing ANP routes stay untouched.
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
                ReviewContent(
                    state = current,
                    onConfirmCandidate = { candidate, product, condition ->
                        viewModel.onConfirm(candidate, product, condition, true)
                    },
                    onConfirmManual = { rawPrice, product, condition ->
                        viewModel.onConfirmManual(rawPrice, product, condition, true)
                    },
                )
                Button(onClick = onNavigateBack) { Text(stringResource(R.string.action_back)) }
            }
            is CaptureOcrUiState.Confirmed -> {
                Text(
                    stringResource(R.string.capture_confirmed_outbox, current.candidate.raw),
                    style = MaterialTheme.typography.bodyLarge,
                )
                Text(
                    text = stringResource(FuelProductI18n.toStringRes(current.product)) +
                        " · " + current.conditionKind,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Button(onClick = onNavigateBack) { Text(stringResource(R.string.capture_done)) }
            }
        }
    }
}

private val REVIEW_CONDITIONS = listOf(
    "STANDARD", "CASH", "DEBIT", "CREDIT", "APP", "LOYALTY", "OTHER",
)

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ReviewContent(
    state: CaptureOcrUiState.NeedsConfirmation,
    onConfirmCandidate: (
        candidate: PortablePriceOcr.OcrCandidate?,
        product: FuelProduct?,
        condition: String?,
    ) -> Unit,
    onConfirmManual: (rawPrice: String, product: FuelProduct?, condition: String?) -> Unit,
    modifier: Modifier = Modifier,
) {
    var selectedIndex by remember(state.candidates) { mutableIntStateOf(0) }
    var selectedProduct by remember { mutableStateOf<FuelProduct?>(null) }
    var selectedCondition by remember { mutableStateOf<String?>(null) }
    var manualPrice by remember(state.candidates) { mutableStateOf("") }
    val showManualEntry = state.candidates.isEmpty() || state.lowConfidence
    val useManual = manualPrice.isNotBlank()
    val canConfirm = selectedProduct != null && selectedCondition != null &&
        (useManual || state.candidates.indices.contains(selectedIndex))

    Column(
        modifier = modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(
            if (state.candidates.isEmpty()) {
                stringResource(R.string.capture_no_price)
            } else if (state.lowConfidence) {
                stringResource(R.string.capture_low_confidence)
            } else {
                stringResource(R.string.capture_check_price)
            },
            style = MaterialTheme.typography.bodyLarge,
        )

        state.candidates.forEachIndexed { index, candidate ->
            val selected = index == selectedIndex && !useManual
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .selectable(
                        selected = selected,
                        onClick = { selectedIndex = index },
                        role = Role.RadioButton,
                    )
                    .padding(vertical = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                RadioButton(selected = selected, onClick = null)
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = CommunityPriceDisplay.formatMilliBrl(candidate.priceMilli),
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        text = candidate.raw,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }

        if (showManualEntry) {
            OutlinedTextField(
                value = manualPrice,
                onValueChange = { manualPrice = it },
                modifier = Modifier.fillMaxWidth(),
                label = { Text(stringResource(R.string.capture_manual_price_label)) },
                placeholder = { Text(stringResource(R.string.capture_manual_price_placeholder)) },
                singleLine = true,
            )
        }

        Text(
            text = stringResource(R.string.capture_pick_product),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            for (product in FuelProduct.entries) {
                FilterChip(
                    selected = selectedProduct == product,
                    onClick = { selectedProduct = product },
                    label = { Text(stringResource(FuelProductI18n.toStringRes(product))) },
                )
            }
        }

        Text(
            text = stringResource(R.string.capture_pick_condition),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            for (condition in REVIEW_CONDITIONS) {
                FilterChip(
                    selected = selectedCondition == condition,
                    onClick = { selectedCondition = condition },
                    label = { Text(condition) },
                )
            }
        }

        Button(
            onClick = {
                if (useManual) {
                    onConfirmManual(manualPrice, selectedProduct, selectedCondition)
                } else {
                    onConfirmCandidate(
                        state.candidates.getOrNull(selectedIndex),
                        selectedProduct,
                        selectedCondition,
                    )
                }
            },
            enabled = canConfirm,
            modifier = Modifier.fillMaxWidth(),
        ) { Text(stringResource(R.string.capture_confirm_explicit)) }
    }
}
