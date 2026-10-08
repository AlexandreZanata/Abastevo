package com.anpfuel.app.capture

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.port.CaptureOcrFlagProvider
import com.anpfuel.application.port.CaptureLocationSource
import com.anpfuel.application.port.PhotoCaptureGate
import com.anpfuel.application.port.PhotoCapturePermission
import com.anpfuel.application.port.ImagePriceOcr
import com.anpfuel.application.port.isEligibleForPhotoCapture
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.Job
import com.anpfuel.application.portable.PhotoFlow
import com.anpfuel.application.usecase.capture.ConfirmPriceCaptureUseCase
import com.anpfuel.application.usecase.contribution.EnqueueContributionOutcome
import com.anpfuel.application.usecase.contribution.EnqueueContributionUseCase
import com.anpfuel.application.usecase.contribution.EnqueueReviewOutcome
import com.anpfuel.application.usecase.contribution.GetOwnedContributionsUseCase
import com.anpfuel.domain.model.PhotoContributionContext
import com.anpfuel.application.usecase.directory.GetNearbyServerStationsUseCase
import com.anpfuel.application.usecase.directory.NearbyServerStationsOutcome
import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.data.remote.PhotoCaptureHttpClient
import com.anpfuel.domain.contribution.ContributionTarget
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.portable.PortablePriceOcr
import com.anpfuel.domain.valueobject.FuelProduct
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * P10-T04 capture/OCR screen state (UC-CAPTURE-01).
 *
 * Flag-gated: [UiState.Disabled] keeps existing ANP browsing unchanged.
 * [UiState.PermissionDenied] and [UiState.Cancelled] perform no OCR and
 * cache nothing. [UiState.NeedsConfirmation] presents every OCR
 * candidate in order (never an auto-picked minimum) plus a manual-entry
 * path when the set is empty or low-confidence. [UiState.Confirmed]
 * exists only after explicit human confirmation with a picked product;
 * it still uploads nothing (P10-T05 owns the outbox).
 */
sealed interface CaptureOcrUiState {
    data object Disabled : CaptureOcrUiState
    data object PermissionDenied : CaptureOcrUiState
    data object Cancelled : CaptureOcrUiState
    data class NeedsConfirmation(
        val candidates: List<PortablePriceOcr.OcrCandidate>,
        val lowConfidence: Boolean,
    ) : CaptureOcrUiState

    data class Confirmed(
        val candidate: PortablePriceOcr.OcrCandidate,
        val product: FuelProduct,
        val conditionKind: String,
    ) : CaptureOcrUiState
}

