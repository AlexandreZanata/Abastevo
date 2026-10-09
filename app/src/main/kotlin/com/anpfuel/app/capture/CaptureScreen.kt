package com.anpfuel.app.capture

import android.content.Context
import android.graphics.Bitmap
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.detectTransformGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.FuelProductIcon
import com.anpfuel.data.local.media.BoundedPhotoBitmap
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.LinearProgressIndicator
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.valueobject.FuelProduct
import java.io.ByteArrayOutputStream
import kotlin.math.roundToInt

/**
 * Photo-contribution screen: camera opens only inside the station area,
 * then the contributor reviews a large zoomable photo, crops to the
 * price board, and fills one row per fuel (blanks are not sent, crossed
 * rows are hidden). Payment conditions are intentionally not collected:
 * every row ships STANDARD. Nothing uploads from here (P10-T05 owns the
 * outbox); existing ANP routes stay untouched.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CaptureScreen(
    onNavigateBack: () -> Unit,
    viewModel: CaptureOcrViewModel = hiltViewModel(),
    stationId: String? = null,
    fuelProductWire: String? = null,
    onReturnCommunity: () -> Unit = onNavigateBack,
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val previewEnabled by viewModel.previewEnabled.collectAsStateWithLifecycle()
    val target by viewModel.target.collectAsStateWithLifecycle()
    val targetInvalid by viewModel.targetInvalid.collectAsStateWithLifecycle()
    val nearby by viewModel.nearby.collectAsStateWithLifecycle()
    val nearbyLoading by viewModel.nearbyLoading.collectAsStateWithLifecycle()
    val nearbyFailed by viewModel.nearbyFailed.collectAsStateWithLifecycle()
    val locationDenied by viewModel.locationDenied.collectAsStateWithLifecycle()
    val pickedStationId by viewModel.pickedStationId.collectAsStateWithLifecycle()
    val photoId by viewModel.photoId.collectAsStateWithLifecycle()
    val photoRefused by viewModel.photoRefused.collectAsStateWithLifecycle()
    val fuelAmounts by viewModel.fuelAmounts.collectAsStateWithLifecycle()
    val removedFuels by viewModel.removedFuels.collectAsStateWithLifecycle()
    val fuelErrors by viewModel.fuelErrors.collectAsStateWithLifecycle()
    val unassignedAmounts by viewModel.unassignedAmounts.collectAsStateWithLifecycle()
    val submit by viewModel.submit.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val files = remember(context) { PrivateCaptureFiles(context) }
    val reviewUri by viewModel.reviewUri.collectAsStateWithLifecycle()
    val gateBusy by viewModel.gateBusy.collectAsStateWithLifecycle()
    val gateFailure by viewModel.gateFailure.collectAsStateWithLifecycle()
    val processing by viewModel.processing.collectAsStateWithLifecycle()
    val conditional by viewModel.conditional.collectAsStateWithLifecycle()
    val defaultConfirmed by viewModel.defaultPriceConfirmed.collectAsStateWithLifecycle()
    val reviewPhotoUri = reviewUri?.let(Uri::parse)
    val takePicture = rememberLauncherForActivityResult(ActivityResultContracts.TakePicture()) { success ->
        val uri = viewModel.pendingUri()?.let(Uri::parse)
        viewModel.rememberPendingUri(null)
        if (success && uri != null) {
            scope.launch {
                val bytes = withContext(Dispatchers.IO) { files.read(uri, viewModel.pendingCaptureStartedAtMillis) }
                if (bytes != null) {
                    val old = viewModel.reviewUri.value?.let(Uri::parse)
                    if (viewModel.acceptCameraPhoto(bytes, "image/jpeg", uri.toString())) {
                        if (old != null && old != uri) withContext(Dispatchers.IO) { files.delete(old) }
                    } else {
                        withContext(Dispatchers.IO) { files.delete(uri) }
                        viewModel.cameraCancelled()
                    }
                } else {
                    withContext(Dispatchers.IO) { files.delete(uri) }
                    viewModel.cameraCancelled()
                }
            }
        } else {
            scope.launch { if (uri != null) withContext(Dispatchers.IO) { files.delete(uri) } }
            viewModel.cameraCancelled()
        }
    }
    val launchCamera: () -> Unit = {
        if (viewModel.beginCamera()) scope.launch {
            val uri = withContext(Dispatchers.IO) { files.create(viewModel.pendingCaptureStartedAtMillis) }
            viewModel.rememberPendingUri(uri.toString())
            try { takePicture.launch(uri) }
            catch (_: Exception) {
                withContext(Dispatchers.IO) { files.delete(uri) }
                viewModel.cameraCancelled()
            }
        }
    }
    val cameraPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (granted) launchCamera() else viewModel.onEntryPermissions(false, true)
    }
    val requestCamera: () -> Unit = {
        viewModel.authorizeCamera {
            if (androidx.core.content.ContextCompat.checkSelfPermission(context, android.Manifest.permission.CAMERA) == android.content.pm.PackageManager.PERMISSION_GRANTED) {
                launchCamera()
            } else cameraPermission.launch(android.Manifest.permission.CAMERA)
        }
    }
    val locationPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { grants ->
        val granted = grants[android.Manifest.permission.ACCESS_FINE_LOCATION] == true || grants[android.Manifest.permission.ACCESS_COARSE_LOCATION] == true
        // Camera permission follows signed proximity authorization, never screen entry.
        viewModel.onLocationPermission(granted)
        if (granted && stationId != null && reviewUri == null && viewModel.pendingUri() == null) requestCamera()
    }
    LaunchedEffect(stationId, previewEnabled) {
        viewModel.bindTarget(if (previewEnabled) null else stationId, fuelProductWire)
        withContext(Dispatchers.IO) { files.sweep() }
        if (reviewPhotoUri != null && !withContext(Dispatchers.IO) { files.exists(reviewPhotoUri, viewModel.originalCapturedAtMillis) }) {
            viewModel.rememberReviewUri(null)
        }
        if (!previewEnabled && reviewPhotoUri == null && viewModel.pendingUri() == null) locationPermission.launch(arrayOf(android.Manifest.permission.ACCESS_FINE_LOCATION, android.Manifest.permission.ACCESS_COARSE_LOCATION))
    }
    AnpScaffold(
        modifier = Modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.capture_screen_title)) },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier.fillMaxSize().padding(padding)
                .verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            DeveloperCaptureMode.Controls(viewModel, requestCamera)
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
                return@Column
            }
            if (gateFailure != null) Text(stringResource(R.string.capture_authorization_required), color = MaterialTheme.colorScheme.error)
            when (state) {
                CaptureOcrUiState.Disabled -> {
                    Text(
                        stringResource(R.string.capture_disabled),
                        style = MaterialTheme.typography.bodyLarge,
                    )
                }
                CaptureOcrUiState.PermissionDenied -> {
                    Text(
                        stringResource(R.string.capture_permission_required),
                        style = MaterialTheme.typography.bodyLarge,
                    )
                    Button(onClick = requestCamera, enabled = !gateBusy) { Text(stringResource(R.string.capture_take_photo)) }
                }
                CaptureOcrUiState.Cancelled -> {
                    Text(
                        stringResource(R.string.capture_cancelled),
                        style = MaterialTheme.typography.bodyLarge,
                    )
                    Button(onClick = requestCamera, enabled = !gateBusy) { Text(stringResource(R.string.capture_take_photo)) }
                }
                else -> {
                    val relevantId = target?.stationId ?: pickedStationId
                    val distance = nearby.firstOrNull { it.station.stationId == relevantId }?.distanceMeters
                    val inside = CaptureOcrViewModel.isInsideStationArea(distance)
                    val photoUri = reviewPhotoUri
                    if (photoUri == null) {
                        if (previewEnabled) {
                            Button(onClick = requestCamera, enabled = pickedStationId != null && !gateBusy,
                                modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.capture_take_photo)) }
                            if (gateBusy) LinearProgressIndicator(Modifier.fillMaxWidth())
                            photoRefused?.let { Text(stringResource(R.string.capture_recognition_unavailable)) }
                        } else StationGateContent(
                            targetStationId = target?.stationId,
                            nearby = nearby,
                            nearbyLoading = nearbyLoading,
                            nearbyFailed = nearbyFailed,
                            locationDenied = locationDenied,
                            pickedStationId = pickedStationId,
                            distanceMeters = distance,
                            insideArea = inside,
                            photoRefused = photoRefused,
                            gateBusy = gateBusy,
                            gateFailure = gateFailure,
                            onPickStation = viewModel::pickStation,
                            onTakePhoto = requestCamera,
                        )
                    } else {
                        PhotoReviewContent(
                            photoUri = photoUri,
                            fuelAmounts = fuelAmounts,
                            removedFuels = removedFuels,
                            fuelErrors = fuelErrors,
                            unassigned = unassignedAmounts,
                            photoAttached = photoId != null,
                            photoRefused = photoRefused,
                            submit = submit,
                            onAmount = viewModel::setFuelAmount,
                            onRemoveFuel = viewModel::removeFuel,
                            onAddFuel = viewModel::addFuel,
                            onAssignUnassigned = viewModel::assignUnassigned,
                            onDismissUnassigned = viewModel::dismissUnassigned,
                            processing = processing,
                            conditional = conditional,
                            defaultPriceConfirmed = defaultConfirmed,
                            onConfirmDefault = viewModel::confirmDefaultPrice,
                            onReanalyze = {
                                scope.launch {
                                    val bytes = withContext(Dispatchers.IO) { files.read(photoUri, viewModel.originalCapturedAtMillis) }
                                    if (bytes != null) viewModel.replacePhoto(bytes, "image/jpeg")
                                }
                            },
                            onCrop = { bytes -> viewModel.replacePhoto(bytes, "image/jpeg") },
                            onRetake = requestCamera,
                            onNavigateBack = onReturnCommunity,
                            onSubmit = viewModel::submitContributions,
                        )
                    }
                }
            }
        }
    }
}

/**
 * Station-area gate: the camera opens only when the contributor is
 * inside the minimum station radius. Outside it (or without location)
 * the screen shows what to do instead of opening the camera.
 */
