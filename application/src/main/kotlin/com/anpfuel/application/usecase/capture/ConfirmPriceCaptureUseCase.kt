package com.anpfuel.application.usecase.capture

import com.anpfuel.application.port.CaptureOcrFlagProvider
import com.anpfuel.application.port.OcrPort
import com.anpfuel.domain.portable.PortableMoney
import com.anpfuel.domain.portable.PortableMoneyException
import com.anpfuel.domain.portable.PortablePriceOcr
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P10-T04 — Capture + local OCR confirmation (UC-CAPTURE-01,
 * B-BR-010/011/015/016).
 *
 * Flag disabled: [StartOutcome.Disabled], caller keeps browsing/ANP and
 * metadata-only paths. Flag enabled + no camera permission:
 * [StartOutcome.PermissionDenied] (nothing read, nothing cached).
 * Cancelled capture: [StartOutcome.Cancelled] (no partial entry, matching
 * the write-then-publish [com.anpfuel.application.portable.PhotoFlow]).
 * Otherwise OCR candidates surface as [StartOutcome.NeedsHumanChoice] —
 * even a single high-confidence candidate still waits for the contributor
 * to confirm price + pick product/condition. There is no auto-confirm and
 * no `min()` lowest-price choice anywhere in this flow.
 *
 * [confirm] completes only when the contributor explicitly confirms with a
 * non-null product and an explicitly picked condition; otherwise it returns
 * [ConfirmOutcome.StillNeedsChoice]. [confirmManual] accepts a human-typed
 * price (exact [PortableMoney] milli-BRL, never rounded) for empty or
 * low-confidence OCR sets. This use case never uploads: P10-T05 owns the
 * outbox/direct media flow.
 */
class ConfirmPriceCaptureUseCase(
    private val flagProvider: CaptureOcrFlagProvider,
    private val ocr: OcrPort,
) {

    /** Capture intake: permission and cancellation are explicit inputs. */
    data class CaptureRequest(
        val hasPermission: Boolean,
        val cancelled: Boolean,
        val ocrText: String?,
    )

    sealed interface StartOutcome {
        data object Disabled : StartOutcome
        data object PermissionDenied : StartOutcome
        data object Cancelled : StartOutcome
        data class NeedsHumanChoice(
            val candidates: List<PortablePriceOcr.OcrCandidate>,
        ) : StartOutcome
    }

    sealed interface ConfirmOutcome {
        data class StillNeedsChoice(
            val candidates: List<PortablePriceOcr.OcrCandidate>,
        ) : ConfirmOutcome

        data class Confirmed(
            val candidate: PortablePriceOcr.OcrCandidate,
            val product: FuelProduct,
            val conditionKind: String,
        ) : ConfirmOutcome
    }

    /**
     * Starts one capture review. The OCR port runs only when the flag is
     * on, permission is granted and the capture was not cancelled, so a
     * denied/cancelled/disabled capture leaves no cache entry and performs
     * no recognition work.
     */
    fun start(request: CaptureRequest): StartOutcome {
        if (!flagProvider.isEnabled()) return StartOutcome.Disabled
        if (!request.hasPermission) return StartOutcome.PermissionDenied
        if (request.cancelled) return StartOutcome.Cancelled
        val text = request.ocrText
        if (text.isNullOrBlank()) {
            return StartOutcome.NeedsHumanChoice(emptyList())
        }
        return StartOutcome.NeedsHumanChoice(ocr.candidatesFromText(text))
    }

    /**
     * Confirms one candidate only with explicit human approval plus a
     * contributor-picked product and condition. Condition/product are never
     * taken from OCR; a null product, blank condition or
     * `humanConfirmed = false` stays in choice.
     */
    fun confirm(
        candidate: PortablePriceOcr.OcrCandidate?,
        product: FuelProduct?,
        conditionKind: String?,
        humanConfirmed: Boolean,
    ): ConfirmOutcome {
        if (!flagProvider.isEnabled()) {
            return ConfirmOutcome.StillNeedsChoice(
                if (candidate == null) emptyList() else listOf(candidate),
            )
        }
        val condition = conditionKind?.trim().orEmpty()
        if (candidate == null || product == null || condition.isEmpty() || !humanConfirmed) {
            return ConfirmOutcome.StillNeedsChoice(
                if (candidate == null) emptyList() else listOf(candidate),
            )
        }
        return ConfirmOutcome.Confirmed(candidate, product, condition)
    }

    /**
     * Confirms a human-typed price for empty/low-confidence OCR sets. The
     * raw text must parse to exact milli-BRL; invalid input stays in
     * choice with an empty set so the caller keeps its own candidates.
     * Typed entries skip the OCR confidence gate ([OcrCandidate.manualEntry]).
     */
    fun confirmManual(
        rawPrice: String?,
        product: FuelProduct?,
        conditionKind: String?,
        humanConfirmed: Boolean,
    ): ConfirmOutcome {
        if (!flagProvider.isEnabled()) return ConfirmOutcome.StillNeedsChoice(emptyList())
        val condition = conditionKind?.trim().orEmpty()
        if (product == null || condition.isEmpty() || !humanConfirmed) {
            return ConfirmOutcome.StillNeedsChoice(emptyList())
        }
        val raw = rawPrice?.trim().orEmpty()
        if (raw.isEmpty()) return ConfirmOutcome.StillNeedsChoice(emptyList())
        val milli = try {
            PortableMoney.parse(raw)
        } catch (error: PortableMoneyException) {
            return ConfirmOutcome.StillNeedsChoice(emptyList())
        }
        return ConfirmOutcome.Confirmed(
            candidate = PortablePriceOcr.OcrCandidate(
                priceMilli = milli,
                raw = raw,
                confidence = 1.0,
                hasCurrencyMarker = false,
                manualEntry = true,
            ),
            product = product,
            conditionKind = condition,
        )
    }

    /** Local hint: below-threshold candidates require manual entry. */
    fun isLowConfidence(candidate: PortablePriceOcr.OcrCandidate): Boolean =
        PortablePriceOcr.isLowConfidence(candidate)
}
