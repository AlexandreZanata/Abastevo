package com.anpfuel.application.usecase.capture

import com.anpfuel.application.port.CaptureOcrFlagProvider
import com.anpfuel.application.port.OcrPort
import com.anpfuel.domain.portable.PortablePriceOcr
import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T04: permission denied, cancel, multiple prices/conditions, low
 * OCR confidence, explicit human confirmation. No upload path exists in
 * this use case (P10-T05 owns the outbox).
 */
class ConfirmPriceCaptureUseCaseTest {

    private class FakeFlags(val enabled: Boolean) : CaptureOcrFlagProvider {
        override fun isEnabled(): Boolean = enabled
    }

    private class FakeOcr(
        val backing: (String) -> List<PortablePriceOcr.OcrCandidate> =
            PortablePriceOcr::parseCandidates,
    ) : OcrPort {
        var calls = 0
        override fun candidatesFromText(ocrText: String): List<PortablePriceOcr.OcrCandidate> {
            calls += 1
            return backing(ocrText)
        }
    }

    private fun candidate(raw: String, milli: Long, confidence: Double, marked: Boolean) =
        PortablePriceOcr.OcrCandidate(milli, raw, confidence, marked)

    @Test
    fun `disabled flag never runs OCR`() {
        val ocr = FakeOcr()
        val useCase = ConfirmPriceCaptureUseCase(FakeFlags(false), ocr)
        val out = useCase.start(
            ConfirmPriceCaptureUseCase.CaptureRequest(true, false, "R$ 5,89"),
        )
        assertTrue(out is ConfirmPriceCaptureUseCase.StartOutcome.Disabled)
        assertTrue(ocr.calls == 0)
    }

    @Test
    fun `permission denied leaves no recognition work`() {
        val ocr = FakeOcr()
        val useCase = ConfirmPriceCaptureUseCase(FakeFlags(true), ocr)
        val out = useCase.start(
            ConfirmPriceCaptureUseCase.CaptureRequest(false, false, "R$ 5,89"),
        )
        assertTrue(out is ConfirmPriceCaptureUseCase.StartOutcome.PermissionDenied)
        assertTrue(ocr.calls == 0)
    }

    @Test
    fun `cancelled capture leaves no partial entry`() {
        val ocr = FakeOcr()
        val useCase = ConfirmPriceCaptureUseCase(FakeFlags(true), ocr)
        val out = useCase.start(
            ConfirmPriceCaptureUseCase.CaptureRequest(true, true, "R$ 5,89"),
        )
        assertTrue(out is ConfirmPriceCaptureUseCase.StartOutcome.Cancelled)
        assertTrue(ocr.calls == 0)
    }

    @Test
    fun `multiple prices require human choice without auto-pick`() {
        val useCase = ConfirmPriceCaptureUseCase(FakeFlags(true), FakeOcr())
        val out = useCase.start(
            ConfirmPriceCaptureUseCase.CaptureRequest(
                true, false, "R$ 6,19\nR$ 5,89",
            ),
        )
        assertTrue(out is ConfirmPriceCaptureUseCase.StartOutcome.NeedsHumanChoice)
        val choices = (out as ConfirmPriceCaptureUseCase.StartOutcome.NeedsHumanChoice).candidates
        assertTrue(choices.size == 2)
        assertTrue(choices[0].priceMilli == 6190L && choices[1].priceMilli == 5890L)
    }

    @Test
    fun `low confidence surfaces manual entry and confirm stays until human picks`() {
        val useCase = ConfirmPriceCaptureUseCase(FakeFlags(true), FakeOcr())
        val out = useCase.start(
            ConfirmPriceCaptureUseCase.CaptureRequest(true, false, "5,89"),
        )
        assertTrue(out is ConfirmPriceCaptureUseCase.StartOutcome.NeedsHumanChoice)
        val choices = (out as ConfirmPriceCaptureUseCase.StartOutcome.NeedsHumanChoice).candidates
        assertTrue(choices.size == 1 && useCase.isLowConfidence(choices[0]))

        val noProduct = useCase.confirm(choices[0], null, true)
        assertTrue(noProduct is ConfirmPriceCaptureUseCase.ConfirmOutcome.StillNeedsChoice)
        val unconfirmed = useCase.confirm(choices[0], FuelProduct.GASOLINE_REGULAR, false)
        assertTrue(unconfirmed is ConfirmPriceCaptureUseCase.ConfirmOutcome.StillNeedsChoice)
        val ok = useCase.confirm(choices[0], FuelProduct.GASOLINE_REGULAR, true)
        assertTrue(ok is ConfirmPriceCaptureUseCase.ConfirmOutcome.Confirmed)
        assertTrue((ok as ConfirmPriceCaptureUseCase.ConfirmOutcome.Confirmed).product ==
            FuelProduct.GASOLINE_REGULAR)
    }

    @Test
    fun `condition is never taken from OCR`() {
        val ocr = FakeOcr { _ ->
            listOf(candidate("5,89", 5890L, 0.5, false))
        }
        val useCase = ConfirmPriceCaptureUseCase(FakeFlags(true), ocr)
        val out = useCase.start(
            ConfirmPriceCaptureUseCase.CaptureRequest(true, false, "5,89"),
        )
        assertTrue(out is ConfirmPriceCaptureUseCase.StartOutcome.NeedsHumanChoice)
        val stayed = useCase.confirm(
            (out as ConfirmPriceCaptureUseCase.StartOutcome.NeedsHumanChoice).candidates.first(),
            null,
            humanConfirmed = true,
        )
        assertTrue(stayed is ConfirmPriceCaptureUseCase.ConfirmOutcome.StillNeedsChoice)
    }
}
