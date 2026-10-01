package com.anpfuel.app.capture

import android.content.Context
import com.anpfuel.application.port.CaptureOcrFlagProvider
import com.anpfuel.application.port.OcrPort
import com.anpfuel.application.usecase.capture.ConfirmPriceCaptureUseCase
import com.anpfuel.domain.portable.PortablePriceOcr
import com.anpfuel.domain.valueobject.FuelProduct
import io.mockk.every
import io.mockk.mockk
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T04: ViewModel gates on flag/permission/cancel and never
 * auto-confirms or uploads.
 */
class CaptureOcrViewModelTest {

    private fun viewModel(
        enabled: Boolean,
        hasPermission: Boolean,
        ocrText: (String) -> List<PortablePriceOcr.OcrCandidate> =
            PortablePriceOcr::parseCandidates,
    ): CaptureOcrViewModel {
        val flags = object : CaptureOcrFlagProvider {
            override fun isEnabled(): Boolean = enabled
        }
        val ocr = object : OcrPort {
            override fun candidatesFromText(text: String) = ocrText(text)
        }
        val useCase = ConfirmPriceCaptureUseCase(flags, ocr)
        val context = mockk<Context>(relaxed = true)
        val permissions = CameraPermissionHandler(context)
        val handler = mockk<CameraPermissionHandler>()
        every { handler.hasCameraPermission() } returns hasPermission
        return CaptureOcrViewModel(useCase, handler, flags)
    }

    @Test
    fun `denied permission surfaces PermissionDenied`() {
        val vm = viewModel(enabled = true, hasPermission = false)
        vm.onCaptureResult(cancelled = false, ocrText = "R$ 5,89")
        assertTrue(vm.state.value is CaptureOcrUiState.PermissionDenied)
    }

    @Test
    fun `cancelled capture surfaces Cancelled`() {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.onCaptureResult(cancelled = true, ocrText = "R$ 5,89")
        assertTrue(vm.state.value is CaptureOcrUiState.Cancelled)
    }

    @Test
    fun `multi-price capture needs human choice and confirms explicitly`() {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.onCaptureResult(cancelled = false, ocrText = "R$ 6,19\nR$ 5,89")
        val pending = vm.state.value
        assertTrue(pending is CaptureOcrUiState.NeedsConfirmation)
        assertTrue((pending as CaptureOcrUiState.NeedsConfirmation).candidates.size == 2)

        vm.onConfirm(pending.candidates.first(), null, true)
        assertTrue(vm.state.value is CaptureOcrUiState.NeedsConfirmation)

        vm.onConfirm(pending.candidates[1], FuelProduct.GASOLINE_REGULAR, true)
        val done = vm.state.value
        assertTrue(done is CaptureOcrUiState.Confirmed)
        assertTrue((done as CaptureOcrUiState.Confirmed).candidate.priceMilli == 5890L)
    }
}
