package com.anpfuel.app.capture

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.community.CommunityPriceDisplay
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.portable.PortablePriceOcr
import com.anpfuel.domain.valueobject.FuelProduct
import java.io.File

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
    stationId: String? = null,
    fuelProductWire: String? = null,
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val target by viewModel.target.collectAsStateWithLifecycle()
    val targetInvalid by viewModel.targetInvalid.collectAsStateWithLifecycle()
    val nearby by viewModel.nearby.collectAsStateWithLifecycle()
    val nearbyLoading by viewModel.nearbyLoading.collectAsStateWithLifecycle()
    val nearbyFailed by viewModel.nearbyFailed.collectAsStateWithLifecycle()
    val locationDenied by viewModel.locationDenied.collectAsStateWithLifecycle()
    val pickedStationId by viewModel.pickedStationId.collectAsStateWithLifecycle()
    val photoId by viewModel.photoId.collectAsStateWithLifecycle()
    val photoRefused by viewModel.photoRefused.collectAsStateWithLifecycle()
    androidx.compose.runtime.LaunchedEffect(stationId, fuelProductWire) {
        viewModel.bindTarget(stationId, fuelProductWire)
    }
    val context = LocalContext.current
    var pendingPhotoUri by remember { mutableStateOf<android.net.Uri?>(null) }
    val takePicture = rememberLauncherForActivityResult(ActivityResultContracts.TakePicture()) { success ->
        val uri = pendingPhotoUri
        if (success && uri != null) {
            val bytes = runCatching {
                context.contentResolver.openInputStream(uri)?.use { it.readBytes() }
            }.getOrNull()
            if (bytes != null) viewModel.preparePhoto(bytes, "image/jpeg")
            else viewModel.onCaptureResult(cancelled = true, ocrText = null)
        } else {
            viewModel.onCaptureResult(cancelled = true, ocrText = null)
        }
    }
    val permissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions(),
    ) { grants ->
        viewModel.onEntryPermissions(
            cameraGranted = grants[android.Manifest.permission.CAMERA] == true,
            locationGranted = grants[android.Manifest.permission.ACCESS_FINE_LOCATION] == true ||
                grants[android.Manifest.permission.ACCESS_COARSE_LOCATION] == true,
        )
    }
    LaunchedEffect(Unit) {
        permissionLauncher.launch(
            arrayOf(
                android.Manifest.permission.CAMERA,
                android.Manifest.permission.ACCESS_FINE_LOCATION,
                android.Manifest.permission.ACCESS_COARSE_LOCATION,
            ),
        )
    }
    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        CaptureEntryContent(
            nearby = nearby,
            nearbyLoading = nearbyLoading,
            nearbyFailed = nearbyFailed,
            locationDenied = locationDenied,
            pickedStationId = pickedStationId,
            photoAttached = photoId != null,
            photoRefused = photoRefused,
            onPickStation = viewModel::pickStation,
            onTakePhoto = {
                val dir = File(context.cacheDir, "capture").apply { mkdirs() }
                val file = File.createTempFile("price_", ".jpg", dir)
                val uri = FileProvider.getUriForFile(context, context.packageName + ".fileprovider", file)
                pendingPhotoUri = uri
                takePicture.launch(uri)
            },
        )
        target?.let {
            Text(
                text = stringResource(
                    R.string.capture_target_label,
                    it.stationId.take(8),
                    it.fuelProductWire,
                ),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (targetInvalid) {
            Text(
                text = stringResource(R.string.capture_target_invalid),
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.error,
            )
            Button(onClick = onNavigateBack) { Text(stringResource(R.string.action_back)) }
            return@Column
        }
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
                val submit by viewModel.submit.collectAsStateWithLifecycle()
                when (val sent = submit) {
                    is CaptureOcrViewModel.SubmitState.Queued ->
                        Text(
                            text = stringResource(R.string.capture_submitted),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                    is CaptureOcrViewModel.SubmitState.Failed ->
                        Text(
                            text = stringResource(R.string.capture_submit_failed, sent.reason),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                        )
                    is CaptureOcrViewModel.SubmitState.NoTarget ->
                        Text(
                            text = stringResource(R.string.capture_target_invalid),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                        )
                    is CaptureOcrViewModel.SubmitState.FuelMismatch ->
                        Text(
                            text = stringResource(R.string.capture_target_invalid),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                        )
                    is CaptureOcrViewModel.SubmitState.Disabled, null -> Unit
                }
                Button(onClick = { viewModel.submitConfirmed() }, enabled = target != null || pickedStationId != null) {
                    Text(stringResource(R.string.capture_submit))
                }
                if (target == null && pickedStationId == null) {
                    Text(
                        text = stringResource(R.string.capture_pick_station),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Button(onClick = onNavigateBack) { Text(stringResource(R.string.capture_done)) }
            }
        }
    }
}

private val REVIEW_CONDITIONS = listOf(
    "STANDARD", "CASH", "DEBIT", "CREDIT", "APP", "LOYALTY", "OTHER",
)

/**
 * Camera-module entry: one explicit photo shot plus the geolocated
 * station picker. The shot is compressed through the KiB-bounded
 * PhotoFlow; review stays human (manual entry until on-device OCR text
 * exists). Picking a station binds the submit target for the
 * context-free Community entry.
 */
@Composable
private fun CaptureEntryContent(
    nearby: List<NearbyServerStation>,
    nearbyLoading: Boolean,
    nearbyFailed: Boolean,
    locationDenied: Boolean,
    pickedStationId: String?,
    photoAttached: Boolean,
    photoRefused: String?,
    onPickStation: (String?) -> Unit,
    onTakePhoto: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        OutlinedButton(onClick = onTakePhoto, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.capture_take_photo))
        }
        if (photoAttached) {
            Text(
                text = stringResource(R.string.capture_photo_attached),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        photoRefused?.let { code ->
            Text(
                text = stringResource(R.string.capture_photo_refused, code),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
            )
        }
        Text(
            text = stringResource(R.string.capture_pick_station),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        when {
            nearbyLoading -> Text(
                text = stringResource(R.string.capture_nearby_loading),
                style = MaterialTheme.typography.bodySmall,
            )
            locationDenied -> Text(
                text = stringResource(R.string.capture_location_needed),
                style = MaterialTheme.typography.bodySmall,
            )
            nearbyFailed -> Text(
                text = stringResource(R.string.capture_nearby_failed),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
            )
            nearby.isEmpty() -> Text(
                text = stringResource(R.string.capture_nearby_empty),
                style = MaterialTheme.typography.bodySmall,
            )
            else -> LazyColumn(modifier = Modifier.fillMaxWidth().heightIn(max = 220.dp)) {
                items(nearby, key = { it.station.stationId }) { row ->
                    val selected = row.station.stationId == pickedStationId
                    Row(
                        modifier = Modifier.fillMaxWidth()
                            .selectable(selected = selected, onClick = { onPickStation(row.station.stationId) }, role = Role.RadioButton)
                            .padding(vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        RadioButton(selected = selected, onClick = null)
                        Column(modifier = Modifier.weight(1f)) {
                            Text(text = row.station.displayName, style = MaterialTheme.typography.bodyMedium)
                            Text(
                                text = stringResource(R.string.capture_nearby_distance, row.distanceMeters.toInt()),
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                }
            }
        }
    }
}

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