@HiltViewModel
class CaptureOcrViewModel @Inject constructor(
    private val useCase: ConfirmPriceCaptureUseCase,
    private val permissionHandler: CameraPermissionHandler,
    private val flagProvider: CaptureOcrFlagProvider,
    private val enqueue: EnqueueContributionUseCase,
    private val locationHandler: LocationPermissionHandler,
    private val nearbyStations: GetNearbyServerStationsUseCase,
    private val photoFlow: PhotoFlow,
    private val captureLocation: CaptureLocationSource,
    private val captureGate: PhotoCaptureGate,
    private val imageOcr: ImagePriceOcr,
    private val savedState: SavedStateHandle,
    private val cityFeed: com.anpfuel.application.usecase.community.GetCityCommunityFeedUseCase,
    private val stationGateway: com.anpfuel.domain.repository.ServerStationGateway,
    private val ownedContributions: GetOwnedContributionsUseCase,

) : ViewModel() {

    private val _previewEnabled = MutableStateFlow(false)
    val previewEnabled = _previewEnabled.asStateFlow()
    private val _previewCity = MutableStateFlow<com.anpfuel.domain.community.FeedCity?>(null)
    val previewCity = _previewCity.asStateFlow()
    private val _previewQuery = MutableStateFlow("")
    val previewQuery = _previewQuery.asStateFlow()
    private val _previewStations = MutableStateFlow<List<com.anpfuel.domain.discovery.ServerStation>>(emptyList())
    val previewStations = _previewStations.asStateFlow()
    private val _previewSearchBusy = MutableStateFlow(false)
    val previewSearchBusy = _previewSearchBusy.asStateFlow()
    private val _previewSearchFailed = MutableStateFlow(false)
    val previewSearchFailed = _previewSearchFailed.asStateFlow()
    private var stationSearch: Job? = null
    private var previewPhoto = savedState.get<Boolean>("previewPhoto") ?: false

    fun setPreviewEnabled(enabled: Boolean) {
        if (!DeveloperCaptureMode.available || submitting || _processing.value || _gateBusy.value || enabled == _previewEnabled.value) return
        ++generation; stationSearch?.cancel(); recognition?.cancel()
        pendingReceipt = null; receipt = null
        savedState.remove<ArrayList<String>>("pendingReceipt"); savedState.remove<ArrayList<String>>("receipt")
        _photoId.value?.takeIf { it !in queuedPhotoIds }?.let { id -> viewModelScope.launch(Dispatchers.IO) { photoFlow.discard(id) } }
        _photoId.value = null; savedState.remove<String>("photoId")
        rememberReviewUri(null); _fuelAmounts.value = emptyMap(); _removedFuels.value = emptySet(); edited.clear()
        _unassignedAmounts.value = emptyList(); savedState.remove<ArrayList<String>>("unassigned")
        FuelProduct.entries.forEach { savedState.remove<String>("amount.${it.name}"); savedState.remove<Boolean>("edited.${it.name}"); savedState.remove<Boolean>("removed.${it.name}") }
        _submit.value = null; savedState.remove<Int>("queuedCount")
        submittedIds = emptyList(); savedState.remove<ArrayList<String>>("submittedIds")
        _pickedStationId.value = null; _previewStations.value = emptyList(); _previewQuery.value = ""
        previewPhoto = false; savedState["previewPhoto"] = false
        _gateFailure.value = null; _state.value = initialState(); _previewEnabled.value = enabled
        if (enabled) viewModelScope.launch {
            _previewCity.value = runCatching { cityFeed.city() }.getOrNull()
            if (_previewEnabled.value && _reviewUri.value == null && _previewCity.value != null) {
                searchPreviewStation("")
            }
        }
    }

    /**
     * Developer station search scoped to the manually selected Home city.
     * A blank query lists every station in the city (first page); typing
     * 2+ characters filters by station name. A single character waits for
     * more input because the server requires 2..100 characters for `q`.
     */
    fun searchPreviewStation(query: String) {
        if (!DeveloperCaptureMode.available || !_previewEnabled.value || _reviewUri.value != null) return
        _previewQuery.value = query.take(100); _previewStations.value = emptyList(); pickStation(null)
        stationSearch?.cancel(); _previewSearchBusy.value = false; _previewSearchFailed.value = false
        val city = _previewCity.value ?: return
        val trimmed = query.trim()
        if (trimmed.length == 1) return
        stationSearch = viewModelScope.launch {
            _previewSearchBusy.value = true
            kotlinx.coroutines.delay(300)
            try { _previewStations.value = withContext(Dispatchers.IO) { stationGateway.search(city.code, trimmed, 20).items } }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (_: Exception) { _previewSearchFailed.value = true }
            finally { _previewSearchBusy.value = false }
        }
    }

    fun pickPreviewStation(id: String) {
        if (DeveloperCaptureMode.available && _previewEnabled.value && _previewStations.value.any { it.stationId == id }) pickStation(id)
    }

    fun acceptPreviewPhoto(bytes: ByteArray, mime: String, uri: String): Boolean {
        if (!DeveloperCaptureMode.available || !_previewEnabled.value || pendingReceipt?.developmentPreview != true) return false
        return acceptCameraPhoto(bytes, mime, uri)
    }

    private val _state = MutableStateFlow<CaptureOcrUiState>(initialState())
    val state: StateFlow<CaptureOcrUiState> = _state.asStateFlow()

    /**
     * P37-T01 — contextual capture target (canonical UUID + wire fuel).
     * Null binds no target (context-free FAB flow; manual station pick
     * stays future work). An invalid target blocks capture honestly via
     * [targetInvalid] instead of a silent misattribution.
     */
    private val _target = MutableStateFlow<ContributionTarget?>(null)
    val target: StateFlow<ContributionTarget?> = _target.asStateFlow()

    private val _targetInvalid = MutableStateFlow(false)
    val targetInvalid: StateFlow<Boolean> = _targetInvalid.asStateFlow()

    /**
     * Community contribute entry: camera + geolocation.
     * [nearby] holds server stations around the last-known fix for the
     * contributor to pick as the submit target; [pickedStationId] is the
     * explicit pick (null until the contributor taps one). [photoId] is
     * the transient PhotoFlow entry attached to the submit, if the shot
     * fit the KiB budgets; [photoRefused] carries the stable refusal code
     * when it did not (the contributor can still type the price manually).
     */
    private val _nearby = MutableStateFlow<List<NearbyServerStation>>(emptyList())
    val nearby: StateFlow<List<NearbyServerStation>> = _nearby.asStateFlow()

    private val _nearbyLoading = MutableStateFlow(false)
    val nearbyLoading: StateFlow<Boolean> = _nearbyLoading.asStateFlow()

    private val _nearbyFailed = MutableStateFlow(false)
    val nearbyFailed: StateFlow<Boolean> = _nearbyFailed.asStateFlow()

    private val _locationDenied = MutableStateFlow(false)
    val locationDenied: StateFlow<Boolean> = _locationDenied.asStateFlow()

    private val _pickedStationId = MutableStateFlow<String?>(null)
    val pickedStationId: StateFlow<String?> = _pickedStationId.asStateFlow()

    private val _photoId = MutableStateFlow<String?>(savedState["photoId"])
    val photoId: StateFlow<String?> = _photoId.asStateFlow()

    private val _photoRefused = MutableStateFlow<String?>(null)
    val photoRefused: StateFlow<String?> = _photoRefused.asStateFlow()

    /**
     * Multi-fuel report rows: one typed amount per fuel, blank means
     * that fuel is not sent. [removedFuels] hides rows the contributor
     * crossed out; [fuelErrors] marks rows that failed to parse on the
     * last submit attempt.
     */
    private val _fuelAmounts = MutableStateFlow<Map<FuelProduct, String>>(
        FuelProduct.entries.mapNotNull { product -> savedState.get<String>("amount.${product.name}")?.let { product to it } }.toMap())
    val fuelAmounts: StateFlow<Map<FuelProduct, String>> = _fuelAmounts.asStateFlow()

    private val _removedFuels = MutableStateFlow<Set<FuelProduct>>(
        FuelProduct.entries.filter { savedState.get<Boolean>("removed.${it.name}") == true }.toSet())
    val removedFuels: StateFlow<Set<FuelProduct>> = _removedFuels.asStateFlow()

    private val _fuelErrors = MutableStateFlow<Set<FuelProduct>>(emptySet())
    val fuelErrors: StateFlow<Set<FuelProduct>> = _fuelErrors.asStateFlow()

    /**
     * OCR values with no recognized fuel label (e.g. "Diesel Comum").
     * They are never sent as-is: the contributor assigns each one to a
     * fuel (or dismisses it) before submit.
     */
    private val _unassignedAmounts = MutableStateFlow(
        savedState.get<ArrayList<String>>("unassigned")?.toList() ?: emptyList())
    val unassignedAmounts: StateFlow<List<String>> = _unassignedAmounts.asStateFlow()

    @Volatile private var generation = 0L
    private var recognition: Job? = null
    private var submitting = false
    private val queuedPhotoIds = savedState.get<ArrayList<String>>("queuedPhotoIds")?.toMutableSet() ?: mutableSetOf()
    private val edited = FuelProduct.entries.filter { savedState.get<Boolean>("edited.${it.name}") == true }.toMutableSet()
    private val _processing = MutableStateFlow(false)
    val processing = _processing.asStateFlow()
    private val _gateBusy = MutableStateFlow(false)
    val gateBusy = _gateBusy.asStateFlow()
    private val _gateFailure = MutableStateFlow<String?>(null)
    val gateFailure = _gateFailure.asStateFlow()
    private val _conditional = MutableStateFlow(savedState.get<Boolean>("conditional") ?: false)
    val conditional = _conditional.asStateFlow()
    private val _defaultPriceConfirmed = MutableStateFlow(false)
    val defaultPriceConfirmed = _defaultPriceConfirmed.asStateFlow()
    private val _reviewUri = MutableStateFlow<String?>(savedState["reviewUri"])
    val reviewUri = _reviewUri.asStateFlow()
    private var receipt: PhotoCapturePermission? = savedState.get<ArrayList<String>>("receipt")?.let { values ->
        runCatching { PhotoCapturePermission(values[0], values[1], values[2].toLong(), values[3].toLong(), values[4].toLong(), values[5], values[6], values.getOrNull(7) == "true") }.getOrNull()
    }
    private var pendingReceipt: PhotoCapturePermission? = savedState.get<ArrayList<String>>("pendingReceipt")?.let { values ->
        runCatching { PhotoCapturePermission(values[0], values[1], values[2].toLong(), values[3].toLong(), values[4].toLong(), values[5], values[6], values.getOrNull(7) == "true") }.getOrNull()
    }
    var pendingCaptureStartedAtMillis: Long = savedState.get<Long>("pendingCapturedAt") ?: 0L
        private set
    var originalCapturedAtMillis: Long = savedState.get<Long>("capturedAt") ?: 0L
        private set

    fun rememberPendingUri(uri: String?) { savedState["pendingUri"] = uri }
    fun pendingUri(): String? = savedState["pendingUri"]
    fun rememberReviewUri(uri: String?) { savedState["reviewUri"] = uri; _reviewUri.value = uri }
    fun confirmDefaultPrice(confirmed: Boolean) { _defaultPriceConfirmed.value = confirmed }

    /**
     * Server-issued receipt camera window with issuance leeway: phones
     * may trail the server clock by seconds, so a receipt that starts
     * "in the future" is still usable. Same leeway as receipt decode
     * (see [PhotoCaptureHttpClient]); expiry stays strict.
     */
    private fun receiptCameraWindowOpen(now: Long, issuedAtMillis: Long, cameraExpiresAtMillis: Long): Boolean =
        now >= issuedAtMillis - PhotoCaptureHttpClient.ISSUED_AT_LEEWAY_MILLIS && now < cameraExpiresAtMillis

    /** No camera callback is delivered on a missing fix, refused receipt or changed target. */
    fun authorizeCamera(onAuthorized: () -> Unit) {
        if (submitting) return
        if (_gateBusy.value || !flagProvider.isEnabled() || _targetInvalid.value) return
        val station = _target.value?.stationId ?: _pickedStationId.value
        if (station == null) { _gateFailure.value = "photo.station-required"; return }
        val ticket = ++generation
        _gateBusy.value = true
        _gateFailure.value = null
        viewModelScope.launch {
            try {
                val preview = DeveloperCaptureMode.available && _previewEnabled.value
                val clientId = java.util.UUID.randomUUID().toString()
                val permission = if (preview) captureGate.authorizeDevelopmentPreview(station, clientId) else {
                    val fix = captureLocation.freshFix()
                    if (fix == null || !fix.isEligibleForPhotoCapture(System.currentTimeMillis())) {
                        _gateFailure.value = "photo.location-required"
                        return@launch
                    }
                    captureGate.authorize(station, clientId, fix)
                }
                if (permission.developmentPreview != preview) { _gateFailure.value = "photo.permission-invalid"; return@launch }
                if (ticket != generation || station != (_target.value?.stationId ?: _pickedStationId.value)) return@launch
                if (permission.stationId != station || !captureGate.isCurrent(permission) ||
                    !receiptCameraWindowOpen(System.currentTimeMillis(), permission.issuedAtMillis, permission.cameraExpiresAtMillis)) {
                    _gateFailure.value = "photo.permission-expired"
                    return@launch
                }
                pendingReceipt = permission
                savedState["pendingReceipt"] = arrayListOf(permission.captureId, permission.stationId, permission.issuedAtMillis.toString(),
                    permission.cameraExpiresAtMillis.toString(), permission.expiresAtMillis.toString(), permission.ownerScope, permission.origin, permission.developmentPreview.toString())
                onAuthorized()
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (_: Exception) { if (ticket == generation) _gateFailure.value = "photo.authorization-unavailable" }
            finally { if (ticket == generation) _gateBusy.value = false }
        }
    }

    /** Recheck after Android's permission dialog; permission delay cannot extend the receipt. */
    fun beginCamera(): Boolean {
        val permission = pendingReceipt ?: return false
        val now = System.currentTimeMillis()
        if (!captureGate.isCurrent(permission) || !receiptCameraWindowOpen(now, permission.issuedAtMillis, permission.cameraExpiresAtMillis) ||
            permission.stationId != (_target.value?.stationId ?: _pickedStationId.value)) {
            _gateFailure.value = "photo.permission-expired"
            return false
        }
        pendingCaptureStartedAtMillis = now
        savedState["pendingCapturedAt"] = now
        return true
    }

    fun acceptCameraPhoto(bytes: ByteArray, mime: String, uri: String): Boolean {
        val permission = pendingReceipt ?: return false
        val now = System.currentTimeMillis()
        if (!captureGate.isCurrent(permission) || !receiptCameraWindowOpen(now, permission.issuedAtMillis, permission.cameraExpiresAtMillis)) {
            _gateFailure.value = "photo.permission-expired"
            return false
        }
        if (permission.developmentPreview && (!DeveloperCaptureMode.available || !_previewEnabled.value)) return false
        previewPhoto = permission.developmentPreview
        savedState["previewPhoto"] = previewPhoto
        _submit.value = null
        savedState.remove<Int>("queuedCount")
        submittedIds = emptyList()
        savedState.remove<ArrayList<String>>("submittedIds")
        receipt = permission
        savedState["receipt"] = savedState.get<ArrayList<String>>("pendingReceipt")
        pendingReceipt = null
        savedState.remove<ArrayList<String>>("pendingReceipt")
        originalCapturedAtMillis = now
        savedState["capturedAt"] = now
        edited.clear()
        _removedFuels.value = emptySet()
        _fuelAmounts.value = emptyMap()
        _unassignedAmounts.value = emptyList()
        savedState.remove<ArrayList<String>>("unassigned")
        _conditional.value = false
        FuelProduct.entries.forEach { product ->
            savedState.remove<String>("amount.${product.name}")
            savedState.remove<Boolean>("edited.${product.name}")
            savedState.remove<Boolean>("removed.${product.name}")
        }
        savedState["conditional"] = false
        rememberReviewUri(uri)
        if (previewPhoto) processPhoto(bytes, mime) else preparePhoto(bytes, mime)
        return true
    }

    fun cameraCancelled() {
        ++generation
        recognition?.cancel()
        _processing.value = false
        savedState["pendingUri"] = null
        pendingReceipt = null
        savedState.remove<ArrayList<String>>("pendingReceipt")
        _state.value = if (_reviewUri.value == null) CaptureOcrUiState.Cancelled else CaptureOcrUiState.NeedsConfirmation(emptyList(), true)
    }

    fun addFuel(product: FuelProduct) {
        if (submitting || _submit.value is SubmitState.Queued) return
        _removedFuels.value = _removedFuels.value - product
        savedState["removed.${product.name}"] = false
        setFuelAmount(product, _fuelAmounts.value[product].orEmpty())
    }

    fun setFuelAmount(product: FuelProduct, raw: String) {
        if (submitting || _submit.value is SubmitState.Queued) return
        edited += product
        savedState["edited.${product.name}"] = true
        savedState["amount.${product.name}"] = raw.take(16)

        _fuelAmounts.value = _fuelAmounts.value + (product to raw.take(16))
        if (raw.isNotBlank()) _fuelErrors.value = _fuelErrors.value - product
    }

    fun removeFuel(product: FuelProduct) {
        if (submitting || _submit.value is SubmitState.Queued) return
        edited += product
        savedState["edited.${product.name}"] = true
        savedState["removed.${product.name}"] = true
        savedState.remove<String>("amount.${product.name}")
        _removedFuels.value = _removedFuels.value + product
        _fuelAmounts.value = _fuelAmounts.value - product
        _fuelErrors.value = _fuelErrors.value - product
    }

    fun restoreFuels() {
        _removedFuels.value.forEach { addFuel(it) }
        _removedFuels.value = emptySet()
    }

    /**
     * Assigns an OCR value with no recognized fuel to the
     * contributor-chosen product: it becomes a normal editable row (kept
     * across reanalysis) and leaves the unassigned list.
     */
    fun assignUnassigned(index: Int, product: FuelProduct) {
        if (submitting || _submit.value is SubmitState.Queued) return
        val amount = _unassignedAmounts.value.getOrNull(index) ?: return
        setFuelAmount(product, amount)
        _unassignedAmounts.value = _unassignedAmounts.value.filterIndexed { i, _ -> i != index }
        savedState["unassigned"] = ArrayList(_unassignedAmounts.value)
    }

    /** Dismisses an unrecognized OCR value without assigning it. */
    fun dismissUnassigned(index: Int) {
        if (submitting || _submit.value is SubmitState.Queued) return
        if (index !in _unassignedAmounts.value.indices) return
        _unassignedAmounts.value = _unassignedAmounts.value.filterIndexed { i, _ -> i != index }
        savedState["unassigned"] = ArrayList(_unassignedAmounts.value)
    }

    /** Distance of the relevant station, or null when unverifiable. */
    fun stationDistance(stationId: String?): Double? =
        stationId?.let { id -> _nearby.value.firstOrNull { it.station.stationId == id }?.distanceMeters }

    /**
     * Entry permissions for the camera module: without camera the
     * contributor stays on the honest PermissionDenied path; without
     * location the station picker stays unavailable but manual review
     * still works. Nearby lookup starts only with a location grant.
     */
    fun onLocationPermission(granted: Boolean) {
        _locationDenied.value = !granted
        if (granted) loadNearby()
    }

    fun onEntryPermissions(cameraGranted: Boolean, locationGranted: Boolean) {
        _locationDenied.value = !locationGranted
        if (!cameraGranted) {
            _state.value = CaptureOcrUiState.PermissionDenied
            return
        }
        if (locationGranted) loadNearby()
    }

    fun loadNearby() {
        if (!flagProvider.isEnabled() || _previewEnabled.value) return
        viewModelScope.launch {
            _nearbyLoading.value = true
            _nearbyFailed.value = false
            try {
                val fix = locationHandler.getLastKnownLocation()
                if (fix == null) {
                    _locationDenied.value = true
                    return@launch
                }
                val outcome = nearbyStations(fix.latitude, fix.longitude)
                if (_previewEnabled.value) return@launch
                when (outcome) {
                    is NearbyServerStationsOutcome.Fresh -> {
                        _nearby.value = outcome.stations
                        if (outcome.stations.none { it.station.stationId == _pickedStationId.value }) {
                            _pickedStationId.value = null
                        }
                    }
                    is NearbyServerStationsOutcome.Disabled,
                    is NearbyServerStationsOutcome.Unavailable,
                    -> _nearbyFailed.value = true
                }
            } catch (error: Exception) {
                _nearbyFailed.value = true
            } finally {
                _nearbyLoading.value = false
            }
        }
    }

    fun pickStation(stationId: String?) {
        if (_pickedStationId.value != stationId) { generation++; pendingReceipt = null; receipt = null; _gateBusy.value = false }
        _pickedStationId.value = stationId
    }

    /** Pixels and encoding run outside the UI thread; edited/removed rows survive reanalysis. */
    fun preparePhoto(bytes: ByteArray, mime: String) {
        if (!flagProvider.isEnabled()) { _state.value = CaptureOcrUiState.Disabled; return }
        if (!permissionHandler.hasCameraPermission()) { _state.value = CaptureOcrUiState.PermissionDenied; return }
        if (originalCapturedAtMillis == 0L) {
            originalCapturedAtMillis = System.currentTimeMillis()
            savedState["capturedAt"] = originalCapturedAtMillis
        }
        processPhoto(bytes, mime)
    }

    fun replacePhoto(bytes: ByteArray, mime: String) = processPhoto(bytes, mime)

    private fun processPhoto(bytes: ByteArray, mime: String) {
        if (submitting || _submit.value is SubmitState.Queued) return
        recognition?.cancel()
        val ticket = ++generation
        _processing.value = true
        _photoRefused.value = null
        _state.value = CaptureOcrUiState.NeedsConfirmation(emptyList(), true)
        recognition = viewModelScope.launch {
            try {
                val prepared = withContext(Dispatchers.Default) {
                    photoFlow.prepareAt(bytes, mime, originalCapturedAtMillis).also { result ->
                        if (ticket != generation && result is PhotoFlow.PhotoResult.Ready) photoFlow.discard(result.id)
                    }
                }
                if (ticket != generation) {
                    if (prepared is PhotoFlow.PhotoResult.Ready) withContext(Dispatchers.IO) { photoFlow.discard(prepared.id) }
                    return@launch
                }
                when (prepared) {
                    is PhotoFlow.PhotoResult.Ready -> {
                        val old = _photoId.value
                        _photoId.value = prepared.id
                        savedState["photoId"] = prepared.id
                        if (old != null && old != prepared.id && old !in queuedPhotoIds) withContext(Dispatchers.IO) { photoFlow.discard(old) }
                    }
                    is PhotoFlow.PhotoResult.Refused -> {
                        // Never send the old uncropped photo after rejecting the user's crop.
                        _photoId.value?.takeIf { it !in queuedPhotoIds }?.let { id -> withContext(Dispatchers.IO) { photoFlow.discard(id) } }
                        _photoId.value = null
                        savedState.remove<String>("photoId")
                        _photoRefused.value = prepared.code
                    }
                }
                val result = withTimeout(15_000) { imageOcr.recognize(bytes) }
                if (ticket != generation) return@launch
                _conditional.value = _conditional.value || result.conditional
                savedState["conditional"] = _conditional.value
                _defaultPriceConfirmed.value = false
                val detected = result.rows.filter { it.product !in edited && it.product !in _removedFuels.value }
                    .associate { row -> row.product to formatMilli(row.amountMilli) }
                // Untouched OCR values reflect this crop; no stale guesses remain.
                _fuelAmounts.value = _fuelAmounts.value.filterKeys { it in edited } + detected
                val assigned = _fuelAmounts.value.values.toSet()
                val fresh = result.orphans.map(::formatMilli).filter { it !in assigned }.distinct()
                _unassignedAmounts.value = fresh
                savedState["unassigned"] = ArrayList(fresh)
                FuelProduct.entries.forEach { product ->
                    _fuelAmounts.value[product]?.let { savedState["amount.${product.name}"] = it }
                        ?: savedState.remove<String>("amount.${product.name}")
                }
            } catch (_: kotlinx.coroutines.TimeoutCancellationException) { if (ticket == generation) _photoRefused.value = "ocr.unavailable" }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (_: Exception) { if (ticket == generation) _photoRefused.value = _photoRefused.value ?: "ocr.unavailable" }
            finally { if (ticket == generation) _processing.value = false }
        }
    }

    private fun formatMilli(amount: Long): String =
        "${amount / 1000},${(amount % 1000).toString().padStart(3, '0')}"

    /**
     * Sends one contribution per filled fuel row (blank rows are not
     * sent, crossed-out rows are hidden). Every row carries the default
     * STANDARD condition; payment conditions are intentionally not
     * collected. The target resolves per row from the deep-link binding
     * or the picked nearby station combined with that row's fuel.
     */
    fun submitContributions() {
        if (previewPhoto && (!DeveloperCaptureMode.available || !_previewEnabled.value || receipt?.developmentPreview != true)) {
            _submit.value = SubmitState.Failed("photo.development-disabled"); return
        }
        if (submitting || _submit.value is SubmitState.Queued || _processing.value ||
            (_conditional.value && !_defaultPriceConfirmed.value)) return
        val invalid = mutableSetOf<FuelProduct>()
        val selected = FuelProduct.entries.filter { it !in _removedFuels.value }.mapNotNull { product ->
            val raw = _fuelAmounts.value[product].orEmpty()
            if (raw.isBlank()) return@mapNotNull null
            val confirmed = useCase.confirmManual(raw, product, DEFAULT_CONDITION, true)
                as? ConfirmPriceCaptureUseCase.ConfirmOutcome.Confirmed
            val target = resolveRowTarget(product)
            if (confirmed == null || target == null) { invalid += product; null }
            else Triple(confirmed, target, product)
        }
        _fuelErrors.value = invalid
        if (invalid.isNotEmpty() || selected.isEmpty()) { _submit.value = SubmitState.NoTarget; return }
        val permission = receipt
        val photo = _photoId.value
        val now = System.currentTimeMillis()
        if (permission == null || photo == null || !captureGate.isCurrent(permission) ||
            now >= permission.expiresAtMillis ||
            !receiptCameraWindowOpen(originalCapturedAtMillis, permission.issuedAtMillis, permission.cameraExpiresAtMillis) ||
            selected.any { it.second.stationId != permission.stationId }) {
            _submit.value = SubmitState.Failed("photo.review-unavailable")
            return
        }
        val context = PhotoContributionContext(permission.captureId, permission.expiresAtMillis, permission.ownerScope, permission.origin)
        val requests = selected.map { (confirmed, target, product) ->
            EnqueueContributionUseCase.Request(
                clientSubmissionId = "${permission.captureId}:${WireFuelMapper.toWire(product)}",
                stationId = target.stationId, fuelProduct = confirmed.product,
                amountMilliBrl = confirmed.candidate.priceMilli, conditionKind = confirmed.conditionKind,
                capturedAtMillis = originalCapturedAtMillis, photoId = photo, photoContext = context,
                unit = when (product) { FuelProduct.CNG -> "M3"; FuelProduct.LPG_P13 -> "KG_13"; else -> "L" },
            )
        }
        submitting = true
        _submit.value = SubmitState.Submitting
        viewModelScope.launch {
            try {
                _submit.value = when (val result = enqueue.invokeReview(requests)) {
                    EnqueueReviewOutcome.Disabled -> SubmitState.Disabled
                    is EnqueueReviewOutcome.Queued -> {
                        queuedPhotoIds += photo
                        savedState["queuedPhotoIds"] = ArrayList(queuedPhotoIds)
                        savedState["queuedCount"] = result.commands.size
                        submittedIds = result.commands.map { it.commandId }
                        savedState["submittedIds"] = ArrayList(submittedIds)
                        watchSubmitted(submittedIds)
                        SubmitState.Queued(result.historical, result.commands.size)
                    }
                }
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (_: Exception) { _submit.value = SubmitState.Failed("photo.review-unavailable") }
            finally { submitting = false }
        }
    }

    private fun resolveRowTarget(product: FuelProduct): ContributionTarget? {
        val bound = _target.value
        val stationId = bound?.stationId ?: _pickedStationId.value ?: return null
        return runCatching {
            ContributionTarget.create(stationId, WireFuelMapper.toWire(product))
        }.getOrNull()
    }

    /**
     * P37-T02 — submit result for the confirmed capture. `null` until
     * the first submit attempt. A transient network failure stays
     * `Failed` (outbox retry owns recovery); it never becomes accepted.
     */
    sealed interface SubmitState {
        data object NoTarget : SubmitState
        data object FuelMismatch : SubmitState
        data object Disabled : SubmitState
        data object Submitting : SubmitState
        data class Queued(val historical: Boolean, val count: Int = 1, val retrying: Boolean = false) : SubmitState
        data class Partial(val sent: Int, val reason: String) : SubmitState
        data class Failed(val reason: String) : SubmitState
        data class Sent(val count: Int, val pendingValidation: Boolean = false) : SubmitState
    }

    /** Minimum station area: the camera opens only inside this radius. */
    companion object {
        const val MIN_STATION_AREA_METERS: Double = 150.0
        const val DEFAULT_CONDITION: String = "STANDARD"

        fun isInsideStationArea(distanceMeters: Double?): Boolean =
            distanceMeters != null && distanceMeters.isFinite() && distanceMeters in 0.0..MIN_STATION_AREA_METERS
    }

    private val _submit = MutableStateFlow<SubmitState?>(savedState.get<Int>("queuedCount")?.let { SubmitState.Queued(false, it) })
    val submit: StateFlow<SubmitState?> = _submit.asStateFlow()

    /**
     * Command ids of the last submitted review, watched until every row
     * has a server receipt or rejection. Receipt is distinct from validation.
     * Survives process death without resubmitting the review.
     */
    private var submittedIds: List<String> =
        savedState.get<ArrayList<String>>("submittedIds")?.toList() ?: emptyList()
    private var statusPoller: kotlinx.coroutines.Job? = null

    init {
        if (_submit.value is SubmitState.Queued && submittedIds.isNotEmpty() && _reviewUri.value != null) {
            watchSubmitted(submittedIds)
        }
    }

    /**
     * Watches local durable receipts immediately, then on a bounded schedule.
     * Returning to the community never cancels the durable upload worker.
     */
    private fun watchSubmitted(ids: List<String>) {
        if (ids.isEmpty()) return
        statusPoller?.cancel()
        val uri = _reviewUri.value
        statusPoller = viewModelScope.launch {
            repeat(300) {
                if (_submit.value !is SubmitState.Queued || _reviewUri.value != uri) return@launch
                if (refreshSubmitted(ids)) return@launch
                kotlinx.coroutines.delay(2_000)
            }
        }
    }

    /** Returns true once a terminal feedback state was published. */
    private suspend fun refreshSubmitted(ids: List<String>): Boolean {
        val statuses = runCatching { ownedContributions.invoke() }.getOrNull() ?: return false
        val mine = statuses.filter { it.commandId in ids }
        if (mine.size < ids.size) return false
        val received = mine.count { it.state == com.anpfuel.domain.contribution.ContributionState.Accepted ||
            it.state == com.anpfuel.domain.contribution.ContributionState.Pending }
        val rejected = mine.filter { it.state == com.anpfuel.domain.contribution.ContributionState.Rejected }
        _submit.value = when {
            received == ids.size -> SubmitState.Sent(ids.size,
                mine.any { it.state == com.anpfuel.domain.contribution.ContributionState.Pending })
            rejected.isNotEmpty() && received + rejected.size == ids.size ->
                SubmitState.Partial(received, rejected.firstNotNullOfOrNull { it.reason } ?: "contribution.rejected")
            else -> {
                val queued = _submit.value as? SubmitState.Queued ?: return false
                _submit.value = queued.copy(retrying = mine.any {
                    (it.state as? com.anpfuel.domain.contribution.ContributionState.Queued)?.retryable == true })
                return false
            }
        }
        return true
    }

    fun bindTarget(stationId: String?, fuelProductWire: String?) {
        if ((_target.value?.stationId ?: pendingReceipt?.stationId ?: receipt?.stationId) != stationId) { generation++; pendingReceipt = null; receipt = null; _gateBusy.value = false }
        if (stationId == null) {
            _target.value = null
            _targetInvalid.value = false
            return
        }
        val resolved = runCatching {
            ContributionTarget.create(stationId, fuelProductWire ?: "GASOLINE_REGULAR")
        }.getOrNull()
        _target.value = resolved
        _targetInvalid.value = resolved == null
    }

    /**
     * P37-T02 — enqueues the confirmed capture to the durable outbox
     * with the bound canonical target. Requires the bound target and a
     * fuel match between target wire and confirmed product (prevents
     * cross-fuel misattribution); metadata-only when no photo is
     * attached, honestly labelled by the use case. Identity proof
     * travels at worker submit time through the contribution gateway.
     *
     * Community camera entry: when no station deep-link target is bound,
     * the contributor-picked nearby station combines with the confirmed
     * product wire by construction (no mismatch possible); the prepared
     * PhotoFlow entry travels as photo_id when a shot fit the budgets.
     */
    fun submitConfirmed() {
        if (previewPhoto || _previewEnabled.value) { _submit.value = SubmitState.Failed("photo.review-required"); return }
        val confirmed = _state.value as? CaptureOcrUiState.Confirmed
        val target = _target.value ?: _pickedStationId.value?.let { stationId ->
            runCatching {
                ContributionTarget.create(stationId, WireFuelMapper.toWire(confirmed?.product ?: return@let null))
            }.getOrNull()
        }
        if (confirmed == null || target == null) {
            _submit.value = SubmitState.NoTarget
            return
        }
        val targetProduct = WireFuelMapper.fromWire(target.fuelProductWire).getOrNull()
        if (targetProduct != confirmed.product) {
            _submit.value = SubmitState.FuelMismatch
            return
        }
        viewModelScope.launch {
            val outcome = try {
                enqueue.invoke(
                    EnqueueContributionUseCase.Request(
                        clientSubmissionId = java.util.UUID.randomUUID().toString(),
                        stationId = target.stationId,
                        fuelProduct = confirmed.product,
                        amountMilliBrl = confirmed.candidate.priceMilli,
                        conditionKind = confirmed.conditionKind,
                        capturedAtMillis = System.currentTimeMillis(),
                        photoId = _photoId.value,
                    ),
                )
            } catch (error: Exception) {
                _submit.value = SubmitState.Failed(error.message ?: error.javaClass.simpleName)
                return@launch
            }
            _submit.value = when (outcome) {
                is EnqueueContributionOutcome.Disabled -> SubmitState.Disabled
                is EnqueueContributionOutcome.Queued -> SubmitState.Queued(outcome.historical)
            }
        }
    }

    /** Reviews one capture result: system-camera bytes already compressed via PhotoFlow. */
    fun onCaptureResult(cancelled: Boolean, ocrText: String?) {
        val outcome = useCase.start(
            ConfirmPriceCaptureUseCase.CaptureRequest(
                hasPermission = permissionHandler.hasCameraPermission(),
                cancelled = cancelled,
                ocrText = ocrText,
            ),
        )
        _state.value = outcome.toUiState()
    }

    /**
     * P21-T02 — confirms one OCR candidate with explicit approval plus
     * contributor-picked product and condition.
     */
    fun onConfirm(
        candidate: PortablePriceOcr.OcrCandidate?,
        product: FuelProduct?,
        conditionKind: String?,
        humanConfirmed: Boolean,
    ) {
        when (val outcome = useCase.confirm(candidate, product, conditionKind, humanConfirmed)) {
            is ConfirmPriceCaptureUseCase.ConfirmOutcome.Confirmed ->
                _state.value = CaptureOcrUiState.Confirmed(
                    outcome.candidate,
                    outcome.product,
                    outcome.conditionKind,
                )
            is ConfirmPriceCaptureUseCase.ConfirmOutcome.StillNeedsChoice ->
                _state.value = CaptureOcrUiState.NeedsConfirmation(
                    candidates = outcome.candidates,
                    lowConfidence = outcome.candidates.isEmpty() ||
                        outcome.candidates.any { useCase.isLowConfidence(it) },
                )
        }
    }

    /**
     * P21-T02 — confirms a human-typed price for empty/low-confidence OCR
     * sets. A failed parse keeps the current candidate list instead of
     * replacing it with an empty choice.
     */
    fun onConfirmManual(
        rawPrice: String?,
        product: FuelProduct?,
        conditionKind: String?,
        humanConfirmed: Boolean,
    ) {
        when (val outcome = useCase.confirmManual(rawPrice, product, conditionKind, humanConfirmed)) {
            is ConfirmPriceCaptureUseCase.ConfirmOutcome.Confirmed ->
                _state.value = CaptureOcrUiState.Confirmed(
                    outcome.candidate,
                    outcome.product,
                    outcome.conditionKind,
                )
            is ConfirmPriceCaptureUseCase.ConfirmOutcome.StillNeedsChoice -> {
                val current = _state.value
                if (current !is CaptureOcrUiState.NeedsConfirmation) {
                    _state.value = CaptureOcrUiState.NeedsConfirmation(
                        candidates = emptyList(),
                        lowConfidence = true,
                    )
                }
            }
        }
    }

    private fun initialState(): CaptureOcrUiState =
        if (!flagProvider.isEnabled()) CaptureOcrUiState.Disabled
        else CaptureOcrUiState.NeedsConfirmation(emptyList(), lowConfidence = true)

    private fun ConfirmPriceCaptureUseCase.StartOutcome.toUiState(): CaptureOcrUiState =
        when (this) {
            ConfirmPriceCaptureUseCase.StartOutcome.Disabled -> CaptureOcrUiState.Disabled
            ConfirmPriceCaptureUseCase.StartOutcome.PermissionDenied ->
                CaptureOcrUiState.PermissionDenied
            ConfirmPriceCaptureUseCase.StartOutcome.Cancelled -> CaptureOcrUiState.Cancelled
            is ConfirmPriceCaptureUseCase.StartOutcome.NeedsHumanChoice ->
                CaptureOcrUiState.NeedsConfirmation(
                    candidates = candidates,
                    lowConfidence = candidates.isEmpty() ||
                        candidates.any { useCase.isLowConfidence(it) },
                )
        }
}