@Composable
private fun StationGateContent(
    targetStationId: String?,
    nearby: List<NearbyServerStation>,
    nearbyLoading: Boolean,
    nearbyFailed: Boolean,
    locationDenied: Boolean,
    pickedStationId: String?,
    distanceMeters: Double?,
    insideArea: Boolean,
    photoRefused: String?,
    gateBusy: Boolean,
    gateFailure: String?,
    onPickStation: (String?) -> Unit,
    onTakePhoto: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        if (targetStationId == null) {
            NearbyStationPicker(
                nearby = nearby,
                nearbyLoading = nearbyLoading,
                nearbyFailed = nearbyFailed,
                locationDenied = locationDenied,
                pickedStationId = pickedStationId,
                onPickStation = onPickStation,
            )
        }
        Card(
            modifier = Modifier.fillMaxWidth(),
            colors = CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.surfaceVariant,
            ),
        ) {
            Column(
                modifier = Modifier.padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    text = stringResource(R.string.capture_area_title),
                    style = MaterialTheme.typography.titleSmall,
                )
                when {
                    nearbyLoading -> Text(
                        text = stringResource(R.string.capture_nearby_loading),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    locationDenied || (nearby.isEmpty() && !nearbyFailed) -> Text(
                        text = stringResource(
                            R.string.capture_area_nolocation,
                            CaptureOcrViewModel.MIN_STATION_AREA_METERS.roundToInt(),
                        ),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    nearbyFailed -> Text(
                        text = stringResource(R.string.capture_nearby_failed),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                    pickedStationId == null && targetStationId == null -> Text(
                        text = stringResource(R.string.capture_pick_station),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    insideArea -> Text(
                        text = stringResource(
                            R.string.capture_area_ok,
                            distanceMeters?.roundToInt() ?: 0,
                        ),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    else -> Text(
                        text = stringResource(
                            R.string.capture_area_far,
                            distanceMeters?.roundToInt() ?: 0,
                            CaptureOcrViewModel.MIN_STATION_AREA_METERS.roundToInt(),
                        ),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        }
        Button(
            onClick = onTakePhoto,
            enabled = !gateBusy && !locationDenied && (targetStationId != null || pickedStationId != null),
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(stringResource(R.string.capture_take_photo))
        }
        if (gateBusy) LinearProgressIndicator(Modifier.fillMaxWidth())
        if (gateFailure != null) Text(stringResource(R.string.capture_authorization_required), color = MaterialTheme.colorScheme.error)
        photoRefused?.let { code ->
            Text(
                text = stringResource(R.string.capture_recognition_unavailable),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
            )
        }
    }
}

@Composable
private fun NearbyStationPicker(
    nearby: List<NearbyServerStation>,
    nearbyLoading: Boolean,
    nearbyFailed: Boolean,
    locationDenied: Boolean,
    pickedStationId: String?,
    onPickStation: (String?) -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
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
                    val inside = CaptureOcrViewModel.isInsideStationArea(row.distanceMeters)
                    Row(
                        modifier = Modifier.fillMaxWidth()
                            .selectable(
                                selected = selected,
                                onClick = { onPickStation(row.station.stationId) },
                                role = Role.RadioButton,
                            )
                            .padding(vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        RadioButton(selected = selected, onClick = null)
                        Column(modifier = Modifier.weight(1f)) {
                            Text(text = row.station.displayName, style = MaterialTheme.typography.bodyMedium)
                            Text(
                                text = stringResource(
                                    if (inside) {
                                        R.string.capture_area_row_inside
                                    } else {
                                        R.string.capture_nearby_distance
                                    },
                                    row.distanceMeters.toInt(),
                                ),
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

/**
 * Photo review: the shot large with a fixed crop frame (pre-processing
 * before the outbox), then one row per fuel with the icon, a price
 * input and a remove cross. Blank rows are not sent.
 */
@Composable
internal fun PhotoReviewContent(
    photoUri: Uri,
    fuelAmounts: Map<FuelProduct, String>,
    removedFuels: Set<FuelProduct>,
    fuelErrors: Set<FuelProduct>,
    unassigned: List<String>,
    photoAttached: Boolean,
    photoRefused: String?,
    submit: CaptureOcrViewModel.SubmitState?,
    onAmount: (FuelProduct, String) -> Unit,
    onRemoveFuel: (FuelProduct) -> Unit,
    onAddFuel: (FuelProduct) -> Unit,
    onAssignUnassigned: (Int, FuelProduct) -> Unit,
    onDismissUnassigned: (Int) -> Unit,
    processing: Boolean,
    conditional: Boolean,
    defaultPriceConfirmed: Boolean,
    onConfirmDefault: (Boolean) -> Unit,
    onReanalyze: () -> Unit,
    onRetake: () -> Unit,
    onNavigateBack: () -> Unit,
    onCrop: (ByteArray) -> Unit,
    onSubmit: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val fuels = FuelProduct.entries.filter { it in fuelAmounts && it !in removedFuels }
    var addingFuel by remember { mutableStateOf(false) }
    var assigningIndex by remember { mutableStateOf<Int?>(null) }
    val filled = fuels.count { fuelAmounts[it].orEmpty().isNotBlank() }
    val editable = !processing && submit !is CaptureOcrViewModel.SubmitState.Queued && submit !is CaptureOcrViewModel.SubmitState.Submitting && submit !is CaptureOcrViewModel.SubmitState.Sent
    PhotoSubmissionFeedback(submit, onNavigateBack)
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        StaticCropPhoto(photoUri = photoUri, onCrop = onCrop, enabled = editable)
        if (processing) {
            LinearProgressIndicator(Modifier.fillMaxWidth())
            Text(stringResource(R.string.capture_recognizing))
        }
        OutlinedButton(onClick = onReanalyze, enabled = editable, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.capture_reanalyze))
        }
        TextButton(onClick = onRetake, enabled = !processing && submit !is CaptureOcrViewModel.SubmitState.Submitting) { Text(stringResource(R.string.capture_retake)) }
        if (photoAttached) {
            Text(
                text = stringResource(R.string.capture_photo_attached),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        photoRefused?.let { code ->
            Text(
                text = stringResource(R.string.capture_recognition_unavailable),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
            )
        }
        Text(
            text = stringResource(R.string.capture_review_hint),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        fuels.forEach { product ->
            val raw = fuelAmounts[product].orEmpty()
            val error = product in fuelErrors
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                FuelProductIcon(product, size = 24.dp, contentDescription = null)
                OutlinedTextField(
                    value = raw,
                    onValueChange = { onAmount(product, it) },
                    label = { Text(stringResource(FuelProductI18n.toStringRes(product))) },
                    placeholder = { Text("0,00") },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                    singleLine = true,
                    enabled = editable,
                    isError = error,
                    supportingText = if (error) {
                        { Text(stringResource(R.string.capture_fuel_invalid)) }
                    } else {
                        null
                    },
                    modifier = Modifier.weight(1f),
                )
                IconButton(onClick = { onRemoveFuel(product) }, enabled = editable) {
                    Icon(Icons.Default.Close, contentDescription = stringResource(R.string.capture_remove_fuel))
                }
            }
        }
        Box {
            TextButton(onClick = { addingFuel = true }, enabled = editable) {
                Text(stringResource(R.string.capture_add_fuel))
            }
            DropdownMenu(expanded = addingFuel, onDismissRequest = { addingFuel = false }) {
                FuelProduct.entries.filter { it !in fuels }.forEach { product ->
                    DropdownMenuItem(text = { Text(stringResource(FuelProductI18n.toStringRes(product))) },
                        onClick = { addingFuel = false; onAddFuel(product) })
                }
            }
        }
        unassigned.forEachIndexed { index, amount ->
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    text = amount,
                    style = MaterialTheme.typography.bodyLarge,
                    modifier = Modifier.weight(1f),
                )
                Box {
                    OutlinedButton(
                        onClick = { assigningIndex = index },
                        enabled = editable && FuelProduct.entries.any { it !in fuels },
                    ) {
                        Text(stringResource(R.string.capture_choose_fuel))
                    }
                    DropdownMenu(expanded = assigningIndex == index, onDismissRequest = { assigningIndex = null }) {
                        FuelProduct.entries.filter { it !in fuels }.forEach { product ->
                            DropdownMenuItem(
                                text = { Text(stringResource(FuelProductI18n.toStringRes(product))) },
                                onClick = { assigningIndex = null; onAssignUnassigned(index, product) },
                            )
                        }
                    }
                }
                IconButton(onClick = { onDismissUnassigned(index) }, enabled = editable) {
                    Icon(Icons.Default.Close, contentDescription = stringResource(R.string.capture_remove_fuel))
                }
            }
        }
        if (fuels.isEmpty() && !processing) Text(stringResource(R.string.capture_no_recognition))
        if (conditional) {
            Text(stringResource(R.string.capture_conditional_warning), color = MaterialTheme.colorScheme.error)
            Row(verticalAlignment = Alignment.CenterVertically) {
                Checkbox(checked = defaultPriceConfirmed, onCheckedChange = onConfirmDefault)
                Text(stringResource(R.string.capture_default_confirmation), modifier = Modifier.weight(1f))
            }
        }
        Button(onClick = onSubmit, enabled = filled > 0 && editable && (!conditional || defaultPriceConfirmed), modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.capture_send_prices, filled))
        }
        when (submit) {
            CaptureOcrViewModel.SubmitState.Submitting -> Text(stringResource(R.string.capture_saving))
            is CaptureOcrViewModel.SubmitState.Queued ->
                Text(
                    text = stringResource(R.string.capture_sent_count, submit.count),
                    style = MaterialTheme.typography.bodyMedium,
                )
            is CaptureOcrViewModel.SubmitState.Sent -> {
                Text(
                    text = stringResource(R.string.capture_sent, submit.count),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.primary,
                )
                Button(onClick = onNavigateBack, modifier = Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.capture_back_community))
                }
            }
            is CaptureOcrViewModel.SubmitState.Partial ->
                Text(
                    text = stringResource(R.string.capture_partial, submit.sent, submit.reason),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                )
            is CaptureOcrViewModel.SubmitState.Failed ->
                Text(
                    text = stringResource(R.string.capture_submit_failed, stringResource(R.string.capture_retry_guidance)),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                )
            is CaptureOcrViewModel.SubmitState.NoTarget ->
                Text(
                    text = stringResource(R.string.capture_pick_station),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            is CaptureOcrViewModel.SubmitState.FuelMismatch ->
                Text(
                    text = stringResource(R.string.capture_target_invalid),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
            is CaptureOcrViewModel.SubmitState.Disabled -> Text(stringResource(R.string.capture_disabled))
            null -> Unit
        }
    }
}

/**
 * Photo with a fixed crop frame and pinch-to-zoom positioning. The user
 * zooms (1-5x) and drags to frame the price board, double-tap resets,
 * and "Crop to frame" maps the frame onto the original bitmap for
 * recognition/upload — then the view returns to the static full photo
 * on its own, so zoom can never get stuck.
 */
@Composable
internal fun StaticCropPhoto(
    photoUri: Uri,
    onCrop: (ByteArray) -> Unit,
    enabled: Boolean = true,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    var bitmap by remember(photoUri) { mutableStateOf<Bitmap?>(null) }
    LaunchedEffect(photoUri) {
        bitmap = withContext(Dispatchers.IO) { decodeSampled(context, photoUri, 1600) }
    }
    val cropScope = rememberCoroutineScope()
    var cropping by remember { mutableStateOf(false) }
    var scale by remember(photoUri) { mutableStateOf(1f) }
    var offset by remember(photoUri) { mutableStateOf(Offset.Zero) }
    var containerSize by remember { mutableStateOf(IntSize.Zero) }
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Box(
            modifier = Modifier.fillMaxWidth().aspectRatio(4f / 3f)
                .clip(RoundedCornerShape(12.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant)
                .onSizeChanged { containerSize = it }
                .pointerInput(photoUri) {
                    detectTransformGestures { _, pan, zoom, _ ->
                        val next = (scale * zoom).coerceIn(1f, 5f)
                        scale = next
                        offset = if (next <= 1f) {
                            Offset.Zero
                        } else {
                            val maxX = (size.width * next - size.width) / 2f
                            val maxY = (size.height * next - size.height) / 2f
                            Offset(
                                (offset.x + pan.x).coerceIn(-maxX, maxX),
                                (offset.y + pan.y).coerceIn(-maxY, maxY),
                            )
                        }
                    }
                }
                .pointerInput(photoUri) {
                    detectTapGestures(onDoubleTap = {
                        scale = 1f
                        offset = Offset.Zero
                    })
                },
            contentAlignment = Alignment.Center,
        ) {
            bitmap?.let {
                Image(
                    bitmap = it.asImageBitmap(),
                    contentDescription = stringResource(R.string.capture_photo_content),
                    contentScale = ContentScale.Fit,
                    modifier = Modifier.fillMaxSize()
                        .graphicsLayer(scaleX = scale, scaleY = scale, translationX = offset.x, translationY = offset.y),
                )
            }
            // Fixed crop frame: 85% width, 4:3, centered (matches cropToFrame).
            Box(
                modifier = Modifier.fillMaxSize(),
                contentAlignment = Alignment.Center,
            ) {
                Box(
                    modifier = Modifier.fillMaxWidth(0.85f).aspectRatio(4f / 3f)
                        .border(2.dp, MaterialTheme.colorScheme.primary, RoundedCornerShape(8.dp)),
                )
            }
        }
        OutlinedButton(
            onClick = {
                val bmp = bitmap ?: return@OutlinedButton
                val cw = containerSize.width.toFloat()
                val ch = containerSize.height.toFloat()
                if (cw <= 0f || ch <= 0f) return@OutlinedButton
                cropping = true
                val frozenScale = scale
                val frozenOffset = offset
                cropScope.launch {
                    try {
                        val bytes = withContext(Dispatchers.Default) { cropToFrame(bmp, cw, ch, frozenScale, frozenOffset) }
                        if (bytes != null) {
                            onCrop(bytes)
                            scale = 1f
                            offset = Offset.Zero
                        }
                    } finally { cropping = false }
                }
            },
            enabled = enabled && !cropping && bitmap != null,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(stringResource(R.string.capture_crop))
        }
    }
}

private fun decodeSampled(context: Context, uri: Uri, maxSide: Int): Bitmap? = try {
    // Only the app's private provider can be read; external content is not accepted here.
    if (uri.authority != context.packageName + ".fileprovider") null else {
        val bytes = PrivateCaptureFiles(context).readReview(uri)
        bytes?.let { BoundedPhotoBitmap.decode(it, maxSide) }
    }
} catch (_: Exception) { null }

private fun cropToFrame(
    bitmap: Bitmap,
    containerW: Float,
    containerH: Float,
    scale: Float,
    offset: Offset,
): ByteArray? {
    val bw = bitmap.width.toFloat()
    val bh = bitmap.height.toFloat()
    if (bw <= 0f || bh <= 0f) return null
    val fit = minOf(containerW / bw, containerH / bh)
    if (fit <= 0f) return null
    fun toBitmapX(px: Float): Float = (px - containerW / 2f - offset.x) / (fit * scale) + bw / 2f
    fun toBitmapY(py: Float): Float = (py - containerH / 2f - offset.y) / (fit * scale) + bh / 2f
    // Frame matches the overlay: 85% width, centered, 4:3 aspect.
    val frameW = containerW * 0.85f
    val frameH = frameW * 3f / 4f
    val left = ((toBitmapX(containerW / 2f - frameW / 2f)).coerceIn(0f, bw - 1f)).roundToInt()
    val top = ((toBitmapY(containerH / 2f - frameH / 2f)).coerceIn(0f, bh - 1f)).roundToInt()
    val right = ((toBitmapX(containerW / 2f + frameW / 2f)).coerceIn(0f, bw)).roundToInt()
    val bottom = ((toBitmapY(containerH / 2f + frameH / 2f)).coerceIn(0f, bh)).roundToInt()
    val width = (right - left).coerceAtLeast(0)
    val height = (bottom - top).coerceAtLeast(0)
    if (width < 120 || height < 120) return null
    return try {
        val cropped = Bitmap.createBitmap(bitmap, left, top, width, height)
        val out = ByteArrayOutputStream()
        cropped.compress(Bitmap.CompressFormat.JPEG, 92, out)
        if (cropped !== bitmap) cropped.recycle()
        out.toByteArray()
    } catch (_: Exception) {
        null
    }
}
