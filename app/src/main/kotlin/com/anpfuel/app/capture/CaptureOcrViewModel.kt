package com.anpfuel.app.capture

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.port.CaptureOcrFlagProvider
import com.anpfuel.application.usecase.capture.ConfirmPriceCaptureUseCase
import com.anpfuel.application.usecase.contribution.EnqueueContributionOutcome
import com.anpfuel.application.usecase.contribution.EnqueueContributionUseCase
import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.contribution.ContributionTarget
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
) : ViewModel() {

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
     * P37-T02 — submit result for the confirmed capture. `null` until
     * the first submit attempt. A transient network failure stays
     * `Failed` (outbox retry owns recovery); it never becomes accepted.
     */
    sealed interface SubmitState {
        data object NoTarget : SubmitState
        data object FuelMismatch : SubmitState
        data object Disabled : SubmitState
        data class Queued(val historical: Boolean) : SubmitState
        data class Failed(val reason: String) : SubmitState
    }

    private val _submit = MutableStateFlow<SubmitState?>(null)
    val submit: StateFlow<SubmitState?> = _submit.asStateFlow()

    fun bindTarget(stationId: String?, fuelProductWire: String?) {
        if (stationId == null || fuelProductWire == null) {
            _target.value = null
            _targetInvalid.value = false
            return
        }
        val resolved = runCatching {
            ContributionTarget.create(stationId, fuelProductWire)
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
     */
    fun submitConfirmed() {
        val confirmed = _state.value as? CaptureOcrUiState.Confirmed
        val target = _target.value
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
                        photoId = null,
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
